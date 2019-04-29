package wavelet

import (
	"bytes"
	"context"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"runtime"
	"sync"
)

type verificationRequest struct {
	creator   common.AccountID
	payload   []byte
	signature common.Signature
	response  chan bool
}

type verifier struct {
	wg     sync.WaitGroup
	cancel func()
	bus    chan verificationRequest
}

func NewVerifier(workersNum int, capacity uint32) *verifier {
	ctx, cancel := context.WithCancel(context.Background())
	v := verifier{
		cancel: cancel,
		bus:    make(chan verificationRequest, capacity),
	}

	v.wg.Add(workersNum)
	for i := 0; i < workersNum; i++ {
		go func(ctx context.Context) {
			defer v.wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				case request := <-v.bus:
					request.response <- edwards25519.Verify(request.creator, request.payload, request.signature)
				}
			}
		}(ctx)
	}

	return &v
}

func (v *verifier) Stop() {
	v.cancel()
	v.wg.Wait()
	close(v.bus)
}

func (v *verifier) verify(creator common.AccountID, payload []byte, signature common.Signature) bool {
	request := verificationRequest{
		creator:   creator,
		payload:   payload,
		signature: signature,
		response:  make(chan bool, 1),
	}
	v.bus <- request
	return <-request.response
}

type Graph struct {
	transactions map[common.TransactionID]*Transaction           // All transactions.
	children     map[common.TransactionID][]common.TransactionID // Children of transactions.

	eligible   map[common.TransactionID]struct{} // Transactions that are eligible to be parent transactions.
	incomplete map[common.TransactionID]struct{} // Transactions that don't have all parents available.

	missing     map[common.TransactionID]struct{} // Transactions that we are missing.
	missingLock sync.Mutex

	seedIndex  map[byte]map[common.TransactionID]struct{}   // Indexes transactions by their seed.
	depthIndex map[uint64]map[common.TransactionID]struct{} // Indexes transactions by their depth.
	roundIndex map[uint64]map[common.TransactionID]struct{} // Indexes transactions by their round.

	rootID common.TransactionID // Root of the graph.
	height uint64               // Height of the graph.

	verifier *verifier
}

func NewGraph(genesis *Round) *Graph {
	g := &Graph{
		transactions: make(map[common.TransactionID]*Transaction),
		children:     make(map[common.TransactionID][]common.TransactionID),

		eligible:   make(map[common.TransactionID]struct{}),
		incomplete: make(map[common.TransactionID]struct{}),

		missing: make(map[common.TransactionID]struct{}),

		seedIndex:  make(map[byte]map[common.TransactionID]struct{}),
		depthIndex: make(map[uint64]map[common.TransactionID]struct{}),
		roundIndex: make(map[uint64]map[common.TransactionID]struct{}),

		height: 1,
	}

	if genesis != nil {
		g.rootID = genesis.Root.ID
		g.transactions[genesis.Root.ID] = &genesis.Root
	} else {
		ptr := new(Transaction)

		g.rootID = ptr.ID
		g.transactions[ptr.ID] = ptr
	}

	root := g.transactions[g.rootID]

	g.height = root.Depth + 1
	g.createTransactionIndices(root)
	g.verifier = NewVerifier(runtime.NumCPU(), 1024)

	return g
}

func (g *Graph) assertTransactionIsValid(tx *Transaction) error {
	if tx.ID == common.ZeroTransactionID {
		return errors.New("tx must have an id")
	}

	if tx.Sender == common.ZeroAccountID {
		return errors.New("tx must have sender associated to it")
	}

	if tx.Creator == common.ZeroAccountID {
		return errors.New("tx must have a creator associated to it")
	}

	if len(tx.ParentIDs) == 0 {
		return errors.New("transaction has no parents")
	}

	// Check that parents are lexicographically sorted, are not itself, and are unique.
	set := make(map[common.TransactionID]struct{})

	for i := len(tx.ParentIDs) - 1; i > 0; i-- {
		if tx.ID == tx.ParentIDs[i] {
			return errors.New("tx must not include itself in its parents")
		}

		if bytes.Compare(tx.ParentIDs[i-1][:], tx.ParentIDs[i][:]) > 0 {
			return errors.New("tx must have sorted parent ids")
		}

		if _, duplicate := set[tx.ParentIDs[i]]; duplicate {
			return errors.New("tx must not have duplicate parent ids")
		}

		set[tx.ParentIDs[i]] = struct{}{}
	}

	if tx.Tag > sys.TagStake {
		return errors.New("tx has an unknown tag")
	}

	if tx.Tag != sys.TagNop && len(tx.Payload) == 0 {
		return errors.New("tx must have payload if not a nop transaction")
	}

	if tx.Tag == sys.TagNop && len(tx.Payload) != 0 {
		return errors.New("tx must have no payload if is a nop transaction")
	}

	var nonce [8]byte // TODO(kenta): nonce

	if !g.verifier.verify(tx.Creator, append(nonce[:], append([]byte{tx.Tag}, tx.Payload...)...), tx.CreatorSignature) {
		return errors.New("tx has invalid creator signature")
	}

	cpy := *tx
	cpy.SenderSignature = common.ZeroSignature

	if !g.verifier.verify(tx.Sender, cpy.Marshal(), tx.SenderSignature) {
		return errors.New("tx has invalid sender signature")
	}

	return nil
}

func (g *Graph) assertTransactionIsComplete(tx *Transaction) error {
	// Check that the transaction's depth is correct according to its parents.
	var maxDepth uint64
	var maxConfidence uint64

	for _, parentID := range tx.ParentIDs {
		parent, exists := g.lookupTransactionByID(parentID)

		if !exists {
			return errors.New("parent not stored in graph")
		}

		// Check if the depth of each parents is acceptable.
		if parent.Depth+sys.MaxEligibleParentsDepthDiff < tx.Depth {
			return errors.Errorf("tx parents exceeds max eligible parents depth diff: parents depth is %d, but tx depth is %d", parent.Depth, tx.Depth)
		}

		// Update max depth witnessed from parents.
		if maxDepth < parent.Depth {
			maxDepth = parent.Depth
		}

		// Update max confidence witnessed from parents.
		if maxConfidence < parent.Confidence {
			maxConfidence = parent.Confidence
		}
	}

	maxDepth++
	maxConfidence += uint64(len(tx.ParentIDs))

	if tx.Depth != maxDepth {
		return errors.Errorf("transactions depth is invalid, expected depth to be %d but got %d", maxDepth, tx.Depth)
	}

	if tx.Confidence != maxConfidence {
		return errors.Errorf("transactions confidence is invalid, expected confidence to be %d but got %d", maxConfidence, tx.Confidence)
	}

	return nil
}

func (g *Graph) processParents(tx *Transaction) []common.TransactionID {
	var missingParentIDs []common.TransactionID

	for _, parentID := range tx.ParentIDs {
		_, exists := g.lookupTransactionByID(parentID)

		_, incomplete := g.incomplete[parentID]

		if !exists || incomplete {
			missingParentIDs = append(missingParentIDs, parentID)
		}

		g.children[parentID] = append(g.children[parentID], tx.ID)

		delete(g.eligible, parentID)
	}

	return missingParentIDs
}

func (g *Graph) lookupTransactionByID(id common.TransactionID) (*Transaction, bool) {
	tx, exists := g.transactions[id]

	if !exists {
		g.missingLock.Lock()
		if _, missing := g.missing[id]; !missing {
			g.missing[id] = struct{}{}
		}
		g.missingLock.Unlock()
	}

	return tx, exists
}

var (
	ErrMissingParents = errors.New("parents for transaction are not in graph")
	ErrAlreadyExists  = errors.New("transaction already exists in the graph")
)

func (g *Graph) addTransaction(tx Transaction) error {
	if _, exists := g.transactions[tx.ID]; exists {
		return ErrAlreadyExists
	}

	ptr := &tx

	if err := g.assertTransactionIsValid(ptr); err != nil {
		return err
	}

	// Add transaction to the view-graph.
	g.transactions[tx.ID] = ptr

	g.missingLock.Lock()
	delete(g.missing, ptr.ID)
	g.missingLock.Unlock()

	missing := g.processParents(ptr)

	if len(missing) > 0 {
		g.incomplete[ptr.ID] = struct{}{}
		return ErrMissingParents
	}

	return g.markTransactionAsComplete(ptr)
}

// deleteTransaction deletes all traces of a transaction from the graph. Note
// however that it does not remove the transaction from any of the graphs
// indices.
func (g *Graph) deleteTransaction(id common.TransactionID) {
	if tx, exists := g.transactions[id]; exists {
		delete(g.seedIndex[tx.Seed], id)
		delete(g.depthIndex[tx.Depth], id)

		if len(g.seedIndex[tx.Seed]) == 0 {
			delete(g.seedIndex, tx.Seed)
		}

		if len(g.depthIndex[tx.Depth]) == 0 {
			delete(g.depthIndex, tx.Depth)
		}
	}

	delete(g.transactions, id)
	delete(g.children, id)

	delete(g.eligible, id)
	delete(g.incomplete, id)

	g.missingLock.Lock()
	delete(g.missing, id)
	g.missingLock.Unlock()
}

// deleteIncompleteTransaction explicitly deletes all traces of a transaction
// alongside its progeny from the graph. Note that incomplete transactions
// are not stored in any indices of the graph, so the function should ONLY
// be used to delete incomplete transactions that have not yet been indexed.
func (g *Graph) deleteIncompleteTransaction(id common.TransactionID) {
	children := g.children[id]

	g.deleteTransaction(id)

	for _, childID := range children {
		g.deleteTransaction(childID)
	}
}

func (g *Graph) createTransactionIndices(tx *Transaction) {
	if _, exists := g.seedIndex[tx.Seed]; !exists {
		g.seedIndex[tx.Seed] = make(map[common.TransactionID]struct{})
	}

	g.seedIndex[tx.Seed][tx.ID] = struct{}{}

	if _, exists := g.depthIndex[tx.Depth]; !exists {
		g.depthIndex[tx.Depth] = make(map[common.TransactionID]struct{})
	}

	g.depthIndex[tx.Depth][tx.ID] = struct{}{}

	if g.height < tx.Depth {
		g.height = tx.Depth + 1
	}

	if _, exists := g.children[tx.ID]; !exists {
		if tx.Depth+sys.MaxEligibleParentsDepthDiff >= g.height {
			g.eligible[tx.ID] = struct{}{}
		}
	}
}

func (g *Graph) findEligibleParents() []common.TransactionID {
	root := g.transactions[g.rootID]

	var eligibleIDs []common.TransactionID

	for eligibleID := range g.eligible {
		eligibleParent, exists := g.transactions[eligibleID]

		if !exists {
			delete(g.eligible, eligibleID)
			continue
		}

		if eligibleParent.ID != root.ID && eligibleParent.Depth <= root.Depth {
			delete(g.eligible, eligibleID)
			continue
		}

		if eligibleParent.Depth+sys.MaxEligibleParentsDepthDiff <= g.height {
			delete(g.eligible, eligibleID)
			continue
		}

		eligibleIDs = append(eligibleIDs, eligibleID)
	}

	return eligibleIDs
}

func (g *Graph) markTransactionAsComplete(tx *Transaction) error {
	err := g.assertTransactionIsComplete(tx)

	if err != nil {
		g.deleteIncompleteTransaction(tx.ID)
		return err
	}

	// All complete transactions run instructions here exactly once.

	g.createTransactionIndices(tx)

	// for child in children(tx):
	//		if child in incomplete:
	//			if complete = reduce(lambda acc, tx: acc and (parent in graph), child.parents, True):
	//				mark child as complete

	for _, childID := range g.children[tx.ID] {
		_, incomplete := g.incomplete[childID]

		if !incomplete {
			continue
		}

		child, exists := g.transactions[childID]

		if !exists {
			continue
		}

		complete := true // Complete if parents are complete, and parent transaction contents exist in graph.

		for _, parentID := range child.ParentIDs {
			if _, incomplete := g.incomplete[parentID]; incomplete {
				complete = false
				break
			}

			if _, exists := g.transactions[parentID]; !exists {
				complete = false
				break
			}
		}

		if complete {
			delete(g.incomplete, childID)
			g.markTransactionAsComplete(child)
		}
	}

	return nil
}

func (g *Graph) Reset(newRound *Round) {
	ptr := &newRound.Root

	g.transactions[newRound.Root.ID] = ptr
	g.createTransactionIndices(ptr)

	oldRoot := g.transactions[g.rootID]

	g.roundIndex[newRound.Index] = make(map[common.TransactionID]struct{})

	for i := oldRoot.Depth + 1; i <= newRound.Root.Depth; i++ {
		for id := range g.depthIndex[i] {
			g.roundIndex[newRound.Index][id] = struct{}{}
		}
	}

	g.rootID = newRound.Root.ID
}
