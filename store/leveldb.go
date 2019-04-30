package store

import (
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var _ WriteBatch = (*leveldbWriteBatch)(nil)

type leveldbWriteBatch struct {
	batch *leveldb.Batch
}

func (b *leveldbWriteBatch) Put(key, value []byte) {
	b.batch.Put(key, value)
}

func (b *leveldbWriteBatch) Clear() {
	b.batch.Reset()
}

func (b *leveldbWriteBatch) Count() int {
	return b.batch.Len()
}

func (b *leveldbWriteBatch) Destroy() {
	b.batch = nil
}

var _ KV = (*leveldbKV)(nil)

type leveldbKV struct {
	dir string
	db  *leveldb.DB
}

func (l *leveldbKV) Close() error {
	return l.db.Close()
}

func (l *leveldbKV) Get(key []byte) ([]byte, error) {
	return l.db.Get(key, nil)
}

func (l *leveldbKV) MultiGet(keys ...[]byte) ([][]byte, error) {
	var bufs = make([][]byte, len(keys))

	for i := range keys {
		b, err := l.Get(keys[i])
		if err != nil {
			return nil, err
		}

		bufs[i] = b
	}

	return bufs, nil
}

func (l *leveldbKV) Put(key, value []byte) error {
	return l.db.Put(key, value, nil)
}

func (l *leveldbKV) NewWriteBatch() WriteBatch {
	return &leveldbWriteBatch{
		batch: &leveldb.Batch{},
	}
}

func (l *leveldbKV) CommitWriteBatch(batch WriteBatch) error {
	wb, ok := batch.(*leveldbWriteBatch)
	if !ok {
		return errors.New("leveldb: not fed in a proper leveldb write batch")
	}

	return l.db.Write(wb.batch, nil)
}

func (l *leveldbKV) Delete(key []byte) error {
	return l.db.Delete(key, nil)
}

func NewLevelDB(dir string) (*leveldbKV, error) {
	opts := &opt.Options{
		Filter:       filter.NewBloomFilter(10),
		NoWriteMerge: true,
	}

	db, err := leveldb.OpenFile(dir, opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Level DB")
	}

	return &leveldbKV{
		dir: dir,
		db:  db,
	}, nil
}
