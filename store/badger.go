package store

import (
	badger "github.com/dgraph-io/badger"
	"github.com/pkg/errors"
)

type badgerWriteBatch struct {
	db    *badgerKV
	batch *badger.WriteBatch
}

func (b *badgerWriteBatch) Put(key, value []byte) {
	err := b.batch.Set(key, value)
	if err != nil {
		panic(err)
	}
}

func (b *badgerWriteBatch) Clear() {
	panic("not supported")
}

func (b *badgerWriteBatch) Count() int {
	panic("not supported")
}

func (b *badgerWriteBatch) Destroy() {
	b.batch.Cancel()
}

type badgerKV struct {
	dir string
	db  *badger.DB
}

func NewBadger(dir string) (*badgerKV, error) {
	db, err := badger.Open(badger.DefaultOptions(dir).WithLogger(nullLog{}))
	if err != nil {
		return nil, err
	}

	return &badgerKV{
		dir: dir,
		db:  db,
	}, nil
}

func (b *badgerKV) Close() error {
	return b.db.Close()
}

func (b *badgerKV) Get(key []byte) ([]byte, error) {
	var value []byte
	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		value, err = item.ValueCopy(nil)
		return err
	})

	return value, err
}

func (b *badgerKV) MultiGet(keys ...[]byte) ([][]byte, error) {
	var bufs = make([][]byte, len(keys))

	for i := range keys {
		b, err := b.Get(keys[i])
		if err != nil {
			return nil, err
		}

		bufs[i] = b
	}

	return bufs, nil
}

func (b *badgerKV) Put(key, value []byte) error {
	return b.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})
}

func (b *badgerKV) Delete(key []byte) error {
	return b.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

func (b *badgerKV) NewWriteBatch() WriteBatch {
	return &badgerWriteBatch{
		batch: b.db.NewWriteBatch(),
	}
}

func (b *badgerKV) CommitWriteBatch(batch WriteBatch) error {
	wb, ok := batch.(*badgerWriteBatch)
	if !ok {
		return errors.New("badger: not fed in a proper badger write batch")
	}

	return wb.batch.Flush()
}

type nullLog struct {
}

func (l nullLog) Errorf(f string, v ...interface{}) {
}

func (l nullLog) Warningf(f string, v ...interface{}) {
}

func (l nullLog) Infof(f string, v ...interface{}) {
}

func (l nullLog) Debugf(f string, v ...interface{}) {
}
