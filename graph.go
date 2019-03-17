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
	"go.uber.org/atomic"
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

	height atomic.Uint64

	kv store.KV

	resetCounter int
}

func newGraph(kv store.KV, genesis *Transaction) *graph {
	g := &graph{
		kv:           kv,
		transactions: make(map[common.TransactionID]*Transaction),
		children:     make(map[common.TransactionID][]common.TransactionID),
	}

	// Initialize difficulty if not exist.
	if difficulty := g.loadDifficulty(); difficulty == 0 {
		g.saveDifficulty(uint64(sys.MinDifficulty))
	}

	// Initialize genesis if not spawned.
	if genesis != nil {
		g.saveRoot(genesis)
	} else if genesis = g.loadRoot(); genesis == nil {
		panic("genesis transaction must be specified")
	}

	g.transactions[genesis.ID] = genesis

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
	}

	// Update the transactions depth.
	tx.depth = maxDepth(parents) + 1

	// Update the graphs frontier depth/height.
	if g.height.Load() < tx.depth {
		g.height.Store(tx.depth)
	}

	// Add the transaction to our view-graph.
	g.transactions[tx.ID] = tx

	logger := log.TX(tx.ID, tx.Sender, tx.Creator, tx.ParentIDs, tx.Tag, tx.Payload, "new")
	logger.Log().Uint64("depth", tx.depth).Msg("Added transaction to view-graph.")

	return nil
}

// reset resets the entire graph and sets the graph to start from
// the specified root (latest critical transaction of the entire ledger).
func (g *graph) reset(root *Transaction) {
	g.Lock()

	// Prune every 30 resets.
	g.resetCounter++

	if g.resetCounter >= 30 {
		logger := log.Consensus("prune")
		logger.Debug().
			Int("num_tx", len(g.transactions)).
			Uint64("view_id", root.ViewID+1).
			Msg("Pruned transactions.")

		g.transactions = make(map[common.TransactionID]*Transaction)
		g.children = make(map[common.TransactionID][]common.TransactionID)

		g.resetCounter = 0
	}

	g.height.Store(0)

	root.depth = 0
	g.transactions[root.ID] = root

	original, adjusted := g.loadDifficulty(), computeNextDifficulty(root)

	logger := log.Consensus("update_difficulty")
	logger.Info().
		Uint64("old_difficulty", original).
		Uint64("new_difficulty", adjusted).
		Msg("Ledger difficulty has been adjusted.")

	g.saveDifficulty(adjusted)
	g.saveViewID(root.ViewID + 1)

	g.saveRoot(root)

	g.Unlock()
}

func (g *graph) findEligibleParents() (eligible []common.TransactionID) {
	g.RLock()
	defer g.RUnlock()

	root := g.loadRoot()

	if root == nil {
		return
	}

	visited := make(map[common.TransactionID]struct{})
	visited[root.ID] = struct{}{}

	q := queue.New()
	q.PushBack(root)

	height := g.height.Load()
	viewID := g.loadViewID()

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
		} else if popped.depth+sys.MaxEligibleParentsDepthDiff >= height && (popped.ID == root.ID || popped.ViewID == viewID) {
			// All eligible parents are within the graph depth [frontier_depth - max_depth_diff, frontier_depth].
			eligible = append(eligible, popped.ID)
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

func (g *graph) loadViewID() uint64 {
	buf, err := g.kv.Get(keyLedgerViewID[:])
	if len(buf) != 8 || err != nil {
		return 0
	}

	return binary.LittleEndian.Uint64(buf)
}

func (g *graph) loadDifficulty() uint64 {
	buf, err := g.kv.Get(keyLedgerDifficulty[:])
	if len(buf) != 8 || err != nil {
		return uint64(sys.MinDifficulty)
	}

	return binary.LittleEndian.Uint64(buf)
}

func (g *graph) saveDifficulty(difficulty uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], difficulty)

	_ = g.kv.Put(keyLedgerDifficulty[:], buf[:])
}

func (g *graph) saveViewID(viewID uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], viewID)

	_ = g.kv.Put(keyLedgerViewID[:], buf[:])
}
