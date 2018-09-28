package iavl

import (
	"math/rand"
	"strconv"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func BenchmarkBasicRand(b *testing.B) {
	for i := 0; i < b.N; i++ {
		strconv.Itoa(rand.Intn(2000000000))
	}
}

func BenchmarkStoreLoadDelete(b *testing.B) {
	var root *Node
	data := []byte("Hello world")

	for i := 0; i < b.N; i++ {
		key := strconv.Itoa(rand.Intn(2000000000))
		root, _ = root.Store(key, data)
		_, val := root.Load(key)
		if unsafe.Pointer(&val[0]) != unsafe.Pointer(&data[0]) {
			panic("data mismatch")
		}
		root, _ = root.Delete(key)
	}
}

func BenchmarkLoad(b *testing.B) {
	const NUM_ITEMS = 100000

	var root *Node
	var selectedKey string
	data := []byte("Hello world")

	for i := 0; i < NUM_ITEMS; i++ {
		key := strconv.Itoa(rand.Intn(2000000000))
		root, _ = root.Store(key, data)
		if rand.Intn(30) == 15 {
			selectedKey = key
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, val := root.Load(selectedKey)
		if unsafe.Pointer(&val[0]) != unsafe.Pointer(&data[0]) {
			panic("data mismatch")
		}
	}
}

func BenchmarkStoreLoad(b *testing.B) {
	var root *Node
	data := []byte("Hello world")

	for i := 0; i < b.N; i++ {
		key := strconv.Itoa(rand.Intn(2000000000))
		root, _ = root.Store(key, data)
		_, val := root.Load(key)
		if unsafe.Pointer(&val[0]) != unsafe.Pointer(&data[0]) {
			panic("data mismatch")
		}
	}
}

func TestTree(t *testing.T) {
	type KVPair struct {
		Key   string
		Value []byte
	}

	var root *Node

	pairs := []KVPair{
		{"0", []byte("this is a hidden value!")},
		{"1", []byte("things that work")},
		{"a", []byte("hello")},
		{"b", []byte("weird value!")},
		{"c", []byte("more beautiful lorem ipsum")},
	}

	shuffledPairs := make([]KVPair, len(pairs))
	copy(shuffledPairs, pairs)
	rand.Shuffle(len(shuffledPairs), func(i, j int) {
		shuffledPairs[i], shuffledPairs[j] = shuffledPairs[j], shuffledPairs[i]
	})

	root, orig := root.Store("a", []byte("_"))
	assert.Equal(t, orig, ([]byte)(nil), "orig should be nil")

	root, orig = root.Store("a", []byte("!"))
	assert.Equal(t, orig, []byte("_"), "bytes mismatch")

	for _, p := range shuffledPairs {
		root, _ = root.Store(p.Key, p.Value)
	}

	pairID := 0
	root.Range(func(key string, value []byte) {
		assert.Equal(t, pairs[pairID], KVPair{
			Key:   key,
			Value: value,
		})
		pairID++
	})
	assert.Equal(t, pairID, len(pairs))

	_, value := root.Load("b")
	assert.NotEqual(t, value, ([]byte)(nil))

	root, _ = root.Delete("b")
	_, value = root.Load("b")
	assert.Equal(t, value, ([]byte)(nil))

	pairID = 0
	root.Range(func(_ string, _ []byte) {
		pairID++
	})
	assert.Equal(t, pairID, len(pairs)-1)
}
