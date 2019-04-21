package wavelet

import (
	"bytes"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
)

type Graph struct {
	transactions map[common.TransactionID]*Transaction           // All transactions.
	children     map[common.TransactionID][]common.TransactionID // Children of transactions.

	eligible   map[common.TransactionID]struct{} // Transactions that are eligible to be parent transactions.
	incomplete map[common.TransactionID]struct{} // Transactions that don't have all parents available.
	missing    map[common.TransactionID]struct{} // Transactions that we are missing.

	seedIndex  map[byte]map[common.TransactionID]struct{}   // Indexes transactions by their seed.
	depthIndex map[uint64]map[common.TransactionID]struct{} // Indexes transactions by their depth.
	roundIndex map[uint64]map[common.TransactionID]struct{} // Indexes transactions by their round.

	rootID common.TransactionID // Root of the graph.
	height uint64               // Height of the graph.
}

func NewGraph(genesis *Round) *Graph {
	g := &Graph{
		transactions: make(map[common.TransactionID]*Transaction),
		children:     make(map[common.TransactionID][]common.TransactionID),

		eligible:   make(map[common.TransactionID]struct{}),
		incomplete: make(map[common.TransactionID]struct{}),
		missing:    make(map[common.TransactionID]struct{}),

		seedIndex:  make(map[byte]map[common.TransactionID]struct{}),
		depthIndex: make(map[uint64]map[common.TransactionID]struct{}),
		roundIndex: make(map[uint64]map[common.TransactionID]struct{}),

		height: 1,
	}

	if genesis != nil {
		g.rootID = genesis.Root.ID
		g.transactions[genesis.Root.ID] = &genesis.Root
	} else {
		g.rootID = common.ZeroTransactionID
		g.transactions[common.ZeroTransactionID] = new(Transaction)
	}

	g.eligible[g.rootID] = struct{}{}

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

	// TODO(kenta): validate signature

	return nil
}

func (g *Graph) assertTransactionIsComplete(tx *Transaction) error {
	// Check that the transaction's depth is correct according to its parents.
	var maxDepth uint64
	var maxConfidence uint64

	for _, parentID := range tx.ParentIDs {
		parent, exists := g.transactions[parentID]

		if !exists {
			return errors.New("parent not stored in graph")
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
		_, exists := g.transactions[parentID]

		if !exists {
			g.missing[parentID] = struct{}{}
		}

		_, incomplete := g.incomplete[parentID]

		if !exists || incomplete {
			missingParentIDs = append(missingParentIDs, parentID)
		}

		g.children[parentID] = append(g.children[parentID], tx.ID)

		delete(g.eligible, parentID)
	}

	return missingParentIDs
}

func (g *Graph) addTransaction(tx Transaction) error {
	if _, exists := g.transactions[tx.ID]; exists {
		return nil
	}

	ptr := &tx

	if err := g.assertTransactionIsValid(ptr); err != nil {
		return err
	}

	// Add transaction to the view-graph.
	g.transactions[tx.ID] = ptr
	delete(g.missing, ptr.ID)

	missing := g.processParents(ptr)

	if len(missing) > 0 {
		g.incomplete[ptr.ID] = struct{}{}
		return errors.New("parents for transaction are not in graph")
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
	}

	delete(g.transactions, id)
	delete(g.children, id)

	delete(g.eligible, id)
	delete(g.incomplete, id)
	delete(g.missing, id)
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
	var eligibleIDs []common.TransactionID

	for id := range g.eligible {
		tx, exists := g.transactions[id]

		if !exists {
			delete(g.eligible, id)
			continue
		}

		if tx.Depth+sys.MaxEligibleParentsDepthDiff < g.height {
			delete(g.eligible, id)
			continue
		}

		eligibleIDs = append(eligibleIDs, id)
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

const pruningDepth = 30

func (g *Graph) Reset(newRound *Round) {
	oldRoot := g.transactions[g.rootID]

	g.roundIndex[newRound.Index] = make(map[common.TransactionID]struct{})

	for i := oldRoot.Depth + 1; i <= newRound.Root.Depth; i++ {
		for id := range g.depthIndex[i] {
			g.roundIndex[newRound.Index][id] = struct{}{}
		}
	}

	// Prune away all transactions and indices with a view ID < (current view ID - pruningDepth).

	for roundID, transactions := range g.roundIndex {
		if roundID+pruningDepth < newRound.Index {
			for id := range transactions {
				g.deleteTransaction(id)
			}

			delete(g.roundIndex, roundID)

			logger := log.Consensus("prune")
			logger.Debug().
				Int("num_tx", len(g.roundIndex[newRound.Index])).
				Uint64("current_round_id", newRound.Index).
				Uint64("pruned_round_id", roundID).
				Msg("Pruned transactions.")
		}
	}

	g.rootID = newRound.Root.ID
}
