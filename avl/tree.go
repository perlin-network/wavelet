package avl

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
	"sync"
)

var NodeKeyPrefix = []byte("@")
var RootKey = []byte(".root")

const DefaultCacheSize = 128
const MaxWriteBatchSize = 1024

type Tree struct {
	maxWriteBatchSize int

	kv store.KV

	root *node

	cache   *lru
	pending sync.Map
}

type Snapshot struct {
	root *node
}

func New(kv store.KV) *Tree {
	t := &Tree{kv: kv, cache: newLRU(DefaultCacheSize), maxWriteBatchSize: MaxWriteBatchSize}

	// Load root node if it already exists.
	if buf, err := t.kv.Get(RootKey); err == nil && len(buf) == MerkleRootSize {
		var rootID [MerkleRootSize]byte
		copy(rootID[:], buf)

		t.root = t.loadNode(rootID)
	}

	return t
}

func LoadFromSnapshot(kv store.KV, ss Snapshot) *Tree {
	return &Tree{root: ss.root, kv: kv, cache: newLRU(DefaultCacheSize), maxWriteBatchSize: MaxWriteBatchSize}
}

func (t *Tree) WithLRUCache(size *int) *Tree {
	if size == nil {
		t.cache = nil
	} else {
		t.cache = newLRU(*size)
	}

	return t
}

func (t *Tree) WithMaxWriteBatchSize(maxWriteBatchSize int) *Tree {
	t.maxWriteBatchSize = maxWriteBatchSize
	return t
}

func (t *Tree) Insert(key, value []byte) {
	if t.root == nil {
		t.root = newLeafNode(t, key, value)
	} else {
		t.root = t.root.insert(t, key, value)
	}
}

func (t *Tree) Lookup(k []byte) ([]byte, bool) {
	if t.root == nil {
		return nil, false
	}

	return t.root.lookup(t, k)
}

func (t *Tree) Delete(k []byte) bool {
	if t.root == nil {
		return false
	}

	root, deleted := t.root.delete(t, k)
	t.root = root

	return deleted
}

func (t *Tree) Snapshot() Snapshot {
	return Snapshot{root: t.root}
}

func (t *Tree) Revert(snapshot Snapshot) {
	t.root = snapshot.root
}

func (t *Tree) Range(callback func(key, value []byte)) {
	t.doRange(callback, t.root)
}

func (t *Tree) doRange(callback func(k []byte, v []byte), n *node) {
	if n == nil {
		return
	}

	if n.kind == NodeLeafValue {
		callback(n.key, n.value)
		return
	}

	t.doRange(callback, t.loadNode(n.left))
	t.doRange(callback, t.loadNode(n.right))
}

func (t *Tree) PrintContents() {
	if t.root != nil {
		t.doPrintContents(t.root, 0)
	} else {
		fmt.Println("(empty)")
	}
}

func (t *Tree) doPrintContents(n *node, depth int) {
	for i := 0; i < depth; i++ {
		fmt.Print(" ")
	}

	fmt.Printf("%s: %s\n", hex.EncodeToString(n.id[:]), n.String())

	t.doPrintContents(t.loadNode(n.left), depth+1)
	t.doPrintContents(t.loadNode(n.right), depth+1)
}

func (t *Tree) queueWrite(n *node) {
	t.pending.Store(n.id, n)
}

func (t *Tree) Commit() error {
	for {
		batch := t.kv.NewWriteBatch()

		t.pending.Range(func(k, v interface{}) bool {
			if batch.Count() > t.maxWriteBatchSize {
				return false
			}

			t.pending.Delete(k)

			id, node := k.([MerkleRootSize]byte), v.(*node)

			var buf bytes.Buffer
			node.serialize(&buf)

			batch.Put(append(NodeKeyPrefix, id[:]...), buf.Bytes())

			return true
		})

		if batch.Count() == 0 {
			break
		}

		err := t.kv.CommitWriteBatch(batch)
		if err != nil {
			return errors.Wrap(err, "failed to commit write batch to db")
		}
	}

	if t.root != nil {
		return t.kv.Put(RootKey, t.root.id[:])
	}

	// If deleting the root fails because it doesn't exist, ignore the error.
	_ = t.kv.Delete(RootKey)

	return nil
}

func (t *Tree) Checksum() [MerkleRootSize]byte {
	if t.root == nil {
		return [MerkleRootSize]byte{}
	}

	return t.root.id
}

func (t *Tree) loadNode(id [MerkleRootSize]byte) *node {
	if n, ok := t.pending.Load(id); ok {
		return n.(*node)
	}

	if n, ok := t.cache.load(id); ok {
		return n.(*node)
	}

	buf, err := t.kv.Get(append(NodeKeyPrefix, id[:]...))

	if err != nil || len(buf) == 0 {
		panic(errors.Errorf("avl: could not find node %x", id))
	}

	n := deserialize(buf)
	t.cache.put(id, n)

	return n
}
