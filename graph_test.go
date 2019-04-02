package wavelet

import (
	"crypto/rand"
	"testing"

	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
)

func TestGraph(t *testing.T) {
	g, genesis := createGraph(t)

	var buf [200]byte
	_, err := rand.Read(buf[:])
	assert.NoError(t, err)

	tx := &Transaction{
		Tag:     sys.TagTransfer,
		Payload: buf[:],
	}
	tx.rehash()

	err = g.addTransaction(tx)
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

	rootTx := &Transaction{
		Tag:     sys.TagTransfer,
		Payload: buf[:],
	}
	rootTx.rehash()

	g.saveRoot(rootTx)
	assert.Equal(t, rootTx.ID, g.loadRoot().ID)
}

func TestGraphReset(t *testing.T) {
	g, genesis := createGraph(t)

	var buf [200]byte
	_, err := rand.Read(buf[:])
	assert.NoError(t, err)
	resetTx := &Transaction{
		Tag:     sys.TagTransfer,
		Payload: buf[:],
	}
	resetTx.rehash()

	// Reset
	g.reset(resetTx)
	assert.Equal(t, resetTx.ID, g.loadRoot().ID)

	_, found := g.lookupTransaction(genesis.ID)
	assert.True(t, found)

	_, found = g.lookupTransaction(resetTx.ID)
	assert.True(t, found)

	eligibleParents := g.findEligibleParents()
	assert.Equal(t, 1, len(eligibleParents))
	assert.Equal(t, resetTx.ID, eligibleParents[0])

	assert.Equal(t, uint64(0), g.height.Load())
}

func createGraph(t *testing.T) (*graph, *Transaction) {
	kv := store.NewInmem()
	accounts := newAccounts(kv)

	// We use the default genesis
	genesis, err := performInception(accounts.tree, nil)
	assert.NoError(t, err, "Failed to perform inception with default genesis")

	g := newGraph(kv, genesis)

	// Add random transactions
	var buf [200]byte
	for i := 0; i < 100; i++ {
		_, err := rand.Read(buf[:])
		assert.NoError(t, err)

		tx := &Transaction{
			Tag:     sys.TagTransfer,
			Payload: buf[:],
		}
		tx.rehash()
		assert.NoError(t, g.addTransaction(tx))
	}

	return g, genesis
}
