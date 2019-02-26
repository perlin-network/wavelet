package avl

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLRU(t *testing.T) {
	lru := newLRU(2)

	lru.put([MerkleRootSize]byte{'a'}, 1)
	lru.put([MerkleRootSize]byte{'b'}, 2)
	_, ok := lru.load([MerkleRootSize]byte{'b'})
	assert.True(t, ok)
	_, ok = lru.load([MerkleRootSize]byte{'a'})
	assert.True(t, ok)

	lru.put([MerkleRootSize]byte{'c'}, 3)
	_, ok = lru.load([MerkleRootSize]byte{'b'})
	assert.False(t, ok)

	val, ok := lru.load([MerkleRootSize]byte{'a'})
	assert.True(t, ok)
	assert.Equal(t, 1, val.(int))

	val, ok = lru.load([MerkleRootSize]byte{'c'})
	assert.True(t, ok)
	assert.Equal(t, 3, val.(int))
}
