package wavelet

import (
	"github.com/perlin-network/wavelet/sys"
	"github.com/phf/go-queue/queue"
	"golang.org/x/crypto/blake2b"
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
