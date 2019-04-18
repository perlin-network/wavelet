package wavelet

import (
	"bytes"
	"encoding/binary"
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

	checksums    map[uint64]*Transaction
	transactions map[common.TransactionID]*Transaction
	children     map[common.TransactionID][]*Transaction
	viewIDs      map[uint64]map[common.TransactionID]*Transaction

	root   atomic.Value
	height atomic.Uint64

	kv store.KV
}

func newGraph(kv store.KV, genesis *Transaction) *graph {
	g := &graph{
		kv: kv,

		checksums:    make(map[uint64]*Transaction),
		transactions: make(map[common.TransactionID]*Transaction),
		children:     make(map[common.TransactionID][]*Transaction),
		viewIDs:      make(map[uint64]map[common.TransactionID]*Transaction),
	}

	// Initialize difficulty if not exist.
	if difficulty := g.loadDifficulty(); difficulty == 0 {
		g.saveDifficulty(uint64(sys.MinDifficulty))
	}

	// Initialize genesis if not spawned.
	if genesis != nil {
		g.saveRoot(genesis)
	} else {
		genesis = g.loadRoot()

		if genesis == nil {
			panic("genesis transaction must be specified")
		}
	}

	g.push(genesis)

	return g
}

// push adds a transaction forcefully into the view-graph with no checks. It is
// responsible for updating in-memory indices for a transaction as well.
func (g *graph) push(tx *Transaction) {
	// Add transaction to the view-graph.
	g.transactions[tx.ID] = tx

	// Update view ID index.
	if _, exists := g.viewIDs[tx.ViewID]; !exists {
		g.viewIDs[tx.ViewID] = make(map[common.TransactionID]*Transaction)
	}
	g.viewIDs[tx.ViewID][tx.ID] = tx

	// Update checksum index.
	g.checksums[tx.Checksum] = tx
}

// addTransaction adds a transaction to the view-graph. Note that it
// should not be invoked with malformed transaction data.
//
// For example, it must be guaranteed that:
// 1) the transaction has at least 1 parent recorded.
// 2) the transaction has no children recorded.
//
// It will throw an error however if the transaction already exists
// in the view-graph, or if the transactions parents are not
// previously recorded in the view-graph.
func (g *graph) addTransaction(tx *Transaction, critical bool) error {
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
		g.children[parent.ID] = append(g.children[parent.ID], tx)
	}

	// Update the transactions depth if the transaction is not critical.
	if !critical {
		for _, parent := range parents {
			if tx.depth < parent.depth {
				tx.depth = parent.depth
			}
		}

		tx.depth++
	}

	// Update the graphs frontier depth/height, and set of eligible transactions if the transaction
	// is eligible to be placed in our current view ID.
	if viewID := g.loadViewID(nil); tx.ViewID == viewID {
		height := g.height.Load()

		if height < tx.depth {
			height = tx.depth
			g.height.Store(height)
		}
	}

	g.push(tx)

	logger := log.TX(tx.ID, tx.Sender, tx.Creator, tx.ParentIDs, tx.Tag, tx.Payload, "new")
	logger.Log().Uint64("depth", tx.depth).Msg("Added transaction to view-graph.")

	return nil
}

const pruningDepth = 30

// reset prunes away old transactions, resets the entire graph, and sets the
// graph root to a specified transaction.
func (g *graph) reset(root *Transaction) {
	newViewID := g.loadViewID(root)

	g.Lock()
	defer g.Unlock()

	// Prune away all transactions and indices with a view ID < (current view ID - pruningDepth).
	for viewID, transactions := range g.viewIDs {
		if viewID+pruningDepth < newViewID {
			for _, tx := range transactions {
				delete(g.checksums, tx.Checksum)
				delete(g.transactions, tx.ID)
				delete(g.children, tx.ID)
			}
			delete(g.viewIDs, viewID)

			logger := log.Consensus("prune")
			logger.Debug().
				Int("num_tx", len(transactions)).
				Uint64("current_view_id", newViewID).
				Uint64("pruned_view_id", viewID).
				Msg("Pruned transactions.")
		}
	}

	g.height.Store(0)

	original, adjusted := g.loadDifficulty(), computeNextDifficulty(root)

	logger := log.Consensus("update_difficulty")
	logger.Info().
		Uint64("old_difficulty", original).
		Uint64("new_difficulty", adjusted).
		Msg("Ledger difficulty has been adjusted.")

	g.saveDifficulty(adjusted)

	root.depth = 0

	g.saveRoot(root)
	g.push(root)
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

	q := queuePool.Get().(*queue.Queue)
	defer func() {
		q.Init()
		queuePool.Put(q)
	}()

	q.PushBack(root)

	height := g.height.Load()
	viewID := g.loadViewID(root)

	for q.Len() > 0 {
		popped := q.PopFront().(*Transaction)

		if children := g.children[popped.ID]; len(children) > 0 {
			for _, child := range children {
				if _, seen := visited[child.ID]; !seen {
					q.PushBack(child)
					visited[child.ID] = struct{}{}
				}
			}
		} else if popped.depth+sys.MaxEligibleParentsDepthDiff >= height && (popped.ID == root.ID || popped.ViewID == viewID) {
			// All eligible parents are within the graph depth [frontier_depth - max_depth_diff, frontier_depth].
			eligible = append(eligible, popped.ID)
		}
	}

	return
}

func (g *graph) numTransactions(viewID uint64) int {
	g.RLock()
	num := len(g.viewIDs[viewID])
	g.RUnlock()

	return num
}

func (g *graph) lookupTransaction(id common.TransactionID) (*Transaction, bool) {
	if id == common.ZeroTransactionID {
		return nil, false
	}

	g.RLock()
	tx, exists := g.transactions[id]
	g.RUnlock()

	return tx, exists
}

func (g *graph) loadChecksums(viewID uint64) (checksums []uint64) {
	g.RLock()
	defer g.RUnlock()

	transactions, exists := g.viewIDs[viewID]

	if !exists {
		return
	}

	for _, tx := range transactions {
		checksums = append(checksums, tx.Checksum)
	}

	return checksums
}

func (g *graph) lookupTransactionByChecksum(checksum uint64) (*Transaction, bool) {
	g.RLock()
	tx, exists := g.checksums[checksum]
	g.RUnlock()

	return tx, exists
}

func (g *graph) loadHeight() uint64 {
	return g.height.Load()
}

func (g *graph) saveRoot(root *Transaction) {
	g.root.Store(root)
	_ = g.kv.Put(keyGraphRoot[:], root.Marshal())
}

func (g *graph) loadRoot() *Transaction {
	if root := g.root.Load(); root != nil {
		return root.(*Transaction)
	}

	buf, err := g.kv.Get(keyGraphRoot[:])
	if len(buf) == 0 || err != nil {
		return nil
	}

	tx, err := UnmarshalTransaction(bytes.NewReader(buf))
	if err != nil {
		panic("graph: root data is malformed")
	}

	root := &tx
	g.root.Store(root)

	return root
}

func (g *graph) loadViewID(root *Transaction) uint64 {
	if root == nil {
		root = g.loadRoot()
	}

	return root.ViewID + 1
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
