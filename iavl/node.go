package iavl

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/perlin-network/wavelet/security"

	"github.com/gogo/protobuf/proto"
)

type Node struct {
	Key   string
	Value []byte

	height int
	size   int

	hash []byte

	Left, Right *Node
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (root *Node) balanceFactor() int {
	return root.Left.Height() - root.Right.Height()
}

// Rebalance the tree.
func (root *Node) rebalance() *Node {
	balance := root.balanceFactor()

	if balance > 1 {
		if root.Left.balanceFactor() < 0 {
			root.Left = root.Left.leftRotate()
		}
		return root.rightRotate()
	} else if balance < -1 {
		if root.Right.balanceFactor() > 0 {
			root.Right = root.Right.rightRotate()
		}
		return root.leftRotate()
	} else {
		return root
	}
}

func (root *Node) update() {
	root.height = max(root.Left.Height(), root.Right.Height()) + 1
	root.size = root.Left.Size() + root.Right.Size() + 1
}

func (root *Node) Hash() []byte {
	if root == nil {
		return []byte{0}
	}

	if root.hash != nil {
		return root.hash
	}

	leftHash := root.Left.Hash()
	rightHash := root.Right.Hash()

	// 1024 is chosen to make sure smaller data (tags, lengths) fits in.
	buffer := make([]byte, len(root.Key)+len(root.Value)+len(leftHash)+len(rightHash)+1024)
	cursor := buffer

	n := binary.PutUvarint(cursor, uint64(len(root.Key)))
	cursor = cursor[n:]

	n = copy(cursor, []byte(root.Key))
	cursor = cursor[n:]

	n = binary.PutUvarint(cursor, uint64(len(root.Value)))
	cursor = cursor[n:]

	n = copy(cursor, root.Value)
	cursor = cursor[n:]

	n = binary.PutUvarint(cursor, uint64(root.height))
	cursor = cursor[n:]

	n = binary.PutUvarint(cursor, uint64(root.size))
	cursor = cursor[n:]

	n = copy(cursor, leftHash)
	cursor = cursor[n:]

	n = copy(cursor, rightHash)
	cursor = cursor[n:]

	written := len(buffer) - len(cursor)
	root.hash = append([]byte{1}, security.Hash(buffer[:written])...)

	return root.hash
}

func (root *Node) leftRotate() *Node {
	right := root.Right

	root.Right = right.Left
	right.Left = root

	root.update()
	right.update()

	return right
}

func (root *Node) rightRotate() *Node {
	left := root.Left
	root.Left = left.Right
	left.Right = root

	root.update()
	left.update()

	return left
}

func (root *Node) Height() int {
	if root != nil {
		return int(root.height)
	}
	return 0
}

func (root *Node) Size() int {
	if root != nil {
		return int(root.size)
	}

	return 0
}

func (root *Node) Load(key string) (int, []byte) {
	if root == nil {
		return 0, nil
	}

	if root.Value != nil {
		if root.Key == key {
			return 0, root.Value
		} else {
			return 0, nil
		}
	}

	cmpResult := strings.Compare(key, root.Key)
	if cmpResult == 0 {
		cmpResult = 1
	}

	switch cmpResult {
	case -1:
		index, value := root.Left.Load(key)
		return index + 1, value
	case 1:
		index, value := root.Right.Load(key)
		index += root.Size() - root.Right.Size()
		return index, value
	}

	panic("BUG: unreachable")
}

func (root *Node) LoadAt(index int) (key string, value []byte) {
	if root == nil {
		return
	}

	if index == 0 {
		return root.Key, root.Value
	}

	if index < root.Size() {
		return root.Left.LoadAt(index)
	}

	return root.Right.LoadAt(index - root.Size())
}

func newNode(key string, value []byte) *Node {
	node := &Node{
		Key:   key,
		Value: value,
	}
	node.update()
	return node
}

func (root *Node) Store(key string, value []byte) (*Node, []byte) {
	if value == nil {
		panic("cannot store nil value")
	}

	var original []byte
	root = root.doStore(key, value, &original)
	return root, original
}

// It is up to the caller to ensure value != nil.
func (root *Node) doStore(key string, value []byte, original *[]byte) *Node {
	if root == nil {
		return newNode(key, value)
	}

	root.hash = nil

	cmpResult := strings.Compare(key, root.Key)
	if cmpResult == 0 {
		if root.Key == key && root.Value != nil {
			if *original != nil {
				panic("BUG: attempting to set original value twice")
			}
			*original = root.Value
			root.Value = value
			return root
		}
		cmpResult = 1
	}

	switch cmpResult {
	case -1:
		needFixup := false
		if root.Left == nil && root.Value != nil {
			needFixup = true
		}

		root.Left = root.Left.doStore(key, value, original)

		if needFixup {
			if root.Right != nil {
				panic("BUG: internal state corrupted")
			}
			root.Right = newNode(root.Key, root.Value)
			root.Value = nil
		}
	case 1:
		needFixup := false
		if root.Right == nil && root.Value != nil {
			needFixup = true
		}

		root.Right = root.Right.doStore(key, value, original)

		if needFixup {
			if root.Left != nil {
				panic("BUG: internal state corrupted")
			}
			root.Left = newNode(root.Key, root.Value)
			root.Key = key
			root.Value = nil
		}
	}

	root.update()
	return root.rebalance()
}

func (root *Node) Range(callback func(key string, value []byte)) {
	if root == nil {
		return
	}
	root.Left.Range(callback)
	if root.Value != nil {
		callback(root.Key, root.Value)
	}
	root.Right.Range(callback)
}

func (root *Node) Delete(key string) (*Node, bool) {
	deleted := false
	root = root.doDelete(key, &deleted)
	return root, deleted
}

func (root *Node) doDelete(key string, deleted *bool) *Node {
	if root == nil {
		return root
	}

	root.hash = nil

	switch strings.Compare(key, root.Key) {
	case -1:
		root.Left = root.Left.doDelete(key, deleted)
	case 0:
		if root.Value != nil {
			if *deleted {
				panic("BUG: attempting to delete twice")
			}
			*deleted = true
			return nil
		} else {
			root.Right = root.Right.doDelete(key, deleted)
		}
	case 1:
		root.Right = root.Right.doDelete(key, deleted)
	}

	if root.Value == nil {
		if root.Left == nil {
			return root.Right
		} else if root.Right == nil {
			return root.Left
		}
	}

	root.update()
	return root.rebalance()
}

func (root *Node) Print(level int) {
	for i := 0; i < level; i++ {
		fmt.Printf(" ")
	}
	if root == nil {
		fmt.Println("/")
		return
	}

	fmt.Printf("%s -> %s\n", root.Key, string(root.Value))
	root.Left.Print(level + 1)
	root.Right.Print(level + 1)
}

func (root *Node) JSON() map[string]interface{} {
	data := make(map[string]interface{})
	root.Range(func(key string, value []byte) {
		data[key] = value
	})

	return data
}

func (root *Node) Marshal() []byte {
	if root == nil {
		return nil
	}

	msg := root.buildMessage()
	bytes, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return bytes
}

func (root *Node) buildMessage() *Tree {
	if root == nil {
		return nil
	}

	return &Tree{
		Key:    root.Key,
		Value:  root.Value,
		Height: uint32(root.height),
		Size_:  uint64(root.size),
		Left:   root.Left.buildMessage(),
		Right:  root.Right.buildMessage(),
	}
}

func Unmarshal(bytes []byte) (*Node, error) {
	var msg Tree
	err := proto.Unmarshal(bytes, &msg)
	if err != nil {
		return nil, err
	}

	return buildTree(&msg), nil
}

func buildTree(msg *Tree) *Node {
	if msg == nil {
		return nil
	}

	return &Node{
		Key:    msg.Key,
		Value:  msg.Value,
		height: int(msg.Height),
		size:   int(msg.Size_),
		Left:   buildTree(msg.Left),
		Right:  buildTree(msg.Right),
	}
}
