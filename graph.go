package wavelet

import (
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
)

type Round struct {
	root         *Transaction
	transactions map[common.TransactionID]struct{}
}

type Graph struct {
	rounds map[uint64]Round

	transactions map[common.TransactionID]*Transaction           // All transactions.
	children     map[common.TransactionID][]common.TransactionID // Children of transactions.

	incomplete map[common.TransactionID]struct{} // Transactions that don't have all parents available.
	missing    map[common.TransactionID]struct{} // Transactions that we are missing.
}

func NewGraph() *Graph {
	g := &Graph{
		rounds: make(map[uint64]Round),

		transactions: make(map[common.TransactionID]*Transaction),
		children:     make(map[common.TransactionID][]common.TransactionID),

		incomplete: make(map[common.TransactionID]struct{}),
		missing:    make(map[common.TransactionID]struct{}),
	}

	g.transactions[common.ZeroTransactionID] = &Transaction{}

	return g
}

func (g *Graph) assertTransactionIsValid(tx *Transaction) error {
	if len(tx.ParentIDs) == 0 {
		return errors.New("transaction has no parents")
	}

	return nil
}

func (g *Graph) assertTransactionIsComplete(tx *Transaction) error {
	// Check that the transaction's depth is correct according to its parents.
	var maxDepth uint64
	var expectedConfidence uint64

	for _, parentID := range tx.ParentIDs {
		parent, exists := g.transactions[parentID]

		if !exists {
			return errors.New("parent not stored in graph")
		}

		// Update max depth witnessed from parents.
		if maxDepth < parent.Depth {
			maxDepth = parent.Depth
		}

		expectedConfidence += parent.Confidence + 1
	}

	maxDepth++

	if tx.Depth != maxDepth {
		return errors.Errorf("transactions depth is invalid, expected depth to be %d but got %d", maxDepth, tx.Depth)
	}

	if tx.Confidence != expectedConfidence {
		return errors.Errorf("transactions confidence is invalid, expected confidence to be %d but got %d", expectedConfidence, tx.Confidence)
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

func (g *Graph) deleteTransaction(id common.TransactionID) {
	delete(g.transactions, id)
	delete(g.incomplete, id)
	delete(g.missing, id)

	for _, childID := range g.children[id] {
		g.deleteTransaction(childID)
	}
}

func (g *Graph) markTransactionAsComplete(tx *Transaction) error {
	err := g.assertTransactionIsComplete(tx)

	if err != nil {
		g.deleteTransaction(tx.ID)
		return err
	}

	// All complete transactions run instructions here

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

// txs: 1, 2, 3, 4, 5

// txs: 3, 4, 2, 1, 5

// incomplete = {}
