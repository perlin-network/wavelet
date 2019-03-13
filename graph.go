package wavelet

import (
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"sync"
)

var (
	ErrParentsNotAvailable = errors.New("parents for transaction are not in our view-graph")
	ErrTxAlreadyExists     = errors.New("transaction already exists")
)

type graph struct {
	sync.RWMutex

	transactions map[common.TransactionID]*Transaction
	children     map[common.TransactionID][]common.TransactionID
	candidates   map[common.TransactionID]struct{}

	height uint64

	kv store.KV
}

func newGraph(kv store.KV, genesis *Transaction) *graph {
	g := &graph{
		kv:           kv,
		transactions: make(map[common.TransactionID]*Transaction),
		children:     make(map[common.TransactionID][]common.TransactionID),
		candidates:   make(map[common.TransactionID]struct{}),
	}

	if genesis != nil {
		g.saveRoot(genesis)
	} else {
		genesis = g.loadRoot()
	}

	g.transactions[genesis.ID] = genesis
	g.candidates[genesis.ID] = struct{}{}

	return g
}

func maxDepth(parents []*Transaction) (max uint64) {
	for _, parent := range parents {
		if max < parent.depth {
			max = parent.depth
		}
	}

	return
}

func (g *graph) addTransaction(tx *Transaction) error {
	g.Lock()
	defer g.Unlock()

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

	// Update the parents children.
	for _, parent := range parents {
		g.children[parent.ID] = append(g.children[parent.ID], tx.ID)

		// Any parent that has children should not be an eligible candidate parent
		// for future transactions our node makes.
		delete(g.candidates, parent.ID)
	}

	// Update the transactions depth.
	tx.depth = maxDepth(parents) + 1

	// Update the graphs frontier depth/height.
	if g.height < tx.depth {
		g.height = tx.depth
	}

	// Add the transaction to our view-graph.
	g.transactions[tx.ID] = tx

	// Assumption: When a transaction is added to the view-graph, they must have
	// no children.
	//
	// Therefore, we may assume the transaction is an eligible parent candidate
	// for future transactions we create.
	g.candidates[tx.ID] = struct{}{}

	logger := log.TX(tx.ID, tx.Sender, tx.Creator, tx.ParentIDs, tx.Tag, tx.Payload, "new")
	logger.Log().Uint64("depth", tx.depth).Msg("Added transaction to view-graph.")

	return nil
}

// reset resets the entire graph and sets the graph to start from
// the specified root (latest critical transaction of the entire ledger).
func (g *graph) reset(root *Transaction) {
	g.Lock()

	g.height = 0
	//g.transactions = make(map[common.TransactionID]*Transaction)
	//g.children = make(map[common.TransactionID][]common.TransactionID)
	g.candidates = make(map[common.TransactionID]struct{})

	g.transactions[root.ID] = root
	g.candidates[root.ID] = struct{}{}

	g.saveRoot(root)

	g.Unlock()
}

func (g *graph) findEligibleParents() (eligible []common.TransactionID) {
	g.RLock()
	defer g.RUnlock()

	for candidateID := range g.candidates {
		candidate, exists := g.transactions[candidateID]

		if !exists {
			continue
		}

		// All eligible parents are within the graph depth [frontier_depth - max_depth_diff, frontier_depth].
		if candidate.depth+sys.MaxEligibleParentsDepthDiff >= g.height {
			eligible = append(eligible, candidateID)
		}
	}

	return
}

func (g *graph) lookupTransaction(id common.TransactionID) (*Transaction, bool) {
	g.RLock()
	tx, exists := g.transactions[id]
	g.RUnlock()

	return tx, exists
}

func (g *graph) saveRoot(root *Transaction) {
	_ = g.kv.Put(keyGraphRoot[:], root.Write())
}

func (g *graph) loadRoot() *Transaction {
	buf, err := g.kv.Get(keyGraphRoot[:])
	if len(buf) == 0 || err != nil {
		return nil
	}

	msg, err := Transaction{}.Read(payload.NewReader(buf))
	if err != nil {
		panic("graph: root data is malformed")
	}

	tx := msg.(Transaction)

	return &tx
}

// Root returns the current root (the latest critical transaction) of the graph.
func (g *graph) Root() *Transaction {
	g.RLock()
	root := g.loadRoot()
	g.RUnlock()

	return root
}

// Height returns the current depth of the frontier of the graph.
func (g *graph) Height() uint64 {
	g.RLock()
	height := g.height
	g.RUnlock()

	return height
}
