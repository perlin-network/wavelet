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

package store

import (
	"bytes"
	"sync"

	"github.com/huandu/skiplist"
	"github.com/pkg/errors"
)

type kvPair struct {
	key, value []byte
}

var _ WriteBatch = (*inmemWriteBatch)(nil)

type inmemWriteBatch struct {
	pairs []kvPair
}

func (b *inmemWriteBatch) Put(key, value []byte) {
	b.pairs = append(b.pairs, kvPair{key: key, value: value})
}

func (b *inmemWriteBatch) Clear() {
	b.pairs = make([]kvPair, 0)
}

func (b *inmemWriteBatch) Count() int {
	return len(b.pairs)
}

func (b *inmemWriteBatch) Destroy() {
	b.pairs = nil
}

var _ KV = (*inmemKV)(nil)

type inmemKV struct {
	sync.RWMutex
	db *skiplist.SkipList
}

func (s *inmemKV) Close() error {
	s.Lock()
	defer s.Unlock()

	// Do nothing if already closed
	if s.db == nil {
		return nil
	}

	s.db.Init()
	s.db = nil
	return nil
}

func (s *inmemKV) Get(key []byte) ([]byte, error) {
	s.RLock()
	defer s.RUnlock()

	buf, found := s.db.GetValue(key)
	if !found {
		return nil, errors.New("key not found")
	}

	return buf.([]byte), nil
}

func (s *inmemKV) MultiGet(keys ...[]byte) ([][]byte, error) {
	s.RLock()
	defer s.RUnlock()

	var bufs [][]byte

	for _, key := range keys {
		buf, found := s.db.GetValue(key)
		if !found {
			return nil, errors.New("key not found")
		}

		bufs = append(bufs, buf.([]byte))
	}

	return bufs, nil
}

func (s *inmemKV) Put(key, value []byte) error {
	s.Lock()
	defer s.Unlock()

	_ = s.db.Set(key, value)
	return nil
}

var (
	writeBatchPool = sync.Pool{
		New: func() interface{} {
			return new(inmemWriteBatch)
		},
	}
)

func (s *inmemKV) NewWriteBatch() WriteBatch {
	return writeBatchPool.Get().(WriteBatch)
}

func (s *inmemKV) CommitWriteBatch(batch WriteBatch) error {
	s.Lock()
	defer s.Unlock()

	if wb, ok := batch.(*inmemWriteBatch); ok {
		for _, pair := range wb.pairs {
			_ = s.db.Set(pair.key, pair.value)
		}

		writeBatchPool.Put(wb)
		return nil
	}

	return errors.New("inmem: not fed in a proper in-memory write batch")
}

func (s *inmemKV) Delete(key []byte) error {
	s.Lock()
	defer s.Unlock()

	_ = s.db.Remove(key)
	return nil
}

func (s *inmemKV) Dir() string {
	return ""
}

func NewInmem() *inmemKV {
	var comparator skiplist.GreaterThanFunc = func(lhs, rhs interface{}) bool {
		return bytes.Compare(lhs.([]byte), rhs.([]byte)) == 1
	}

	return &inmemKV{db: skiplist.New(comparator)}
}
