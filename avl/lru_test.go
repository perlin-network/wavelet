package avl

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLRU(t *testing.T) {
	lru := newLRU(2)

	lru.put([MerkleHashSize]byte{'a'}, 1)
	lru.put([MerkleHashSize]byte{'b'}, 2)
	_, ok := lru.load([MerkleHashSize]byte{'b'})
	assert.True(t, ok)
	_, ok = lru.load([MerkleHashSize]byte{'a'})
	assert.True(t, ok)

	lru.put([MerkleHashSize]byte{'c'}, 3)
	_, ok = lru.load([MerkleHashSize]byte{'b'})
	assert.False(t, ok)

	val, ok := lru.load([MerkleHashSize]byte{'a'})
	assert.True(t, ok)
	assert.Equal(t, 1, val.(int))

	val, ok = lru.load([MerkleHashSize]byte{'c'})
	assert.True(t, ok)
	assert.Equal(t, 3, val.(int))
}
