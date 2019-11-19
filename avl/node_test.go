package avl

import (
	"crypto/rand"
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"testing"
)

func BenchmarkRehashNoWrite(b *testing.B) {
	kv, cleanup := store.NewTestKV(b, "inmem", "")
	defer cleanup()

	tree := New(kv)

	var key [32]byte
	_, err := rand.Read(key[:]) // nolint:gosec
	assert.NoError(b, err)

	var val [64]byte
	_, err = rand.Read(val[:]) // nolint:gosec
	assert.NoError(b, err)

	node := newLeafNode(tree, key[:], val[:])

	b.ResetTimer()
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		node.rehashNoWrite()
	}
}
