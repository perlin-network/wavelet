package store

import (
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
	db map[string][]byte
}

func (s *inmemKV) Close() error {
	s.db = make(map[string][]byte)
	return nil
}

func (s *inmemKV) Get(key []byte) ([]byte, error) {
	v, ok := s.db[string(key)]
	if !ok {
		return nil, errors.New("key not found")
	}

	return v, nil
}

func (s *inmemKV) MultiGet(keys ...[]byte) ([][]byte, error) {
	var bufs [][]byte

	for _, key := range keys {
		buf, err := s.Get(key)

		if err != nil {
			return nil, err
		}

		bufs = append(bufs, buf)
	}

	return bufs, nil
}

func (s *inmemKV) Put(key, value []byte) error {
	s.db[string(key)] = value
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
	delete(s.db, string(key))
	return nil
}

func NewInmem() *inmemKV {
	return &inmemKV{db: make(map[string][]byte)}
}
