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
	"sync"

	"github.com/perlin-network/noise/edwards25519"
)

type PeerBlockLRU struct {
	sync.Mutex

	size int

	elements map[edwards25519.PublicKey]*list.Element
	access   *list.List
}

type objectInfoPeerBlock struct {
	key edwards25519.PublicKey
	obj *Block
}

func NewPeerBlockLRU(size int) *PeerBlockLRU {
	return &PeerBlockLRU{
		size:     size,
		elements: make(map[edwards25519.PublicKey]*list.Element, size),
		access:   list.New(),
	}
}

func (l *PeerBlockLRU) Load(key edwards25519.PublicKey) (*Block, bool) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]
	if !ok {
		return nil, false
	}

	l.access.MoveToFront(elem)

	return elem.Value.(*objectInfoPeerBlock).obj, ok
}

func (l *PeerBlockLRU) LoadOrPut(key edwards25519.PublicKey, val *Block) (*Block, bool) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]

	if ok {
		val = elem.Value.(*objectInfoPeerBlock).obj
		l.access.MoveToFront(elem)
	} else {
		l.elements[key] = l.access.PushFront(&objectInfoPeerBlock{
			key: key,
			obj: val,
		})
		for len(l.elements) > l.size {
			back := l.access.Back()
			info := back.Value.(*objectInfoPeerBlock)
			delete(l.elements, info.key)
			l.access.Remove(back)
		}
	}

	return val, ok
}

func (l *PeerBlockLRU) Put(key edwards25519.PublicKey, val *Block) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]

	if ok {
		elem.Value.(*objectInfoPeerBlock).obj = val
		l.access.MoveToFront(elem)
	} else {
		l.elements[key] = l.access.PushFront(&objectInfoPeerBlock{
			key: key,
			obj: val,
		})
		for len(l.elements) > l.size {
			back := l.access.Back()
			info := back.Value.(*objectInfoPeerBlock)
			delete(l.elements, info.key)
			l.access.Remove(back)
		}
	}
}

func (l *PeerBlockLRU) Remove(key edwards25519.PublicKey) {
	l.Lock()
	defer l.Unlock()

	elem, ok := l.elements[key]
	if ok {
		delete(l.elements, key)
		l.access.Remove(elem)
	}
}
