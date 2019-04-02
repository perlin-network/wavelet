package wavelet

import (
	"crypto/rand"
	"github.com/perlin-network/wavelet/common"
	"testing"

	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
)

func TestGraph(t *testing.T) {
	g, genesis := createGraph(t)

	tx := createTx(t)

	err := g.addTransaction(tx)
	assert.NoError(t, err)

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

	rootTx := createTx(t)

	g.saveRoot(rootTx)
	assert.Equal(t, rootTx.ID, g.loadRoot().ID)
}

func TestGraphReset(t *testing.T) {
	g, genesis := graphWithTxs(t, true)

	var buf [200]byte
	_, err := rand.Read(buf[:])
	assert.NoError(t, err)
	resetTx := &Transaction{
		Tag:     sys.TagTransfer,
		Payload: buf[:],
		// high view id to ensure all tx will be removed
		ViewID: pruningDepth + 1,
	}
	resetTx.rehash()

	// Reset
	g.reset(resetTx)
	assert.Equal(t, resetTx.ID, g.loadRoot().ID)

	_, found := g.lookupTransaction(genesis.ID)
	assert.False(t, found)

	_, found = g.lookupTransaction(resetTx.ID)
	assert.True(t, found)

	eligibleParents := g.findEligibleParents()
	assert.Equal(t, 1, len(eligibleParents))
	assert.Equal(t, resetTx.ID, eligibleParents[0])

	assert.Equal(t, uint64(0), g.height.Load())

	assert.Equal(t, 0, g.numTransactions(g.loadViewID(genesis)))
}

func TestAddTransactionWithParents(t *testing.T) {
	g, genesis := createGraph(t)

	tx := createTx(t)
	tx.ParentIDs = []common.TransactionID{genesis.ID}
	assert.NoError(t, g.addTransaction(tx))

	// adding tx with non existing parents should result in error
	tx = createTx(t)
	tx.ParentIDs = []common.TransactionID{{}}
	assert.Equal(t, ErrParentsNotAvailable, g.addTransaction(tx))
}

func TestLookupTransaction(t *testing.T) {
	g, genesis := graphWithTxs(t, false)
	_, exists := g.lookupTransaction(genesis.ID)
	assert.True(t, exists)

	tx, exists := g.lookupTransaction(common.ZeroTransactionID)
	assert.Nil(t, tx)
	assert.False(t, exists)
}

func TestDefaultDifficulty(t *testing.T) {
	g, genesis := createGraph(t)
	g.saveDifficulty(0)
	assert.Equal(t, uint64(0), g.loadDifficulty())

	// new graph using same store should return default difficulty
	g = newGraph(g.kv, genesis)
	assert.Equal(t, uint64(sys.MinDifficulty), g.loadDifficulty())
}

func createGraph(t *testing.T) (*graph, *Transaction) {
	kv := store.NewInmem()
	accounts := newAccounts(kv)

	// We use the default genesis
	genesis, err := performInception(accounts.tree, nil)
	assert.NoError(t, err, "Failed to perform inception with default genesis")

	g := newGraph(kv, genesis)

	return g, genesis
}

func graphWithTxs(t *testing.T, withParents bool) (*graph, *Transaction) {
	g, genesis := createGraph(t)

	// Add random transactions
	for i := 0; i < 100; i++ {
		tx := createTx(t)
		if withParents {
			tx.ParentIDs = g.findEligibleParents()
		}

		assert.NoError(t, g.addTransaction(tx))
	}

	return g, genesis
}

func createTx(t *testing.T) *Transaction {
	var buf [200]byte
	_, err := rand.Read(buf[:])
	assert.NoError(t, err)

	tx := &Transaction{
		Tag:     sys.TagTransfer,
		Payload: buf[:],
	}
	tx.rehash()

	return tx
}