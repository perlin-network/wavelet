package avl

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/wavelet/store"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"sync"
)

var NodeKeyPrefix = []byte("@")
var RootKey = []byte(".root")

const DefaultCacheSize = 128
const MaxWriteBatchSize = 1024

type Tree struct {
	sync.RWMutex

	maxWriteBatchSize int

	kv store.KV

	root *node

	cache   *lru
	pending sync.Map

	viewID uint64
}

type Snapshot struct {
	root *node
}

func New(kv store.KV) *Tree {
	t := &Tree{kv: kv, cache: newLRU(DefaultCacheSize), maxWriteBatchSize: MaxWriteBatchSize}

	// Load root node if it already exists.
	if buf, err := t.kv.Get(RootKey); err == nil && len(buf) == MerkleHashSize {
		var rootID [MerkleHashSize]byte
		copy(rootID[:], buf)

		t.root = t.mustLoadNode(rootID)
	}

	return t
}

func LoadFromSnapshot(kv store.KV, ss Snapshot) *Tree {
	return &Tree{root: ss.root, kv: kv, cache: newLRU(DefaultCacheSize), maxWriteBatchSize: MaxWriteBatchSize}
}

func (t *Tree) WithLRUCache(size *int) *Tree {
	t.Lock()
	defer t.Unlock()

	if size == nil {
		t.cache = nil
	} else {
		t.cache = newLRU(*size)
	}

	return t
}

func (t *Tree) WithMaxWriteBatchSize(maxWriteBatchSize int) *Tree {
	t.Lock()
	defer t.Unlock()

	t.maxWriteBatchSize = maxWriteBatchSize
	return t
}

func (t *Tree) Insert(key, value []byte) {
	t.Lock()
	defer t.Unlock()

	if t.root == nil {
		t.root = newLeafNode(t, key, value)
	} else {
		t.root = t.root.insert(t, key, value)
	}
}

func (t *Tree) Lookup(k []byte) ([]byte, bool) {
	t.RLock()
	defer t.RUnlock()

	if t.root == nil {
		return nil, false
	}

	return t.root.lookup(t, k)
}

func (t *Tree) Delete(k []byte) bool {
	t.Lock()
	defer t.Unlock()

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
	t.Lock()
	defer t.Unlock()

	t.root = snapshot.root
}

func (t *Tree) Range(callback func(key, value []byte)) {
	t.RLock()
	defer t.RUnlock()

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

	t.doRange(callback, t.mustLoadNode(n.left))
	t.doRange(callback, t.mustLoadNode(n.right))
}

func (t *Tree) PrintContents() {
	t.RLock()
	defer t.RUnlock()

	if t.root != nil {
		t.doPrintContents(t.root, 0)
	} else {
		fmt.Println("(empty)")
	}
}

func (t *Tree) doPrintContents(n *node, depth int) {
	t.RLock()
	defer t.RUnlock()

	for i := 0; i < depth; i++ {
		fmt.Print(" ")
	}

	fmt.Printf("%s: %s\n", hex.EncodeToString(n.id[:]), n.getString())
	if n.kind == NodeNonLeaf {
		t.doPrintContents(t.mustLoadNode(n.left), depth+1)
		t.doPrintContents(t.mustLoadNode(n.right), depth+1)
	}
}

func (t *Tree) queueWrite(n *node) {
	t.pending.Store(n.id, n)
}

func (t *Tree) Commit() error {
	t.Lock()
	defer t.Unlock()

	batch := t.kv.NewWriteBatch()

	for {
		t.pending.Range(func(k, v interface{}) bool {
			if batch.Count() > t.maxWriteBatchSize {
				return false
			}

			t.pending.Delete(k)

			id, node := k.([MerkleHashSize]byte), v.(*node)

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

		batch.Clear()
	}

	if t.root != nil {
		return t.kv.Put(RootKey, t.root.id[:])
	}

	// If deleting the root fails because it doesn't exist, ignore the error.
	_ = t.kv.Delete(RootKey)

	return nil
}

func (t *Tree) Checksum() [MerkleHashSize]byte {
	t.RLock()
	defer t.RUnlock()

	if t.root == nil {
		return [MerkleHashSize]byte{}
	}

	return t.root.id
}

func (t *Tree) loadNode(id [MerkleHashSize]byte) (*node, error) {
	if n, ok := t.pending.Load(id); ok {
		return n.(*node), nil
	}

	if n, ok := t.cache.load(id); ok {
		return n.(*node), nil
	}

	buf, err := t.kv.Get(append(NodeKeyPrefix, id[:]...))

	if err != nil || len(buf) == 0 {
		return nil, errors.Errorf("avl: could not find node %x", id)
	}

	n := mustDeserialize(bytes.NewReader(buf))
	t.cache.put(id, n)

	return n, nil
}

func (t *Tree) mustLoadNode(id [MerkleHashSize]byte) *node {
	n, err := t.loadNode(id)
	if err != nil {
		panic(err)
	}
	return n
}

func (t *Tree) SetViewID(viewID uint64) {
	t.Lock()
	defer t.Unlock()

	t.viewID = viewID
}

func (t *Tree) DumpDiff(prevViewID uint64) []byte {
	t.RLock()
	defer t.RUnlock()

	var stack queue.Queue
	stack.PushBack(t.root)

	buf := bytes.NewBuffer(nil)

	for stack.Len() > 0 {
		current := stack.PopBack().(*node)

		if current.viewID <= prevViewID {
			continue
		}

		current.serializeForDifference(buf)

		if current.size > 1 {
			stack.PushBack(t.mustLoadNode(current.right))
			stack.PushBack(t.mustLoadNode(current.left))
		}
	}

	return buf.Bytes()
}

func (t *Tree) ApplyDiff(diff []byte) error {
	t.Lock()
	defer t.Unlock()

	reader := bytes.NewReader(diff)

	var root *node
	unresolved := make(map[[MerkleHashSize]byte]struct{})
	preloaded := make(map[[MerkleHashSize]byte]*node, 0)

	for reader.Len() > 0 {
		n, err := deserializeFromDifference(reader, t.viewID)
		if err != nil {
			return err
		}
		preloaded[n.id] = n
		if root == nil {
			root = n
		} else {
			if _, ok := unresolved[n.id]; !ok {
				return errors.Errorf("unexpected node")
			}
			delete(unresolved, n.id)
		}
		if n.kind == NodeNonLeaf {
			unresolved[n.left] = struct{}{}
			unresolved[n.right] = struct{}{}
		}
	}

	if root == nil {
		return nil
	}

	_, _, _, _, err := populateDiffs(t, root.id, preloaded, make(map[[MerkleHashSize]byte]struct{}))
	if err != nil {
		return errors.Wrap(err, "invalid difference")
	}

	for _, n := range preloaded {
		t.pending.Store(n.id, n)
	}

	t.viewID = root.viewID
	t.root = root

	return nil
}
