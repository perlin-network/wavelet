package store

import (
	"bytes"
	"github.com/huandu/skiplist"
	"github.com/pkg/errors"
	"sync"
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

func (s *inmemKV) NewWriteBatch() WriteBatch {
	return new(inmemWriteBatch)
}

func (s *inmemKV) CommitWriteBatch(batch WriteBatch) error {
	s.Lock()
	defer s.Unlock()

	if wb, ok := batch.(*inmemWriteBatch); ok {
		for _, pair := range wb.pairs {
			_ = s.db.Set(pair.key, pair.value)
		}

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

func NewInmem() *inmemKV {
	var comparator skiplist.GreaterThanFunc = func(lhs, rhs interface{}) bool {
		return bytes.Compare(lhs.([]byte), rhs.([]byte)) == 1
	}

	return &inmemKV{db: skiplist.New(comparator)}
}
