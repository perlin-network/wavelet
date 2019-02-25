package wavelet

import (
	"github.com/perlin-network/wavelet/sys"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

var (
	ErrParentsNotAvailable = errors.New("graph: do not have parents available")
	ErrTxAlreadyExists     = errors.New("graph: transaction already exists")
)

type graph struct {
	transactions map[[blake2b.Size256]byte]*Transaction

	root   *Transaction // The current root (the latest critical transaction) of the graph.
	height *uint64      // The current depth of the frontier of the graph.
}

func newGraph(root *Transaction) graph {
	return graph{
		transactions: map[[blake2b.Size256]byte]*Transaction{root.ID: root},
		root:         root,

		height: new(uint64),
	}
}

func (g graph) addTransaction(tx *Transaction) error {
	// Return an error if the transaction is already inside the graph.
	if _, stored := g.transactions[tx.ID]; stored {
		return ErrTxAlreadyExists
	}

	var parents []*Transaction

	// Look in the graph for the transactions parents.
	for _, parentID := range tx.ParentIDs {
		if parent, stored := g.transactions[parentID]; stored {
			parents = append(parents, parent)
		} else {
			return ErrParentsNotAvailable
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

	if *g.height < tx.depth {
		*g.height = tx.depth
	}

	g.transactions[tx.ID] = tx

	// Update the parents children.
	for _, parent := range parents {
		parent.children = append(parent.children, tx.ID)
	}

	return nil
}

// reset resets the entire graph and sets the graph to start from
// the specified root (latest critical transaction of the entire ledger).
func (g graph) reset(root *Transaction) {
	g.transactions = map[[blake2b.Size256]byte]*Transaction{root.ID: root}

	g.root = root
	*g.height = 0
}

func (g graph) findEligibleParents() (eligible [][blake2b.Size256]byte) {
	visited := make(map[[blake2b.Size256]byte]struct{})
	queue := queue.New()

	queue.PushBack(g.root)

	for queue.Len() > 0 {
		popped := queue.PopFront().(*Transaction)

		if len(popped.children) > 0 {
			for _, childrenID := range popped.children {
				if _, seen := visited[childrenID]; !seen {
					queue.PushBack(g.transactions[childrenID])
				}
			}
		} else if popped.depth+sys.MaxEligibleParentsDepthDiff >= *g.height {
			// All eligible parents are within the graph depth [frontier_depth - max_depth_diff, frontier_depth].
			eligible = append(eligible, popped.ID)
		}

		visited[popped.ID] = struct{}{}
	}

	return
}
