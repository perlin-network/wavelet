package wavelet

import (
	"encoding/binary"
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
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

	transactions    map[common.TransactionID]*Transaction
	children        map[common.TransactionID][]*Transaction
	eligibleParents map[common.TransactionID]*Transaction

	indexViewID map[uint64][]*Transaction

	height atomic.Uint64

	kv store.KV
}

func newGraph(kv store.KV, genesis *Transaction) *graph {
	g := &graph{
		kv:              kv,
		transactions:    make(map[common.TransactionID]*Transaction),
		children:        make(map[common.TransactionID][]*Transaction),
		eligibleParents: make(map[common.TransactionID]*Transaction),
		indexViewID:     make(map[uint64][]*Transaction),
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
	g.updateIndices(genesis)
	g.eligibleParents[genesis.ID] = genesis

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

func (g *graph) updateIndices(tx *Transaction) {
	g.indexViewID[tx.ViewID] = append(g.indexViewID[tx.ViewID], tx)
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
		g.children[parent.ID] = append(g.children[parent.ID], tx)

		// The parent is no longer eligible
		delete(g.eligibleParents, parent.ID)
	}

	// Update the transactions depth.
	tx.depth = maxDepth(parents) + 1

	// Update the graphs frontier depth/height and update the eligible parents
	height := g.height.Load()
	if height < tx.depth {
		g.height.Store(tx.depth)
		height = tx.depth

		// Since the height has been updated, check each eligible parents' height
		for _, v := range g.eligibleParents {
			if v.depth+sys.MaxEligibleParentsDepthDiff < height {
				delete(g.eligibleParents, v.ID)
			}
		}
	}

	// Add the transaction to the eligible parents if it's qualified
	root := g.loadRoot()
	if root != nil && len(g.children[tx.ID]) == 0 {
		viewID := g.loadViewID(root)

		if tx.depth+sys.MaxEligibleParentsDepthDiff >= height && tx.ViewID == viewID {
			g.eligibleParents[tx.ID] = tx
		}
	}

	// Add the transaction to our view-graph.
	g.transactions[tx.ID] = tx
	g.updateIndices(tx)

	logger := log.TX(tx.ID, tx.Sender, tx.Creator, tx.ParentIDs, tx.Tag, tx.Payload, "new")
	logger.Log().Uint64("depth", tx.depth).Msg("Added transaction to view-graph.")

	return nil
}

const pruningDepth = 30

// reset resets the entire graph and sets the graph to start from
// the specified root (latest critical transaction of the entire ledger).
func (g *graph) reset(root *Transaction) {
	newViewID := root.ViewID + 1

	g.Lock()
	defer g.Unlock()

	// Prune away all transactions and indices with a view ID < (current view ID - pruningDepth).
	for viewID, transactions := range g.indexViewID {
		if viewID+pruningDepth < newViewID {
			for _, tx := range transactions {
				delete(g.transactions, tx.ID)
				delete(g.children, tx.ID)
				delete(g.eligibleParents, tx.ID)
			}

			logger := log.Consensus("prune")
			logger.Debug().
				Int("num_tx", len(transactions)).
				Uint64("current_view_id", newViewID).
				Uint64("pruned_view_id", viewID).
				Msg("Pruned transactions.")

			delete(g.indexViewID, viewID)
		}
	}

	g.height.Store(0)

	root.depth = 0

	_, existed := g.transactions[root.ID]

	g.transactions[root.ID] = root

	if !existed {
		g.updateIndices(root)
	}

	original, adjusted := g.loadDifficulty(), computeNextDifficulty(root)

	logger := log.Consensus("update_difficulty")
	logger.Info().
		Uint64("old_difficulty", original).
		Uint64("new_difficulty", adjusted).
		Msg("Ledger difficulty has been adjusted.")

	g.saveDifficulty(adjusted)
	g.saveRoot(root)
}

func (g *graph) findEligibleParents() (eligible []common.TransactionID) {
	g.RLock()
	defer g.RUnlock()

	for id := range g.eligibleParents {
		eligible = append(eligible, id)
	}

	return
}

func (g *graph) numTransactions(viewID uint64) int {
	g.RLock()
	num := len(g.indexViewID[viewID])
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
