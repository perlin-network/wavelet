package wavelet

import (
	"github.com/perlin-network/wavelet/common"
	"os"
	"testing"
	"time"

	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
)

func TestGraph(t *testing.T) {
	g, genesis, cleanup, err := createGraph(0, false)
	defer cleanup()
	if !assert.NoError(t, err) {
		return
	}

	tx, err := createTx(genesis.ViewID)
	if !assert.NoError(t, err) {
		return
	}

	eligibleParents := g.findEligibleParents()
	assert.Equal(t, 1, len(eligibleParents))
	assert.Equal(t, genesis.ID, eligibleParents[0])

	assert.NoError(t, g.addTransaction(tx, false))

	// Test transaction already exist
	err = g.addTransaction(tx, false)
	assert.Error(t, err)
	assert.Equal(t, ErrTxAlreadyExists, err)

	assert.Equal(t, uint64(0), g.height.Load())
	assert.Equal(t, genesis.ID, g.loadRoot().ID)

	_, found := g.lookupTransaction(tx.ID)
	assert.True(t, found)

	// Checked for undefined behavior adding a transaction with no parents
	// to the view-graph.

	eligibleParents = g.findEligibleParents()
	assert.Equal(t, 1, len(eligibleParents))
	assert.Equal(t, genesis.ID, eligibleParents[0])

	rootTx, err := createTx(0)
	if !assert.NoError(t, err) {
		return
	}

	g.saveRoot(rootTx)
	assert.Equal(t, rootTx.ID, g.loadRoot().ID)
}

func TestGraphReset(t *testing.T) {
	g, genesis, cleanup, err := createGraph(100, true)
	defer cleanup()
	if !assert.NoError(t, err) {
		return
	}

	assert.NotEmpty(t, g.eligibleParents)

	resetTx, err := createTx(genesis.ViewID + pruningDepth + 1)
	if !assert.NoError(t, err) {
		return
	}

	// Reset
	g.reset(resetTx)
	assert.Equal(t, resetTx.ID, g.loadRoot().ID)

	_, found := g.lookupTransaction(genesis.ID)
	assert.False(t, found)

	_, found = g.lookupTransaction(resetTx.ID)
	assert.True(t, found)

	assert.Equal(t, 1, len(g.transactions))

	eligibleParents := g.findEligibleParents()
	if !assert.Equal(t, 1, len(eligibleParents)) {
		return
	}
	assert.Equal(t, resetTx.ID, eligibleParents[0])

	assert.Equal(t, uint64(0), g.height.Load())

	assert.Equal(t, 0, g.numTransactions(g.loadViewID(genesis)))

	assert.NoError(t, addTxs(g, 100, true))
}

func TestAddTransactionWithParents(t *testing.T) {
	g, genesis, cleanup, err := createGraph(0, false)
	defer cleanup()
	if !assert.NoError(t, err) {
		return
	}

	tx, err := createTx(genesis.ViewID)
	if !assert.NoError(t, err) {
		return
	}

	tx.ParentIDs = []common.TransactionID{genesis.ID}
	assert.NoError(t, g.addTransaction(tx, false))

	// adding tx with non existing parents should result in error
	tx, err = createTx(genesis.ViewID)
	if !assert.NoError(t, err) {
		return
	}

	tx.ParentIDs = []common.TransactionID{{}}
	assert.Equal(t, ErrParentsNotAvailable, g.addTransaction(tx, false))
}

func TestLookupTransaction(t *testing.T) {
	g, genesis, cleanup, err := createGraph(100, false)
	defer cleanup()
	if !assert.NoError(t, err) {
		return
	}

	_, exists := g.lookupTransaction(genesis.ID)
	assert.True(t, exists)

	tx, exists := g.lookupTransaction(common.ZeroTransactionID)
	assert.Nil(t, tx)
	assert.False(t, exists)
}

func TestDefaultDifficulty(t *testing.T) {
	g, genesis, cleanup, err := createGraph(0, false)
	defer cleanup()
	if !assert.NoError(t, err) {
		return
	}

	g.saveDifficulty(0)
	assert.Equal(t, uint64(0), g.loadDifficulty())

	// new graph using same store should return default difficulty
	g = newGraph(g.kv, genesis)
	assert.Equal(t, uint64(sys.MinDifficulty), g.loadDifficulty())
}

func TestLoadRoot(t *testing.T) {
	g, _, cleanup, err := createGraph(100, true)
	defer cleanup()
	if !assert.NoError(t, err) {
		return
	}

	assert.NotNil(t, g.loadRoot())

	// ensure that root will be loaded from storage
	g = newGraph(g.kv, nil)
	assert.NotNil(t, g.loadRoot())
}

func benchmarkGraph(b *testing.B, n int) {
	for i := 0; i < b.N; i++ {
		g, genesis, cleanup, err := createGraph(n, true)
		if err != nil {
			b.Fatal(err)
		}

		newRoot, err := createTx(genesis.ViewID + 1)
		if err != nil {
			b.Fatal(err)
		}

		g.reset(newRoot)

		cleanup()
	}
}

func BenchmarkGraph1000(b *testing.B)    { benchmarkGraph(b, 1000) }
func BenchmarkGraph10000(b *testing.B)   { benchmarkGraph(b, 10000) }
func BenchmarkGraph100000(b *testing.B)  { benchmarkGraph(b, 100000) }
func BenchmarkGraph1000000(b *testing.B) { benchmarkGraph(b, 1000000) }

func createGraph(txNum int, withParents bool) (*graph, *Transaction, func(), error) {
	kv, cleanup := GetKV("level", "db")

	accounts := newAccounts(kv)

	// We use the default genesis
	genesis, err := performInception(accounts.tree, nil)
	if err != nil {
		return nil, nil, cleanup, err
	}

	g := newGraph(kv, genesis)

	return g, genesis, cleanup, addTxs(g, txNum, withParents)
}

func addTxs(g *graph, txNum int, withParents bool) error {
	viewID := g.loadViewID(g.loadRoot())
	for i := 0; i < txNum; i++ {
		tx, err := createTx(viewID)
		if err != nil {
			return err
		}

		if withParents {
			tx.ParentIDs = g.findEligibleParents()
		}

		if err := g.addTransaction(tx, false); err != nil {
			return err
		}
	}

	return nil
}

func createTx(viewID uint64) (*Transaction, error) {
	tx := &Transaction{
		Tag:       sys.TagTransfer,
		ViewID:    viewID,
		Timestamp: uint64(time.Now().UnixNano()),
	}
	tx.rehash()

	return tx, nil
}

func GetKV(kv string, path string) (store.KV, func()) {
	if kv == "inmem" {
		inmemdb := store.NewInmem()
		return inmemdb, func() {
			_ = inmemdb.Close()
		}
	}
	if kv == "level" {
		// Remove existing db
		_ = os.RemoveAll(path)

		leveldb, err := store.NewLevelDB(path)
		if err != nil {
			panic("failed to create LevelDB: " + err.Error())
		}

		return leveldb, func() {
			_ = leveldb.Close()
			_ = os.RemoveAll(path)
		}
	}

	panic("unknown kv " + kv)
}
