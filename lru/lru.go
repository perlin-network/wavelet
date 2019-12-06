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

	"github.com/mauricelam/genny/generic"
)

//nolint:lll
//go:generate genny -in=$GOFILE -out=../lru.state.go -pkg wavelet gen "KeyType=[32]byte ValueType=*CollapseState NameType=State"

//nolint:lll
//go:generate genny -in=$GOFILE -out=../avl/lru.node.go -pkg avl gen "KeyType=[16]byte ValueType=*node NameType=Node"

//nolint:lll
//go:generate genny -in=$GOFILE -out=../lru.vm.go -pkg wavelet gen "KeyType=[32]byte ValueType=*exec.VirtualMachine NameType=VM"

type KeyType generic.Type
type ValueType generic.Type
type NameType generic.Type

type NameTypeLRU struct {
	sync.Mutex

	size int

	elements map[KeyType]*list.Element
	access   *list.List
}

type objectInfoNameType struct {
	key KeyType
	obj ValueType
}

func NewNameTypeLRU(size int) *NameTypeLRU {
	return &NameTypeLRU{
		size:     size,
		elements: make(map[KeyType]*list.Element, size),
		access:   list.New(),
	}
}

func (l *NameTypeLRU) Load(key KeyType) (ValueType, bool) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]
	if !ok {
		return nil, false
	}

	l.access.MoveToFront(elem)

	return elem.Value.(*objectInfoNameType).obj, ok
}

func (l *NameTypeLRU) LoadOrPut(key KeyType, val ValueType) (ValueType, bool) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]

	if ok {
		val = elem.Value.(*objectInfoNameType).obj
		l.access.MoveToFront(elem)
	} else {
		l.elements[key] = l.access.PushFront(&objectInfoNameType{
			key: key,
			obj: val,
		})
		for len(l.elements) > l.size {
			back := l.access.Back()
			info := back.Value.(*objectInfoNameType)
			delete(l.elements, info.key)
			l.access.Remove(back)
		}
	}

	return val, ok
}

func (l *NameTypeLRU) Put(key KeyType, val ValueType) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]

	if ok {
		elem.Value.(*objectInfoNameType).obj = val
		l.access.MoveToFront(elem)
	} else {
		l.elements[key] = l.access.PushFront(&objectInfoNameType{
			key: key,
			obj: val,
		})
		for len(l.elements) > l.size {
			back := l.access.Back()
			info := back.Value.(*objectInfoNameType)
			delete(l.elements, info.key)
			l.access.Remove(back)
		}
	}
}

func (l *NameTypeLRU) Remove(key KeyType) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]
	if ok {
		delete(l.elements, key)
		l.access.Remove(elem)
	}
}
