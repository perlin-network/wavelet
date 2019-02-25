package store

import (
	"bytes"
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
	k := make([]byte, len(key))
	v := make([]byte, len(value))

	copy(k, key)
	copy(v, value)

	b.pairs = append(b.pairs, kvPair{key: k, value: v})
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
	db *skiplist.SkipList
}

func (s *inmemKV) Close() error {
	s.db.Init()
	s.db = nil
	return nil
}

func (s *inmemKV) Get(key []byte) ([]byte, error) {
	buf, found := s.db.GetValue(key)
	if !found {
		return nil, errors.New("key not found")
	}

	return buf.([]byte), nil
}

func (s *inmemKV) MultiGet(keys ...[]byte) ([][]byte, error) {
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
	_ = s.db.Set(key, value)
	return nil
}

func (s *inmemKV) NewWriteBatch() WriteBatch {
	return new(inmemWriteBatch)
}

func (s *inmemKV) CommitWriteBatch(batch WriteBatch) error {
	if wb, ok := batch.(*inmemWriteBatch); ok {
		for _, pair := range wb.pairs {
			err := s.Put(pair.key, pair.value)

			if err != nil {
				return errors.Wrap(err, "inmem: failed to commit write batch")
			}
		}

		return nil
	}

	return errors.New("inmem: not fed in a proper in-memory write batch")
}

func (s *inmemKV) Delete(key []byte) error {
	_ = s.db.Remove(key)
	return nil
}

func NewInmem() *inmemKV {
	var comparator skiplist.GreaterThanFunc = func(lhs, rhs interface{}) bool {
		return bytes.Compare(lhs.([]byte), rhs.([]byte)) == 1
	}

	return &inmemKV{db: skiplist.New(comparator)}
}
