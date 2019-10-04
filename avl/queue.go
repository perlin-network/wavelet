package avl

import (
	"io"

	"github.com/perlin-network/wavelet/store"
)

type diffQueue struct {
	prefix []byte
	kv     store.KV
	viewID uint64
	ids    map[[MerkleHashSize]byte]struct{}

	Reader    io.Reader
	OnDequeue func(*node) error
}

func newDiffQueue(kv store.KV, prefix []byte, viewID uint64) *diffQueue {
	return &diffQueue{
		prefix: prefix,
		kv:     kv,
		ids:    make(map[[MerkleHashSize]byte]struct{}),
	}
}

func (h *diffQueue) AddID(id [MerkleHashSize]byte) {
	h.ids[id] = struct{}{}
}

func (h *diffQueue) Has(id [MerkleHashSize]byte) bool {
	_, ok := h.ids[id]
	return ok
}

func (h *diffQueue) Dequeue() (*node, error) {
	node, err := DeserializeFromDifference(h.Reader, h.viewID)
	if err != nil {
		return nil, err
	}

	if h.OnDequeue != nil {
		err = h.OnDequeue(node)
	}
	return node, err
}
