package avl

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/pkg/errors"
	"math"
)

const MerkleHashSize = 16

type nodeType byte

const (
	NodeNonLeaf nodeType = iota
	NodeLeafValue
)

type node struct {
	id, left, right [MerkleHashSize]byte

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
		kind:  NodeLeafValue,
		depth: 0,
		size:  1,
	}

	n.rehash()

	if t != nil {
		t.queueWrite(n)
	}

	return n
}

func (n *node) balanceFactor(t *Tree, left *node, right *node) int {
	if left == nil {
		left = t.loadNode(n.left)
	}

	if right == nil {
		right = t.loadNode(n.right)
	}

	return int(left.depth) - int(right.depth)
}

func (n *node) sync(t *Tree, left *node, right *node) {
	if left == nil {
		left = t.loadNode(n.left)
	}

	if right == nil {
		right = t.loadNode(n.right)
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
	right := t.loadNode(n.right)

	n = n.update(t, func(node *node) {
		node.right = right.left
		node.sync(t, nil, nil)
	})

	right = right.update(t, func(node *node) {
		node.left = n.id
		node.sync(t, nil, nil)
	})

	return right
}

func (n *node) rightRotate(t *Tree) *node {
	left := t.loadNode(n.left)

	n = n.update(t, func(node *node) {
		node.left = left.right
		node.sync(t, nil, nil)
	})

	left = left.update(t, func(node *node) {
		node.right = n.id
		node.sync(t, nil, nil)
	})

	return left
}

func (n *node) rebalance(t *Tree) *node {
	left := t.loadNode(n.left)
	right := t.loadNode(n.right)

	balance := n.balanceFactor(t, left, right)

	if balance > 1 {
		if left.balanceFactor(t, nil, nil) < 0 {
			n = n.update(t, func(node *node) {
				node.left = left.leftRotate(t).id
			})
		}

		return n.rightRotate(t)
	} else if balance < -1 {
		if right.balanceFactor(t, nil, nil) > 0 {
			n = n.update(t, func(node *node) {
				node.right = right.rightRotate(t).id
			})
		}

		return n.leftRotate(t)
	}

	return n
}

func (n *node) insert(t *Tree, key, value []byte) *node {
	if n.kind == NodeNonLeaf {
		left := t.loadNode(n.left)
		right := t.loadNode(n.right)

		if bytes.Compare(key, left.key) <= 0 {
			return n.update(t, func(node *node) {
				left = left.insert(t, key, value)

				node.left = left.id
				node.sync(t, left, right)
			}).rebalance(t)
		} else {
			return n.update(t, func(node *node) {
				right = right.insert(t, key, value)

				node.right = right.id
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
					node.left = newLeafNode(t, key, value).id
					node.right = n.id
				} else {
					node.left = n.id
					node.right = newLeafNode(t, key, value).id
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
		child := t.loadNode(n.left)

		if bytes.Compare(key, child.key) <= 0 {
			return child.lookup(t, key)
		} else {
			return t.loadNode(n.right).lookup(t, key)
		}
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

		left := t.loadNode(n.left)
		right := t.loadNode(n.right)

		if bytes.Compare(key, left.key) <= 0 {
			left, deleted = left.delete(t, key)

			if left == nil {
				return right, deleted
			} else if deleted {
				return n.update(t, func(node *node) {
					node.left = left.id
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
	var buf bytes.Buffer
	n.serialize(&buf)

	n.id = md5.Sum(buf.Bytes())
}

func (n *node) clone() *node {
	clone := &node{
		id:    n.id,
		left:  n.left,
		right: n.right,

		key:   make([]byte, len(n.key)),
		value: make([]byte, len(n.value)),

		kind:  n.kind,
		depth: n.depth,
		size:  n.size,
	}

	copy(clone.key, n.key)
	copy(clone.value, n.value)

	return clone
}

func (n *node) update(t *Tree, fn func(node *node)) *node {
	cpy := n.clone()
	fn(cpy)
	cpy.rehash()

	if cpy.id != n.id {
		cpy.viewID = t.ViewID
		t.queueWrite(cpy)
	}

	return cpy
}

func (n *node) String() string {
	switch n.kind {
	case NodeNonLeaf:
		return "(non-leaf)" + hex.EncodeToString(n.key)
	case NodeLeafValue:
		return fmt.Sprintf("%s -> %s", hex.EncodeToString(n.key), hex.EncodeToString(n.value))
	default:
		return "(unknown)"
	}
}

func (n *node) serialize(buf *bytes.Buffer) {
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

func deserialize(buf []byte) *node {
	r := bytes.NewReader(buf)

	n := new(node)

	kindBuf, err := r.ReadByte()
	if err != nil {
		panic(err)
	}
	n.kind = nodeType(kindBuf)

	if n.kind != NodeLeafValue {
		_, err = r.Read(n.left[:])
		if err != nil {
			panic(err)
		}

		_, err = r.Read(n.right[:])
		if err != nil {
			panic(err)
		}
	}

	var buf64 [8]byte

	_, err = r.Read(buf64[:])
	if err != nil {
		panic(err)
	}

	n.viewID = binary.LittleEndian.Uint64(buf64[:])

	// Read key.
	_, err = r.Read(buf64[:4])
	if err != nil {
		panic(err)
	}

	n.key = make([]byte, binary.LittleEndian.Uint32(buf64[:4]))
	_, err = r.Read(n.key)
	if err != nil {
		panic(err)
	}

	if n.kind == NodeLeafValue {
		_, err = r.Read(buf64[:4])
		if err != nil {
			panic(err)
		}

		n.value = make([]byte, binary.LittleEndian.Uint32(buf64[:4]))
		_, err = r.Read(n.value)
		if err != nil {
			panic(err)
		}
	}

	// Read depth.
	n.depth, err = r.ReadByte()
	if err != nil {
		panic(err)
	}

	// Read size.
	_, err = r.Read(buf64[:])
	if err != nil {
		panic(err)
	}

	n.size = binary.LittleEndian.Uint64(buf64[:])

	n.rehash()

	return n
}
