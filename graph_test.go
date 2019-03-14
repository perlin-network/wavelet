package wavelet

import (
	"crypto/rand"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"testing"
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

	assert.Equal(t, uint64(1), g.Height())

	assert.Equal(t, genesis.ID, g.loadRoot().ID)

	_, found := g.lookupTransaction(tx.ID)
	assert.True(t, found)

	eligibleParents := g.findEligibleParents(0)
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
	assert.False(t, found)

	_, found = g.lookupTransaction(resetTx.ID)
	assert.True(t, found)

	eligibleParents := g.findEligibleParents(0)
	assert.Equal(t, 1, len(eligibleParents))
	assert.Equal(t, resetTx.ID, eligibleParents[0])

	assert.Equal(t, uint64(0), g.Height())
}

func createGraph(t *testing.T) (*graph, *Transaction) {
	kv := store.NewInmem()

	// We use the default genesis
	genesis, err := performInception(newAccounts(kv), "")
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
