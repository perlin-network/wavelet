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

package lru

import (
	"container/list"
	"sync"
)

type LRU struct {
	sync.Mutex

	size int

	elements map[interface{}]*list.Element
	access   *list.List // *objectInfo
}

type objectInfo struct {
	key interface{}
	obj interface{}
}

func NewLRU(size int) *LRU {
	return &LRU{
		size:     size,
		elements: make(map[interface{}]*list.Element, size),
		access:   list.New(),
	}
}

func (l *LRU) Load(key interface{}) (interface{}, bool) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]
	if !ok {
		return nil, false
	}

	l.access.MoveToFront(elem)
	return elem.Value.(*objectInfo).obj, ok
}

func (l *LRU) LoadOrPut(key interface{}, val interface{}) (interface{}, bool) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]

	if ok {
		val = elem.Value.(*objectInfo).obj
		l.access.MoveToFront(elem)
	} else {
		l.elements[key] = l.access.PushFront(&objectInfo{
			key: key,
			obj: val,
		})
		for len(l.elements) > l.size {
			back := l.access.Back()
			info := back.Value.(*objectInfo)
			delete(l.elements, info.key)
			l.access.Remove(back)
		}
	}

	return val, ok
}

func (l *LRU) Put(key interface{}, val interface{}) {
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
		for len(l.elements) > l.size {
			back := l.access.Back()
			info := back.Value.(*objectInfo)
			delete(l.elements, info.key)
			l.access.Remove(back)
		}
	}
}

func (l *LRU) PutWithEvictCallback(key interface{}, val interface{}, onEvict func(key interface{}, val interface{})) {
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
		for len(l.elements) > l.size {
			back := l.access.Back()
			info := back.Value.(*objectInfo)
			delete(l.elements, info.key)
			l.access.Remove(back)

			if onEvict != nil {
				onEvict(info.key, info.obj)
			}
		}
	}
}

func (l *LRU) Remove(key interface{}) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]
	if ok {
		delete(l.elements, key)
		l.access.Remove(elem)
	}
}
