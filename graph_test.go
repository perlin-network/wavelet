package wavelet

import (
	"github.com/perlin-network/wavelet/common"
	"testing"
	"time"

	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
)

func TestGraph(t *testing.T) {
	g, genesis, err := createGraph(0, false)
	if !assert.NoError(t, err) {
		return
	}

	tx, err := createTx()
	if !assert.NoError(t, err) {
		return
	}

	assert.NoError(t, g.addTransaction(tx))

	// Test transaction already exist
	err = g.addTransaction(tx)
	assert.Error(t, err)
	assert.Equal(t, ErrTxAlreadyExists, err)

	assert.Equal(t, uint64(1), g.height.Load())

	assert.Equal(t, genesis.ID, g.loadRoot().ID)

	_, found := g.lookupTransaction(tx.ID)
	assert.True(t, found)

	eligibleParents := g.findEligibleParents()
	assert.Equal(t, 1, len(eligibleParents))
	assert.Equal(t, genesis.ID, eligibleParents[0])

	rootTx, err := createTx()
	if !assert.NoError(t, err) {
		return
	}

	g.saveRoot(rootTx)
	assert.Equal(t, rootTx.ID, g.loadRoot().ID)
}

func TestGraphReset(t *testing.T) {
	g, genesis, err := createGraph(100, true)
	if !assert.NoError(t, err) {
		return
	}

	assert.NotEmpty(t, g.eligibleParents)

	resetTx, err := createTx()
	if !assert.NoError(t, err) {
		return
	}

	resetTx.ViewID = genesis.ViewID + pruningDepth + 1
	resetTx.rehash()

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
}

func TestAddTransactionWithParents(t *testing.T) {
	g, genesis, err := createGraph(0, false)
	if !assert.NoError(t, err) {
		return
	}

	tx, err := createTx()
	if !assert.NoError(t, err) {
		return
	}

	tx.ParentIDs = []common.TransactionID{genesis.ID}
	assert.NoError(t, g.addTransaction(tx))

	// adding tx with non existing parents should result in error
	tx, err = createTx()
	if !assert.NoError(t, err) {
		return
	}

	tx.ParentIDs = []common.TransactionID{{}}
	assert.Equal(t, ErrParentsNotAvailable, g.addTransaction(tx))
}

func TestLookupTransaction(t *testing.T) {
	g, genesis, err := createGraph(100,false)
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
	g, genesis, err := createGraph(0, false)
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
	g, _, err := createGraph(100, true)
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
		if _, _, err := createGraph(n, true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGraph1000(b *testing.B) {benchmarkGraph(b, 1000)}
func BenchmarkGraph10000(b *testing.B) {benchmarkGraph(b, 10000)}
func BenchmarkGraph100000(b *testing.B) {benchmarkGraph(b, 100000)}
func BenchmarkGraph1000000(b *testing.B) {benchmarkGraph(b, 1000000)}

func createGraph(txNum int, withParents bool) (*graph, *Transaction, error) {
	kv := store.NewInmem()
	accounts := newAccounts(kv)

	// We use the default genesis
	genesis, err := performInception(accounts.tree, nil)
	if err != nil {
		return nil, nil, err
	}

	g := newGraph(kv, genesis)

	// Add random transactions
	for i := 0; i < txNum; i++ {
		tx, err := createTx()
		if err != nil {
			return nil, nil, err
		}

		tx.ViewID = g.loadViewID(genesis)

		if withParents {
			tx.ParentIDs = g.findEligibleParents()
		}

		if err := g.addTransaction(tx); err != nil {
			return nil, nil, err
		}
	}

	return g, genesis, nil
}

func createTx() (*Transaction, error) {
	tx := &Transaction{
		Tag:     sys.TagTransfer,
		Timestamp: uint64(time.Now().UnixNano()),
	}
	tx.rehash()

	return tx, nil
}