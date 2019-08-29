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
	"time"
)

type LRU struct {
	sync.Mutex

	size int

	elements map[interface{}]*list.Element
	access   *list.List // *objectInfo

	expiration time.Duration // 0 for no expiration check
}

type objectInfo struct {
	key        interface{}
	obj        interface{}
	updateTime time.Time
}

func NewLRU(size int) *LRU {
	return &LRU{
		size:     size,
		elements: make(map[interface{}]*list.Element, size),
		access:   list.New(),
	}
}

func (l *LRU) SetExpiration(d time.Duration) {
	l.Lock()
	l.expiration = d
	l.Unlock()
}

func (l *LRU) cleanupExpiredItemsLocked() {
	if l.expiration == 0 {
		return
	}

	currentTime := time.Now()
	for it := l.access.Back(); it != nil; {
		info := it.Value.(*objectInfo)
		oldIt := it
		it = it.Prev()

		if currentTime.Sub(info.updateTime) < l.expiration {
			break
		}

		l.access.Remove(oldIt)
		delete(l.elements, info.key)
	}
}

func (l *LRU) Load(key interface{}) (interface{}, bool) {
	l.Lock()
	defer l.Unlock()

	l.cleanupExpiredItemsLocked()

	elem, ok := l.elements[key]
	if !ok {
		return nil, false
	}

	objInfo := elem.Value.(*objectInfo)
	objInfo.updateTime = time.Now()
	l.access.MoveToFront(elem)

	return objInfo.obj, ok
}

func (l *LRU) LoadOrPut(key interface{}, val interface{}) (interface{}, bool) {
	l.Lock()
	defer l.Unlock()

	l.cleanupExpiredItemsLocked()

	elem, ok := l.elements[key]

	if ok {
		objInfo := elem.Value.(*objectInfo)
		objInfo.updateTime = time.Now()
		val = objInfo.obj
		l.access.MoveToFront(elem)
	} else {
		l.elements[key] = l.access.PushFront(&objectInfo{
			key:        key,
			obj:        val,
			updateTime: time.Now(),
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

	l.cleanupExpiredItemsLocked()

	elem, ok := l.elements[key]

	if ok {
		objInfo := elem.Value.(*objectInfo)
		objInfo.updateTime = time.Now()
		objInfo.obj = val
		l.access.MoveToFront(elem)
	} else {
		l.elements[key] = l.access.PushFront(&objectInfo{
			key:        key,
			obj:        val,
			updateTime: time.Now(),
		})
		for len(l.elements) > l.size {
			back := l.access.Back()
			info := back.Value.(*objectInfo)
			delete(l.elements, info.key)
			l.access.Remove(back)
		}
	}
}

func (l *LRU) Remove(key interface{}) {
	l.Lock()
	defer l.Unlock()

	l.cleanupExpiredItemsLocked()

	elem, ok := l.elements[key]
	if ok {
		delete(l.elements, key)
		l.access.Remove(elem)
	}
}
