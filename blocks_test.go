// +build unit

package wavelet

import (
	"crypto/rand"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewBlocks(t *testing.T) {
	t.Parallel()

	storage := store.NewInmem()

	b, err := NewBlocks(storage, 10)
	if !assert.EqualError(t, errors.Cause(err), store.ErrNotFound.Error()) {
		return
	}

	if !assert.NotNil(t, b) {
		return
	}

	for i := 0; i < 5; i++ {
		tb := &Block{Index: uint64(i + 1)}
		_, err := b.Save(tb)
		assert.NoError(t, err)
	}

	assert.Equal(t, uint64(5), b.Latest().Index)
	assert.Equal(t, uint64(1), b.Oldest().Index)

	newRM, err := NewBlocks(storage, 10)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, b.latest, newRM.latest)
	assert.Equal(t, b.oldest, newRM.oldest)
	assert.Equal(t, len(b.buffer), len(newRM.buffer))

	assert.Equal(t, uint64(5), newRM.Latest().Index)
	assert.Equal(t, uint64(1), newRM.Oldest().Index)
	assert.Equal(t, uint64(5), newRM.LatestHeight())
}

func TestBlocksCircular(t *testing.T) {
	t.Parallel()

	storage := store.NewInmem()

	b, err := NewBlocks(storage, 10)
	if !assert.EqualError(t, errors.Cause(err), store.ErrNotFound.Error()) {
		return
	}

	if !assert.NotNil(t, b) {
		return
	}

	var merkle MerkleNodeID
	for i := 0; i < 15; i++ {
		_, err := rand.Read(merkle[:])
		if !assert.NoError(t, err) {
			return
		}

		tb := NewBlock(uint64(i+1), merkle, []TransactionID{}...)

		_, err = b.Save(&tb)
		if !assert.NoError(t, err) {
			return
		}
	}

	assert.Equal(t, uint64(15), b.Latest().Index)
	assert.Equal(t, uint64(6), b.Oldest().Index)

	newB, err := NewBlocks(storage, 10)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, b.latest, newB.latest)
	assert.Equal(t, b.oldest, newB.oldest)
	assert.Equal(t, len(b.buffer), len(newB.buffer))
	assert.Equal(t, uint32(4), newB.latest)
	assert.Equal(t, uint32(5), newB.oldest)

	blocks := b.Clone()
	newBlocks := newB.Clone()
	if !assert.Equal(t, len(blocks), len(newBlocks)) {
		return
	}

	for i := range blocks {
		assert.Equal(t, *blocks[i], *newBlocks[i])
	}
}
