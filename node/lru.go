package node

import (
	"container/list"
	"sync"
)

const blake2bHashSize = 32

type lru struct {
	sync.Mutex

	size int

	elements map[[blake2bHashSize]byte]*list.Element
	access   *list.List // *objectInfo
}

type objectInfo struct {
	key [blake2bHashSize]byte
	obj interface{}
}

func newLRU(size int) *lru {
	return &lru{
		size:     size,
		elements: make(map[[blake2bHashSize]byte]*list.Element),
		access:   list.New(),
	}
}

func (l *lru) load(key [blake2bHashSize]byte) (interface{}, bool) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]
	if !ok {
		return nil, false
	}

	l.access.MoveToFront(elem)
	return elem.Value.(*objectInfo).obj, ok
}

func (l *lru) put(key [blake2bHashSize]byte, val interface{}) {
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

func (l *lru) remove(key [blake2bHashSize]byte) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]
	if ok {
		delete(l.elements, key)
		l.access.Remove(elem)
	}
}

func (l *lru) mostRecentlyUsed(n int) [][blake2bHashSize]byte {
	l.Lock()
	defer l.Unlock()

	out := make([][blake2bHashSize]byte, 0)

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
