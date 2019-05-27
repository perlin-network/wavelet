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

package wavelet

import (
	"container/list"
	"golang.org/x/crypto/blake2b"
	"sync"
)

type LRU struct {
	sync.Mutex

	size int

	elements map[[blake2b.Size256]byte]*list.Element
	access   *list.List // *objectInfo
}

type objectInfo struct {
	key [blake2b.Size256]byte
	obj interface{}
}

func NewLRU(size int) *LRU {
	return &LRU{
		size:     size,
		elements: make(map[[blake2b.Size256]byte]*list.Element, size),
		access:   list.New(),
	}
}

func (l *LRU) load(key [blake2b.Size256]byte) (interface{}, bool) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]
	if !ok {
		return nil, false
	}

	l.access.MoveToFront(elem)
	return elem.Value.(*objectInfo).obj, ok
}

func (l *LRU) put(key [blake2b.Size256]byte, val interface{}) {
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

func (l *LRU) remove(key [blake2b.Size256]byte) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]
	if ok {
		delete(l.elements, key)
		l.access.Remove(elem)
	}
}

func (l *LRU) mostRecentlyUsed(n int) [][blake2b.Size256]byte {
	l.Lock()
	defer l.Unlock()

	out := make([][blake2b.Size256]byte, 0)

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
