package wavelet

import (
	"encoding/binary"
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"sort"
	"sync"
)

var (
	ErrParentsNotAvailable = errors.New("graph: do not have parents available")
	ErrTxAlreadyExists     = errors.New("graph: transaction already exists")
)

type graph struct {
	sync.Mutex

	transactions map[common.TransactionID]*Transaction
	children     map[common.TransactionID][]common.TransactionID

	kv store.KV
}

func newGraph(kv store.KV, genesis *Transaction) *graph {
	g := &graph{
		kv:           kv,
		transactions: make(map[common.TransactionID]*Transaction),
		children:     make(map[common.TransactionID][]common.TransactionID),
	}

	if genesis != nil {
		g.saveHeight(0)
		g.saveRoot(genesis)
	} else {
		genesis = g.loadRoot()
	}

	g.transactions[genesis.ID] = genesis

	return g
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

	// Update transaction and graph frontiers depth.
	maxParentsDepth := parents[0].depth
	for _, parent := range parents[1:] {
		if maxParentsDepth < parent.depth {
			maxParentsDepth = parent.depth
		}
	}

	tx.depth = maxParentsDepth + 1

	// Update the graphs height.
	if g.loadHeight() < tx.depth {
		g.saveHeight(tx.depth)
	}

	// Update the parents children.
	for _, parent := range parents {
		g.children[parent.ID] = append(g.children[parent.ID], tx.ID)
	}

	g.transactions[tx.ID] = tx

	logger := log.TX(tx.ID, tx.Sender, tx.Creator, tx.ParentIDs, tx.Tag, tx.Payload, "new")
	logger.Log().Uint64("depth", tx.depth).Msg("Added transaction to view-graph.")

	return nil
}

// reset resets the entire graph and sets the graph to start from
// the specified root (latest critical transaction of the entire ledger).
func (g *graph) reset(root *Transaction) {
	g.Lock()
	defer g.Unlock()

	g.saveRoot(root)

	// Prune away all of roots ancestors.
	visited := make(map[common.TransactionID]struct{})
	q := queue.New()

	for _, parentID := range root.ParentIDs {
		if parent, exists := g.transactions[parentID]; exists {
			q.PushBack(parent)
		}

		visited[parentID] = struct{}{}
	}

	count := 0

	for q.Len() > 0 {
		popped := q.PopFront().(*Transaction)

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; !seen {
				if parent, exists := g.transactions[parentID]; exists {
					q.PushBack(parent)
				}
				visited[parentID] = struct{}{}
			}
		}

		delete(g.transactions, popped.ID)
		delete(g.children, popped.ID)
		count++

		logger := log.TX(popped.ID, popped.Sender, popped.Creator, popped.ParentIDs, popped.Tag, popped.Payload, "pruned")
		logger.Log().Msg("Pruned transaction away from view-graph.")
	}

	logger := log.Consensus("prune")
	logger.Info().Int("count", count).Msg("Pruned away transactions from the view-graph.")
}

func (g *graph) findEligibleParents() (eligible []common.TransactionID) {
	g.Lock()
	defer g.Unlock()

	root := g.loadRoot()
	height := g.loadHeight()

	if root == nil {
		return
	}

	visited := make(map[common.TransactionID]struct{})
	visited[root.ID] = struct{}{}

	q := queue.New()
	q.PushBack(root)

	for q.Len() > 0 {
		popped := q.PopFront().(*Transaction)

		if children := g.children[popped.ID]; len(children) > 0 {
			for _, childrenID := range children {
				if _, seen := visited[childrenID]; !seen {
					if child, exists := g.transactions[childrenID]; exists {
						q.PushBack(child)
					}
					visited[childrenID] = struct{}{}
				}
			}
		} else if popped.depth+sys.MaxEligibleParentsDepthDiff >= height {
			// All eligible parents are within the graph depth [frontier_depth - max_depth_diff, frontier_depth].
			eligible = append(eligible, popped.ID)
		}
	}

	return
}

func (g *graph) lookupTransaction(id common.TransactionID) (*Transaction, bool) {
	g.Lock()
	defer g.Unlock()

	tx, exists := g.transactions[id]
	return tx, exists
}

func (g *graph) saveRoot(root *Transaction) {
	_ = g.kv.Put(keyGraphRoot[:], root.Write())
}

func (g *graph) saveHeight(height uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], height)

	_ = g.kv.Put(keyGraphHeight[:], buf[:])
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
	tx.depth = g.loadHeight()

	return &tx
}

func (g *graph) loadHeight() uint64 {
	buf, err := g.kv.Get(keyGraphHeight[:])
	if len(buf) != 8 || err != nil {
		return 0
	}

	return binary.LittleEndian.Uint64(buf)
}

// Root returns the current root (the latest critical transaction) of the graph.
func (g *graph) Root() *Transaction {
	g.Lock()
	defer g.Unlock()

	return g.loadRoot()
}

// Height returns the current depth of the frontier of the graph.
func (g *graph) Height() uint64 {
	g.Lock()
	defer g.Unlock()

	return g.loadHeight()
}

func (g *graph) Transactions(offset, limit uint64, sender, creator common.AccountID) (transactions []*Transaction) {
	g.Lock()

	for _, tx := range g.transactions {
		if (sender == common.ZeroAccountID && creator == common.ZeroAccountID) || (sender != common.ZeroAccountID && tx.Sender == sender) || (creator != common.ZeroAccountID && tx.Creator == creator) {
			transactions = append(transactions, tx)
		}
	}

	g.Unlock()

	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].depth < transactions[j].depth
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
