package avl

import (
	"container/list"
)

type lru struct {
	size int

	elements map[[MerkleHashSize]byte]*list.Element
	access   *list.List // *objectInfo
}

type objectInfo struct {
	key [MerkleHashSize]byte
	obj interface{}
}

func newLRU(size int) *lru {
	return &lru{
		size:     size,
		elements: make(map[[MerkleHashSize]byte]*list.Element, size),
		access:   list.New(),
	}
}

func (l *lru) load(key [MerkleHashSize]byte) (interface{}, bool) {
	elem, ok := l.elements[key]
	if !ok {
		return nil, false
	}

	l.access.MoveToFront(elem)
	return elem.Value.(*objectInfo).obj, ok
}

func (l *lru) put(key [MerkleHashSize]byte, val interface{}) {
	elem, ok := l.elements[key]

	if ok {
		elem.Value.(*objectInfo).obj = val
		l.access.MoveToFront(elem)
	} else {
		l.elements[key] = l.access.PushFront(&objectInfo{
			key: key,
			obj: val,
		})
	}

	for len(l.elements) > l.size {
		back := l.access.Back()
		info := back.Value.(*objectInfo)
		delete(l.elements, info.key)
		l.access.Remove(back)
	}
}

func (l *lru) remove(key [MerkleHashSize]byte) {
	elem, ok := l.elements[key]
	if ok {
		delete(l.elements, key)
		l.access.Remove(elem)
	}
}
