// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package avl

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"

	"github.com/minio/highwayhash"

	"github.com/pkg/errors"
	"github.com/valyala/bytebufferpool"
)

var (
	hashKey = make([]byte, 32)
)

type nodeType byte

const (
	MerkleHashSize = 16

	NodeNonLeaf nodeType = iota
	NodeLeafValue
)

type node struct {
	id, left, right   [MerkleHashSize]byte
	leftObj, rightObj *node

	wroteBack bool

	viewID uint64

	key, value []byte

	kind nodeType

	depth byte
	size  uint64
}

func newLeafNode(t *Tree, key, value []byte) *node {
	n := &node{
		key:   key,
		value: value,

		kind: NodeLeafValue,

		depth: 0,
		size:  1,

		viewID: t.viewID,
	}

	n.rehash()

	return n
}

func (n *node) balanceFactor(t *Tree, left *node, right *node) int {
	if left == nil {
		left = t.mustLoadLeft(n)
	}

	if right == nil {
		right = t.mustLoadRight(n)
	}

	return int(left.depth) - int(right.depth)
}

func (n *node) sync(t *Tree, left *node, right *node) {
	if left == nil {
		left = t.mustLoadLeft(n)
	}

	if right == nil {
		right = t.mustLoadRight(n)
	}

	if left.depth > right.depth {
		n.depth = left.depth + 1
	} else {
		n.depth = right.depth + 1
	}

	n.size = left.size + right.size

	if bytes.Compare(left.key, right.key) > 0 {
		n.key = left.key
	} else {
		n.key = right.key
	}
}

func (n *node) leftRotate(t *Tree) *node {
	right := t.mustLoadRight(n)

	n = n.update(t, func(node *node) {
		node.right = right.left
		node.rightObj = right.leftObj
		node.sync(t, nil, nil)
	})

	right = right.update(t, func(node *node) {
		node.left = n.id
		node.leftObj = n
		node.sync(t, nil, nil)
	})

	return right
}

func (n *node) rightRotate(t *Tree) *node {
	left := t.mustLoadLeft(n)

	n = n.update(t, func(node *node) {
		node.left = left.right
		node.leftObj = left.rightObj
		node.sync(t, nil, nil)
	})

	left = left.update(t, func(node *node) {
		node.right = n.id
		node.rightObj = n
		node.sync(t, nil, nil)
	})

	return left
}

func (n *node) rebalance(t *Tree) *node {
	left := t.mustLoadLeft(n)
	right := t.mustLoadRight(n)

	balance := n.balanceFactor(t, left, right)

	if balance > 1 {
		if left.balanceFactor(t, nil, nil) < 0 {
			n = n.update(t, func(node *node) {
				newLeft := left.leftRotate(t)
				node.left = newLeft.id
				node.leftObj = newLeft
			})
		}

		return n.rightRotate(t)
	} else if balance < -1 {
		if right.balanceFactor(t, nil, nil) > 0 {
			n = n.update(t, func(node *node) {
				newRight := right.rightRotate(t)
				node.right = newRight.id
				node.rightObj = newRight
			})
		}

		return n.leftRotate(t)
	}

	return n
}

func (n *node) insert(t *Tree, key, value []byte) *node {
	if n.kind == NodeNonLeaf {
		left := t.mustLoadLeft(n)
		right := t.mustLoadRight(n)

		if bytes.Compare(key, left.key) <= 0 {
			return n.update(t, func(node *node) {
				left = left.insert(t, key, value)

				node.left = left.id
				node.leftObj = left
				node.sync(t, left, right)
			}).rebalance(t)
		} else {
			return n.update(t, func(node *node) {
				right = right.insert(t, key, value)

				node.right = right.id
				node.rightObj = right
				node.sync(t, left, right)
			}).rebalance(t)
		}
	} else if n.kind == NodeLeafValue {
		if bytes.Equal(key, n.key) {
			return n.update(t, func(node *node) {
				node.value = value
			})
		} else {
			out := n.update(t, func(node *node) {
				node.kind = NodeNonLeaf

				if bytes.Compare(key, n.key) < 0 {
					newLeft := newLeafNode(t, key, value)
					node.left = newLeft.id
					node.leftObj = newLeft
					node.right = n.id
					node.rightObj = n
				} else {
					node.left = n.id
					node.leftObj = n
					newRight := newLeafNode(t, key, value)
					node.right = newRight.id
					node.rightObj = newRight
				}

				node.sync(t, nil, nil)
			})
			return out
		}
	}

	panic(errors.Errorf("avl: on insert, found an unsupported node kind %d", n.kind))
}

func (n *node) lookup(t *Tree, key []byte) ([]byte, bool) {
	if n.kind == NodeLeafValue {
		if bytes.Equal(n.key, key) {
			return n.value, true
		} else {
			return nil, false
		}
	} else if n.kind == NodeNonLeaf {
		child := t.mustLoadLeft(n)

		if bytes.Compare(key, child.key) <= 0 {
			return child.lookup(t, key)
		} else {
			return t.mustLoadRight(n).lookup(t, key)
		}
	}

	panic(errors.Errorf("avl: on lookup, found an unsupported node kind %d", n.kind))
}

func (n *node) iterateFrom(t *Tree, key []byte, callback func(key, value []byte) bool) bool {
	if n.kind == NodeLeafValue {
		if bytes.Compare(key, n.key) <= 0 {
			return callback(n.key, n.value)
		}
		return true
	} else if n.kind == NodeNonLeaf {
		child := t.mustLoadLeft(n)

		if bytes.Compare(key, child.key) <= 0 {
			cont := child.iterateFrom(t, key, callback)
			if !cont {
				return false
			}
		}
		return t.mustLoadRight(n).iterateFrom(t, key, callback)
	}

	panic(errors.Errorf("avl: on lookup, found an unsupported node kind %d", n.kind))
}

func (n *node) delete(t *Tree, key []byte) (*node, bool) {
	if n.kind == NodeLeafValue {
		if bytes.Equal(n.key, key) {
			return nil, true
		} else {
			return n, false
		}
	} else if n.kind == NodeNonLeaf {
		var deleted bool

		left := t.mustLoadLeft(n)
		right := t.mustLoadRight(n)

		if bytes.Compare(key, left.key) <= 0 {
			left, deleted = left.delete(t, key)

			if left == nil {
				return right, deleted
			} else if deleted {
				return n.update(t, func(node *node) {
					node.left = left.id
					node.leftObj = left
					node.sync(t, left, right)
				}).rebalance(t), deleted
			} else {
				return n, deleted
			}
		} else {
			right, deleted = right.delete(t, key)

			if right == nil {
				return left, deleted
			} else if deleted {
				return n.update(t, func(node *node) {
					node.right = right.id
					node.rightObj = right
					node.sync(t, left, right)
				}).rebalance(t), deleted
			} else {
				return n, deleted
			}
		}
	}

	panic(errors.Errorf("avl: on delete, found an unsupported node kind %d", n.kind))
}

func (n *node) rehash() {
	n.id = n.rehashNoWrite()
}

func (n *node) rehashNoWrite() [MerkleHashSize]byte {
	buf := bytebufferpool.Get()
	defer bytebufferpool.Put(buf)
	n.serialize(buf)
	data := buf.Bytes()

	return highwayhash.Sum128(data, hashKey)
}

func (n *node) clone() *node {
	cloned := *n
	return &cloned
}

func (n *node) update(t *Tree, fn func(node *node)) *node {
	cpy := n.clone()
	fn(cpy)
	cpy.viewID = t.viewID
	cpy.rehash()

	if cpy.id != n.id {
		cpy.wroteBack = false
	}

	return cpy
}

func (n *node) getString() string {
	switch n.kind {
	case NodeNonLeaf:
		return "(non-leaf) " + hex.EncodeToString(n.key)
	case NodeLeafValue:
		return fmt.Sprintf("%s -> %s", hex.EncodeToString(n.key), hex.EncodeToString(n.value))
	default:
		return "(unknown)"
	}
}

func (n *node) serializeForDifference(wr io.Writer) error {
	var buf64 [8]byte

	buf := bytebufferpool.Get()
	defer bytebufferpool.Put(buf)

	buf.Write(n.id[:])
	binary.LittleEndian.PutUint64(buf64[:], n.viewID)
	buf.Write(buf64[:])
	buf.WriteByte(byte(n.kind))

	if n.kind == NodeLeafValue {
		if len(n.key) > math.MaxUint32 {
			panic("avl: key is too long")
		}

		binary.LittleEndian.PutUint32(buf64[:4], uint32(len(n.key)))
		buf.Write(buf64[:4])
		buf.Write(n.key)

		if len(n.value) > math.MaxUint32 {
			panic("avl: value is too long")
		}

		binary.LittleEndian.PutUint32(buf64[:4], uint32(len(n.value)))
		buf.Write(buf64[:4])
		buf.Write(n.value)
	} else {
		buf.Write(n.left[:])
		buf.Write(n.right[:])
	}

	_, err := buf.WriteTo(wr)
	return err
}

func (n *node) dfs(t *Tree, allowMissingNodes bool, cb func(*node) (bool, error)) error {
	recurseInto, err := cb(n)
	if err != nil {
		return err
	}
	if !recurseInto {
		return nil
	}
	if n.kind == NodeLeafValue {
		return nil
	}

	left, err := t.loadLeft(n)
	if err != nil {
		if !allowMissingNodes {
			return err
		}
	} else {
		err = left.dfs(t, allowMissingNodes, cb)
		if err != nil {
			return err
		}
	}

	right, err := t.loadRight(n)
	if err != nil {
		if !allowMissingNodes {
			return err
		}
	} else {
		err = right.dfs(t, allowMissingNodes, cb)
		if err != nil {
			return err
		}
	}

	return nil
}

func DeserializeFromDifference(r io.Reader, localViewID uint64) (*node, error) {
	var buf64 [8]byte

	var id [MerkleHashSize]byte
	_, err := r.Read(id[:])
	if err != nil {
		return nil, err
	}

	_, err = r.Read(buf64[:])
	if err != nil {
		return nil, err
	}
	viewID := binary.LittleEndian.Uint64(buf64[:])
	if viewID <= localViewID {
		return nil, errors.New("got view id < local view id")
	}

	_, err = r.Read(buf64[:1])
	if err != nil {
		return nil, err
	}
	kind := nodeType(buf64[0])

	if kind == NodeLeafValue {
		_, err = r.Read(buf64[:4])
		if err != nil {
			return nil, err
		}
		key := make([]byte, binary.LittleEndian.Uint32(buf64[:4]))
		_, err = r.Read(key)
		if err != nil {
			return nil, err
		}

		_, err = r.Read(buf64[:4])
		if err != nil {
			return nil, err
		}
		value := make([]byte, binary.LittleEndian.Uint32(buf64[:4]))
		_, err = r.Read(value)
		if err != nil {
			return nil, err
		}
		return &node{
			id:     id,
			viewID: viewID,
			key:    key,
			value:  value,
			kind:   kind,
		}, nil

	} else if kind == NodeNonLeaf {
		var left, right [MerkleHashSize]byte
		_, err = r.Read(left[:])
		if err != nil {
			return nil, err
		}
		_, err = r.Read(right[:])
		if err != nil {
			return nil, err
		}
		return &node{
			id:     id,
			viewID: viewID,
			kind:   kind,
			left:   left,
			right:  right,
		}, nil
	} else {
		return nil, errors.New("invalid kind")
	}
}

func (n *node) serialize(buf *bytebufferpool.ByteBuffer) {
	buf.WriteByte(byte(n.kind))

	if n.kind != NodeLeafValue {
		buf.Write(n.left[:])
		buf.Write(n.right[:])
	}

	var buf64 [8]byte

	binary.LittleEndian.PutUint64(buf64[:], n.viewID)
	buf.Write(buf64[:])

	// Write key.
	if len(n.key) > math.MaxUint32 {
		panic("avl: key is too long")
	}

	binary.LittleEndian.PutUint32(buf64[:4], uint32(len(n.key)))
	buf.Write(buf64[:4])
	buf.Write(n.key)

	if n.kind == NodeLeafValue {
		// Write value.
		if len(n.value) > math.MaxUint32 {
			panic("avl: value is too long")
		}

		binary.LittleEndian.PutUint32(buf64[:4], uint32(len(n.value)))
		buf.Write(buf64[:4])
		buf.Write(n.value)
	}

	// Write depth.
	buf.WriteByte(n.depth)

	// Write size.
	binary.LittleEndian.PutUint64(buf64[:], n.size)
	buf.Write(buf64[:])
}

func deserialize(r *bytes.Reader) (*node, error) {
	n := new(node)

	kindBuf, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	n.kind = nodeType(kindBuf)

	if n.kind != NodeLeafValue {
		_, err = r.Read(n.left[:])
		if err != nil {
			return nil, err
		}

		_, err = r.Read(n.right[:])
		if err != nil {
			return nil, err
		}
	}

	var buf64 [8]byte

	_, err = r.Read(buf64[:])
	if err != nil {
		return nil, err
	}

	n.viewID = binary.LittleEndian.Uint64(buf64[:])

	// Read key.
	_, err = r.Read(buf64[:4])
	if err != nil {
		return nil, err
	}

	n.key = make([]byte, binary.LittleEndian.Uint32(buf64[:4]))
	_, err = r.Read(n.key)
	if err != nil {
		return nil, err
	}

	if n.kind == NodeLeafValue {
		_, err = r.Read(buf64[:4])
		if err != nil {
			return nil, err
		}

		n.value = make([]byte, binary.LittleEndian.Uint32(buf64[:4]))
		_, err = r.Read(n.value)
		if err != nil {
			return nil, err
		}
	}

	// Read depth.
	n.depth, err = r.ReadByte()
	if err != nil {
		return nil, err
	}

	// Read size.
	_, err = r.Read(buf64[:])
	if err != nil {
		return nil, err
	}

	n.size = binary.LittleEndian.Uint64(buf64[:])

	n.rehash()

	return n, nil
}

func mustDeserialize(r *bytes.Reader) *node {
	n, err := deserialize(r)
	if err != nil {
		panic(err)
	}
	return n
}

// populateDiffs constructs a valid AVL tree from the incoming preloaded tree difference.
func populateDiffs(t *Tree, id [MerkleHashSize]byte, preloaded *diffQueue, visited map[[MerkleHashSize]byte]struct{}, updateNotifier func(key, value []byte)) (*node, error) {
	if t.root != nil && !t.root.wroteBack {
		return nil, errors.New("cannot call populateDiffs() on a dirty tree")
	}

	if _, seen := visited[id]; seen {
		return nil, errors.New("cycle detected")
	}
	visited[id] = struct{}{}

	// Node is not a preloaded diff
	if !preloaded.Has(id) {
		n, err := t.loadNode(id)
		if err != nil {
			return nil, err
		}

		return n, nil
	}

	n, err := preloaded.Dequeue()
	if err != nil {
		return nil, err
	}

	if n.size != 0 || n.depth != 0 {
		panic("BUG: Size != 0 || Depth != 0, possible inconsistency")
	}

	if n.id != id {
		return nil, errors.New("invalid diff order")
	}

	if n.kind == NodeLeafValue {
		n.size = 1
		n.depth = 0
		if n.id != n.rehashNoWrite() {
			return nil, errors.New("hash mismatch")
		}
		if updateNotifier != nil {
			updateNotifier(n.key, n.value)
		}
		return n, nil
	} else if n.kind == NodeNonLeaf {
		leftNode, err := populateDiffs(t, n.left, preloaded, visited, updateNotifier)
		if err != nil {
			return nil, err
		}

		rightNode, err := populateDiffs(t, n.right, preloaded, visited, updateNotifier)
		if err != nil {
			return nil, err
		}

		n.size = leftNode.size + rightNode.size

		newDepth := leftNode.depth
		if rightNode.depth > leftNode.depth {
			newDepth = rightNode.depth
		}

		if newDepth+1 < newDepth {
			return nil, errors.New("depth overflow")
		}

		n.depth = newDepth + 1

		if bytes.Compare(leftNode.key, rightNode.key) > 0 {
			n.key = leftNode.key
		} else {
			n.key = rightNode.key
		}

		balanceFactor := int(leftNode.depth) - int(rightNode.depth)
		if balanceFactor < -1 || balanceFactor > 1 {
			return nil, errors.New("invalid balance factor")
		}

		if n.viewID < leftNode.viewID || n.viewID < rightNode.viewID {
			return nil, errors.New("invalid view id")
		}

		if n.id != n.rehashNoWrite() {
			return nil, errors.New("hash mismatch")
		}

		n.leftObj = leftNode
		n.rightObj = rightNode

		return n, nil
	} else {
		return nil, errors.New("unknown node kind")
	}

}
