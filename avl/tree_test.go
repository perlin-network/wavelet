package avl

import (
	"bytes"
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
	"testing/quick"
)

func TestSerialize(t *testing.T) {
	kv := store.NewInmem()
	tree := New(kv)

	fn := func(key, value []byte) bool {
		node := newLeafNode(tree, []byte("key"), []byte("value"))

		var buf bytes.Buffer
		node.serialize(&buf)

		assert.ObjectsAreEqual(node, deserialize(bytes.NewReader(buf.Bytes())))

		return true
	}

	assert.NoError(t, quick.Check(fn, nil))
}

func TestTree_Commit(t *testing.T) {
	kv := store.NewInmem()

	{
		tree := New(kv)
		tree.Insert([]byte("key"), []byte("value"))
		assert.NoError(t, tree.Commit())
	}

	{
		tree := New(kv)
		val, ok := tree.Lookup([]byte("key"))
		assert.True(t, ok)
		assert.EqualValues(t, val, []byte("value"))
	}
}

func TestTree_Snapshot(t *testing.T) {
	kv := store.NewInmem()

	tree := New(kv)
	tree.Insert([]byte("k1"), []byte("1"))
	assert.NoError(t, tree.Commit())

	ss := tree.Snapshot()

	tree.Insert([]byte("k1"), []byte("2"))
	tree.Insert([]byte("k2"), []byte("2"))

	tree.Revert(ss)

	v, ok := tree.Lookup([]byte("k1"))
	assert.True(t, ok)
	assert.EqualValues(t, v, []byte("1"))

	_, ok = tree.Lookup([]byte("k2"))
	assert.False(t, ok)
}

func TestTree_Difference(t *testing.T) {
	kv := store.NewInmem()
	kv2 := store.NewInmem()

	tree := New(kv)
	tree.SetViewID(1)
	tree.Insert([]byte("k1"), []byte("1"))

	tree2 := New(kv2)
	tree2.SetViewID(0)
	err := tree2.LoadDifference(tree.DumpDifference(0))
	assert.NoError(t, err)
	assert.Equal(t, tree2.viewID, uint64(1))
	tree2.SetViewID(2)
	tree2.Insert([]byte("k2"), []byte("2"))

	result, _ := tree2.Lookup([]byte("k1"))
	assert.Equal(t, []byte("1"), result)

	result, _ = tree2.Lookup([]byte("k2"))
	assert.Equal(t, []byte("2"), result)

	err = tree.LoadDifference(tree2.DumpDifference(1))
	assert.NoError(t, err)
	assert.Equal(t, tree.viewID, uint64(2))

	result, _ = tree.Lookup([]byte("k2"))
	assert.Equal(t, []byte("2"), result)

	len1 := len(tree.DumpDifference(0))
	len2 := len(tree.DumpDifference(1))
	len3 := len(tree.DumpDifference(2))

	assert.Equal(t, len3, 0)
	assert.True(t, len1 > len2)
	assert.True(t, len2 > len3)
}

func BenchmarkAVL(b *testing.B) {
	const InnerLoopCount = 10000
	const KeySize = 16

	shouldDelete := make([]byte, b.N*InnerLoopCount*2)

	_, err := rand.Read(shouldDelete)
	assert.NoError(b, err)

	b.ResetTimer()

	for i := 0; i < b.N/InnerLoopCount; i++ {
		kv := store.NewInmem()
		tree := New(kv)

		refMap := make(map[string][]byte)
		refDeletes := make(map[string]struct{})

		keys := make([]byte, InnerLoopCount*KeySize)

		_, err := rand.Read(keys)
		assert.NoError(b, err)

		for j := 0; j < InnerLoopCount; j++ {
			shouldDelete := shouldDelete[i*InnerLoopCount+j:]

			if (shouldDelete[0]&1 == 1) && (shouldDelete[1]&1 == 1) {
				if len(refMap) == 0 {
					continue
				}

				var delKey string

				for k, _ := range refMap {
					delKey = k
					break
				}

				lookupResult, ok := tree.Lookup([]byte(delKey))
				assert.True(b, ok)
				assert.EqualValues(b, refMap[delKey], lookupResult)

				delete(refMap, delKey)
				refDeletes[delKey] = struct{}{}
				tree.Delete([]byte(delKey))

				_, ok = tree.Lookup([]byte(delKey))
				assert.False(b, ok)
			} else {
				key := keys[j*KeySize : (j+1)*KeySize]
				value := make([]byte, KeySize)

				for k := 0; k < len(key); k++ {
					value[k] = ^key[k]
				}

				delete(refDeletes, string(key))

				refMap[string(key)] = value
				tree.Insert(key, value)

				lookupResult, ok := tree.Lookup(key)
				assert.True(b, ok)
				assert.EqualValues(b, refMap[string(key)], lookupResult)
			}
		}

		assert.NoError(b, tree.Commit())
	}
}
