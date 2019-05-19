package wavelet

import (
	"github.com/google/btree"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"sort"
	"sync"
)

type Graph struct {
	sync.RWMutex

	transactions map[common.TransactionID]*Transaction           // All transactions.
	children     map[common.TransactionID][]common.TransactionID // Children of transactions.

	eligible   map[common.TransactionID]struct{} // Transactions that are eligible to be parent transactions.
	incomplete map[common.TransactionID]struct{} // Transactions that don't have all parents available.

	missing map[common.TransactionID]struct{} // Transactions that we are missing.

	seedIndex *btree.BTree // Indexes transactions by their seed.

	depthIndex map[uint64]map[common.TransactionID]struct{} // Indexes transactions by their depth.
	roundIndex map[uint64]map[common.TransactionID]struct{} // Indexes transactions by their round.

	rootID common.TransactionID // Root of the graph.
	height uint64               // Height of the graph.
}

type SeedIndexItem struct {
	tx *Transaction
}

func (item SeedIndexItem) Less(_that btree.Item) bool {
	that := _that.(SeedIndexItem)
	if item.tx.Depth == that.tx.Depth {
		return item.tx.SeedLen < that.tx.SeedLen
	}
	return item.tx.Depth < that.tx.Depth
}

func NewGraph(genesis *Round) *Graph {
	g := &Graph{
		transactions: make(map[common.TransactionID]*Transaction),
		children:     make(map[common.TransactionID][]common.TransactionID),

		eligible:   make(map[common.TransactionID]struct{}),
		incomplete: make(map[common.TransactionID]struct{}),

		missing: make(map[common.TransactionID]struct{}),

		seedIndex: btree.New(2),

		depthIndex: make(map[uint64]map[common.TransactionID]struct{}),
		roundIndex: make(map[uint64]map[common.TransactionID]struct{}),

		height: 1,
	}

	if genesis != nil {
		g.rootID = genesis.End.ID
		g.transactions[genesis.End.ID] = &genesis.End
	} else {
		ptr := new(Transaction)

		g.rootID = ptr.ID
		g.transactions[ptr.ID] = ptr
	}

	root := g.transactions[g.rootID]

	g.height = root.Depth + 1
	g.createTransactionIndices(root)

	return g
}

func (g *Graph) assertTransactionIsComplete(tx *Transaction) error {
	// Check that the transaction's depth is correct according to its parents.
	var maxDepth uint64

	for _, parentID := range tx.ParentIDs {
		parent, exists := g.lookupTransactionByID(parentID)

		if !exists {
			return errors.New("parent not stored in graph")
		}

		// Check if the depth of each parents is acceptable.
		if parent.Depth+sys.MaxDepthDiff < tx.Depth {
			return errors.Errorf("tx parents exceeds max eligible parents depth diff: parents depth is %d, but tx depth is %d", parent.Depth, tx.Depth)
		}

		// Update max depth witnessed from parents.
		if maxDepth < parent.Depth {
			maxDepth = parent.Depth
		}
	}

	maxDepth++

	if tx.Depth != maxDepth {
		return errors.Errorf("transactions depth is invalid, expected depth to be %d but got %d", maxDepth, tx.Depth)
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

		//delete(g.eligible, parentID)
	}

	return missingParentIDs
}

func (g *Graph) GetTransaction(id common.TransactionID) *Transaction {
	g.RLock()
	defer g.RUnlock()

	return g.transactions[id]
}

func (g *Graph) LookupTransactionByID(id common.TransactionID) (*Transaction, bool) {
	g.Lock()
	defer g.Unlock()

	return g.lookupTransactionByID(id)
}

func (g *Graph) lookupTransactionByID(id common.TransactionID) (*Transaction, bool) {
	if _, missing := g.missing[id]; missing {
		return nil, false
	}

	tx, exists := g.transactions[id]

	if !exists {
		if _, missing := g.missing[id]; !missing {
			g.missing[id] = struct{}{}
		}
	}

	return tx, exists
}

var (
	ErrMissingParents = errors.New("parents for transaction are not in graph")
	ErrAlreadyExists  = errors.New("transaction already exists in the graph")
)

func (g *Graph) AddTransaction(tx Transaction) error {
	g.Lock()
	defer g.Unlock()

	if _, exists := g.transactions[tx.ID]; exists {
		return ErrAlreadyExists
	}

	ptr := &tx

	// Add transaction to the view-graph.
	g.transactions[tx.ID] = ptr

	delete(g.missing, ptr.ID)

	missing := g.processParents(ptr)

	if len(missing) > 0 {
		g.incomplete[ptr.ID] = struct{}{}
		return ErrMissingParents
	}

	return g.markTransactionAsComplete(ptr)
}

func (g *Graph) DeleteTransaction(id common.TransactionID) {
	g.Lock()
	defer g.Unlock()

	g.deleteTransaction(id)
}

// deleteTransaction deletes all traces of a transaction from the graph. Note
// however that it does not remove the transaction from any of the graphs
// indices.
func (g *Graph) deleteTransaction(id common.TransactionID) {
	if tx, exists := g.transactions[id]; exists {
		g.seedIndex.Delete(SeedIndexItem{tx: tx})

		delete(g.depthIndex[tx.Depth], id)

		if len(g.depthIndex[tx.Depth]) == 0 {
			delete(g.depthIndex, tx.Depth)
		}
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
	g.seedIndex.ReplaceOrInsert(SeedIndexItem{tx: tx})

	if _, exists := g.depthIndex[tx.Depth]; !exists {
		g.depthIndex[tx.Depth] = make(map[common.TransactionID]struct{})
	}

	g.depthIndex[tx.Depth][tx.ID] = struct{}{}

	if g.height < tx.Depth {
		g.height = tx.Depth + 1
	}

	if _, exists := g.children[tx.ID]; !exists {
		if tx.Depth+sys.MaxDepthDiff >= g.height {
			g.eligible[tx.ID] = struct{}{}
		}
	}
}

func (g *Graph) FindEligibleParents() []common.TransactionID {
	g.Lock()
	defer g.Unlock()

	root := g.transactions[g.rootID]

	var eligibleParents []*Transaction

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

		if eligibleParent.Depth+sys.MaxDepthDiff <= g.height {
			delete(g.eligible, eligibleID)
			continue
		}

		eligibleParents = append(eligibleParents, eligibleParent)
	}

	sort.Slice(eligibleParents, func(i, j int) bool {
		return eligibleParents[i].Depth > eligibleParents[j].Depth
	})

	eligibleIDs := make([]common.TransactionID, 0, sys.MaxParentsPerTransaction)

	for _, eligibleParent := range eligibleParents {
		eligibleIDs = append(eligibleIDs, eligibleParent.ID)

		if len(eligibleIDs) == sys.MaxParentsPerTransaction {
			break
		}
	}

	return eligibleIDs
}

func (g *Graph) FindEligibleCritical(rootDepth uint64, difficulty byte) *Transaction {
	g.Lock()
	defer g.Unlock()

	var selected *Transaction
	removalList := make([]SeedIndexItem, 0)
	//last := time.Now()
	//defer func() {
	//	fmt.Println(len(g.seedIndex), candidateIndex, time.Now().Sub(last).String())
	//}()

	g.seedIndex.Ascend(func(_candidate btree.Item) bool {
		candidate := _candidate.(SeedIndexItem)
		tx := candidate.tx
		removalList = append(removalList, candidate)

		if tx.Depth <= rootDepth {
			return true
		}
		if !tx.IsCritical(difficulty) {
			return true
		}
		selected = tx
		return false
	})

	if selected == nil {
		g.seedIndex = btree.New(2)
		return nil
	}

	for _, item := range removalList {
		g.seedIndex.Delete(item)
	}

	return selected
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
	g.Lock()
	defer g.Unlock()

	ptr := &newRound.End

	g.transactions[newRound.End.ID] = ptr
	g.createTransactionIndices(ptr)

	oldRoot := g.transactions[g.rootID]

	g.roundIndex[newRound.Index] = make(map[common.TransactionID]struct{})

	for i := oldRoot.Depth + 1; i <= newRound.End.Depth; i++ {
		for id := range g.depthIndex[i] {
			g.roundIndex[newRound.Index][id] = struct{}{}
		}
	}

	g.rootID = newRound.End.ID
}

func (g *Graph) TransactionApplied(id common.TransactionID) bool {
	g.RLock()
	defer g.RUnlock()

	for _, round := range g.roundIndex {
		if _, exists := round[id]; exists {
			return true
		}
	}

	return false
}

func (g *Graph) ListTransactions(offset, limit uint64, sender, creator common.AccountID) (transactions []*Transaction) {
	g.RLock()
	defer g.RUnlock()

	for _, tx := range g.transactions {
		if (sender == common.ZeroAccountID && creator == common.ZeroAccountID) || (sender != common.ZeroAccountID && tx.Sender == sender) || (creator != common.ZeroAccountID && tx.Creator == creator) {
			transactions = append(transactions, tx)
		}
	}

	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].Depth < transactions[j].Depth
	})

	if offset != 0 || limit != 0 {
		if offset >= limit || offset >= uint64(len(transactions)) {
			return nil
		}

		if offset+limit > uint64(len(transactions)) {
			limit = uint64(len(transactions)) - offset
		}

		transactions = transactions[offset : offset+limit]
	}

	return
}

func (g *Graph) MissingTransactions() []common.TransactionID {
	g.RLock()
	defer g.RUnlock()

	var missing []common.TransactionID

	for id := range g.missing {
		missing = append(missing, id)
	}

	return missing
}

func (g *Graph) NumTransactionsInDepth(depth uint64) uint64 {
	g.RLock()
	defer g.RUnlock()

	return uint64(len(g.depthIndex[depth]))
}

func (g *Graph) NumTransactionsInStore() uint64 {
	g.RLock()
	defer g.RUnlock()

	return uint64(len(g.transactions))
}

func (g *Graph) NumMissingTransactions() uint64 {
	g.RLock()
	defer g.RUnlock()

	return uint64(len(g.missing))
}

func (g *Graph) Height() uint64 {
	g.RLock()
	defer g.RUnlock()

	return g.height
}
