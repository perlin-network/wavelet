package avl

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/wavelet/store"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"math"
	"sync"
)

var NodeKeyPrefix = []byte("@1:")
var NodeLastReferencePrefix = []byte("@2:")
var OldRootsPrefix = []byte("@3:")
var RootKey = []byte(".root")
var NextOldRootIndexKey = []byte(".next_old_root")

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

	{
		oldRootID, err := t.kv.Get(RootKey)

		// If we want to include null roots here, getOldRoot() also needs to be fixed.
		if err == nil && len(oldRootID) == MerkleHashSize {
			nextOldRootIndex := t.getNextOldRootIndex()
			t.setOldRoot(nextOldRootIndex, oldRootID)
			t.setNextOldRootIndex(nextOldRootIndex + 1)
		}
	}

	if t.root != nil {
		updated := t.root.updateBorderReferences(t)
		log.Info().Uint64("updated", updated).Msg("Updated border references")
		return t.kv.Put(RootKey, t.root.id[:])
	}

	// If deleting the root fails because it doesn't exist, ignore the error.
	_ = t.kv.Delete(RootKey)

	return nil
}

func (t *Tree) CollectGarbage(historyDepth uint64) {
	index := t.getNextOldRootIndex()
	if index == 0 {
		return
	}
	index--

	if index >= historyDepth {
		index -= historyDepth
	} else {
		return
	}

	pendingRoots := make([][MerkleHashSize]byte, 0)

	for i := index; i != math.MaxUint64; /* overflow */ i-- {
		rootID, ok := t.getOldRoot(i)
		if !ok {
			break
		}
		pendingRoots = append(pendingRoots, rootID)
		t.deleteOldRoot(i)
	}

	count := uint64(0)

	for i := len(pendingRoots) - 1; i >= 0; i-- {
		rootID := pendingRoots[i]
		n := t.mustLoadNode(rootID)
		count += n.recursivelyDestroy(t, n.viewID)
	}

	log.Info().Uint64("count", count).Msg("Garbage collected")
}

func (t *Tree) getNextOldRootIndex() uint64 {
	nextOldRootIndexBuf, err := t.kv.Get(NextOldRootIndexKey)
	if err != nil || len(nextOldRootIndexBuf) == 0 {
		return 0
	} else {
		return binary.LittleEndian.Uint64(nextOldRootIndexBuf)
	}
}

func (t *Tree) setNextOldRootIndex(x uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], x)
	t.kv.Put(NextOldRootIndexKey, buf[:])
}

func (t *Tree) getOldRoot(idx uint64) ([MerkleHashSize]byte, bool) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], idx)

	out, err := t.kv.Get(append(OldRootsPrefix, buf[:]...))
	if err != nil || len(out) == 0 {
		return [MerkleHashSize]byte{}, false
	} else {
		var ret [MerkleHashSize]byte
		copy(ret[:], out)
		return ret, true
	}
}

func (t *Tree) setOldRoot(idx uint64, value []byte) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], idx)

	t.kv.Put(append(OldRootsPrefix, buf[:]...), value)
}

func (t *Tree) deleteOldRoot(idx uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], idx)

	t.kv.Delete(append(OldRootsPrefix, buf[:]...))
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

func (t *Tree) deleteNodeAndMetadata(id [MerkleHashSize]byte) {
	t.pending.Delete(id)
	t.cache.remove(id)
	t.kv.Delete(append(NodeKeyPrefix, id[:]...))
	t.kv.Delete(append(NodeLastReferencePrefix, id[:]...))
}

func (t *Tree) loadLastReference(id [MerkleHashSize]byte) (uint64, bool, error) {
	buf, err := t.kv.Get(append(NodeLastReferencePrefix, id[:]...))

	if err != nil || len(buf) == 0 {
		return 0, false, nil
	}

	if len(buf) != 8 {
		return 0, false, errors.Errorf("avl: invalid encoding of last reference value")
	}

	return binary.LittleEndian.Uint64(buf), true, nil
}

func (t *Tree) mustLoadLastReference(id [MerkleHashSize]byte) (uint64, bool) {
	x, found, err := t.loadLastReference(id)
	if err != nil {
		panic(err)
	}
	return x, found
}

func (t *Tree) storeLastReference(id [MerkleHashSize]byte, x uint64) error {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], x)
	return t.kv.Put(append(NodeLastReferencePrefix, id[:]...), buf[:])
}

func (t *Tree) mustStoreLastReference(id [MerkleHashSize]byte, x uint64) {
	err := t.storeLastReference(id, x)
	if err != nil {
		panic(err)
	}
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
