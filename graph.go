package wavelet

import (
	"github.com/perlin-network/wavelet/sys"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

var (
	ErrTxNotExist      = errors.New("graph: transaction does not exist")
	ErrTxAlreadyExists = errors.New("graph: transaction already exists")
)

type graph struct {
	transactions map[[blake2b.Size256]byte]*Transaction

	criticalTransactionsWindow      [sys.MaxHistoryWindowSize]*Transaction
	criticalTransactionsWindowIndex int

	// The current depth of the frontier of the graph.
	frontierDepth uint64
}

func newGraph() *graph {
	return &graph{
		transactions: make(map[[blake2b.Size256]byte]*Transaction),
	}
}

func (g *graph) clearTransactions() {
	g.transactions = make(map[[blake2b.Size256]byte]*Transaction)
}

func (g *graph) addTransaction(tx *Transaction) error {
	// Return an error if the transaction is already inside the graph.
	if _, stored := g.transactions[tx.id]; stored {
		return ErrTxAlreadyExists
	}

	var parents []*Transaction

	// Look in the graph for the transactions parents.
	for _, parentID := range tx.parentIDs {
		if parent, stored := g.transactions[parentID]; stored {
			parents = append(parents, parent)
		} else {
			return errors.Wrap(ErrTxNotExist, "could not find transactions parents")
		}
	}

	// Update transaction and graph frontiers depth.
	maxParentsDepth := parents[0].depth
	for _, parent := range parents[1:] {
		if maxParentsDepth < parent.depth {
			maxParentsDepth = parent.depth
		}
	}

	tx.depth = maxParentsDepth + 1

	if g.frontierDepth < tx.depth {
		g.frontierDepth = tx.depth
	}

	g.transactions[tx.id] = tx

	// Update the parents children.
	for _, parent := range parents {
		parent.children = append(parent.children, tx.id)
	}

	return nil
}

func (g *graph) findEligibleParents() (eligible [][blake2b.Size256]byte) {
	visited := make(map[[blake2b.Size256]byte]struct{})
	queue := queue.New()

	queue.PushBack(g.criticalTransactionsWindow[g.criticalTransactionsWindowIndex])

	for queue.Len() > 0 {
		popped := queue.PopFront().(*Transaction)

		if len(popped.children) > 0 {
			for _, childrenID := range popped.children {
				if _, seen := visited[childrenID]; !seen {
					queue.PushBack(g.transactions[childrenID])
				}
			}
		} else if popped.depth >= g.frontierDepth-sys.MaxEligibleParentsDepthDiff {
			// All eligible parents are within the graph depth [frontier_depth - max_depth_diff, frontier_depth].
			eligible = append(eligible, popped.id)
		}

		visited[popped.id] = struct{}{}
	}

	return
}
