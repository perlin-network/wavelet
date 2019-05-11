package avl

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/wavelet/store"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"sync"
)

var NodeKeyPrefix = []byte("@1:")
var GCAliveMarkPrefix = []byte("@2:")
var OldRootsPrefix = []byte("@3:")
var RootKey = []byte(".root")
var NextOldRootIndexKey = []byte(".next_old_root")

const DefaultCacheSize = 2048
const MaxWriteBatchSize = 1024

type Tree struct {
	maxWriteBatchSize int

	kv store.KV

	root *node

	cache   *lru
	pending sync.Map

	viewID uint64
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

func (t *Tree) Snapshot() *Tree {
	return &Tree{kv: t.kv, cache: t.cache, maxWriteBatchSize: t.maxWriteBatchSize, root: t.root}
}

func (t *Tree) Revert(snapshot *Tree) {
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

	t.doRange(callback, t.mustLoadNode(n.left))
	t.doRange(callback, t.mustLoadNode(n.right))
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
	batch := t.kv.NewWriteBatch()

	err := t.root.dfs(t, false, func(n *node) (bool, error) {
		if _, ok := t.pending.Load(n.id); !ok {
			return false, nil
		}
		t.pending.Delete(n.id)
		var buf bytes.Buffer
		n.serialize(&buf)

		batch.Put(append(NodeKeyPrefix, n.id[:]...), buf.Bytes())
		return true, nil
	})
	if err != nil {
		return err
	}

	err = t.kv.CommitWriteBatch(batch)
	if err != nil {
		return errors.Wrap(err, "failed to commit write batch to db")
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
		return t.kv.Put(RootKey, t.root.id[:])
	}

	// If deleting the root fails because it doesn't exist, ignore the error.
	_ = t.kv.Delete(RootKey)

	return nil
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
	_ = t.kv.Put(NextOldRootIndexKey, buf[:])
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

	_ = t.kv.Put(append(OldRootsPrefix, buf[:]...), value)
}

func (t *Tree) deleteOldRoot(idx uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], idx)

	_ = t.kv.Delete(append(OldRootsPrefix, buf[:]...))
}

func (t *Tree) Checksum() [MerkleHashSize]byte {
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
	_ = t.kv.Delete(append(NodeKeyPrefix, id[:]...))
	_ = t.kv.Delete(append(GCAliveMarkPrefix, id[:]...))
}

func (t *Tree) SetViewID(viewID uint64) {
	t.viewID = viewID
}

func (t *Tree) DumpDiff(prevViewID uint64) []byte {
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
	reader := bytes.NewReader(diff)

	var root *node
	unresolved := make(map[[MerkleHashSize]byte]struct{})
	preloaded := make(map[[MerkleHashSize]byte]*node)

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

type GCProfile struct {
	t             *Tree
	lastDepth     uint64
	preserveDepth uint64
}

func (t *Tree) GetGCProfile(preserveDepth uint64) *GCProfile {
	nextDepth := t.getNextOldRootIndex()
	if nextDepth <= preserveDepth {
		return nil
	}

	return &GCProfile{
		t:             t,
		lastDepth:     nextDepth - 1,
		preserveDepth: preserveDepth,
	}
}

func (profile *GCProfile) PerformFullGC() (int, error) {
	var mark [16]byte
	if _, err := rand.Read(mark[:]); err != nil {
		return 0, err
	}

	var i int64
	for i = int64(profile.lastDepth); i >= int64(profile.lastDepth-profile.preserveDepth); i-- {
		i := uint64(i)
		id, ok := profile.t.getOldRoot(i)
		if !ok {
			return 0, nil
		}

		n, err := profile.t.loadNode(id)
		if err != nil {
			return 0, err
		}
		err = n.dfs(profile.t, false, func(n *node) (bool, error) {
			return true, profile.t.kv.Put(append(GCAliveMarkPrefix, n.id[:]...), mark[:])
		})
		if err != nil {
			return 0, err
		}
	}

	deleteCount := 0
	for ; i >= 0; i-- {
		i := uint64(i)
		id, ok := profile.t.getOldRoot(i)
		if !ok {
			return deleteCount, nil
		}

		n, err := profile.t.loadNode(id)
		if err != nil {
			return 0, err
		}
		err = n.dfs(profile.t, true, func(n *node) (bool, error) {
			gotMark, _ := profile.t.kv.Get(append(GCAliveMarkPrefix, n.id[:]...))
			if bytes.Equal(gotMark, mark[:]) {
				return false, nil
			}
			profile.t.deleteNodeAndMetadata(n.id)
			deleteCount++
			return true, nil
		})
		if err != nil {
			return 0, err
		}

		profile.t.deleteOldRoot(i)
	}

	return deleteCount, nil
}
