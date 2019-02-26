package avl

import (
	"container/list"
	"sync"
)

type lru struct {
	sync.Mutex

	size int

	elements map[[MerkleRootSize]byte]*list.Element
	access   *list.List // *objectInfo
}

type objectInfo struct {
	key [MerkleRootSize]byte
	obj interface{}
}

func newLRU(size int) *lru {
	return &lru{
		size:     size,
		elements: make(map[[MerkleRootSize]byte]*list.Element),
		access:   list.New(),
	}
}

func (l *lru) load(key [MerkleRootSize]byte) (interface{}, bool) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]
	if !ok {
		return nil, false
	}

	l.access.MoveToFront(elem)
	return elem.Value.(*objectInfo).obj, ok
}

func (l *lru) put(key [MerkleRootSize]byte, val interface{}) {
	l.Lock()
	defer l.Unlock()

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

func (l *lru) remove(key [MerkleRootSize]byte) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]
	if ok {
		delete(l.elements, key)
		l.access.Remove(elem)
	}
}

func (l *lru) mostRecentlyUsed(n int) [][MerkleRootSize]byte {
	l.Lock()
	defer l.Unlock()

	out := make([][MerkleRootSize]byte, 0)

	current := l.access.Front()
	for current != nil {
		out = append(out, current.Value.(*objectInfo).key)
		if len(out) == n {
			break
		}
		current = current.Next()
	}

	return out
}
