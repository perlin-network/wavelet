package store

import (
	"fmt"
	"os"
	"path"

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

var bucketName = []byte("wavelet")

type bboltPut struct {
	key, value []byte
}

type bboltWriteBatch struct {
	puts []bboltPut
}

func (b *bboltWriteBatch) Put(key, value []byte) {
	b.puts = append(b.puts, bboltPut{key, value})
}

func (b *bboltWriteBatch) Clear() {
	b.puts = []bboltPut{}
}

func (b *bboltWriteBatch) Count() int {
	return len(b.puts)
}

func (b *bboltWriteBatch) Destroy() {
	// Do nothing
}

type bboltKV struct {
	dir string
	db  *bolt.DB
}

func NewBbolt(dir string) (*bboltKV, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	db, err := bolt.Open(path.Join(dir, "wavelet.db"), 0600, nil)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return &bboltKV{
		dir: dir,
		db:  db,
	}, nil
}

func (b *bboltKV) Close() error {
	return b.db.Close()
}

func (b *bboltKV) Get(key []byte) ([]byte, error) {
	var value []byte
	err := b.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		if b == nil {
			return fmt.Errorf("bucket %s does not exist", bucketName)
		}

		v := b.Get(key)
		if v == nil {
			return fmt.Errorf("key %s does not exist", key)
		}

		value = append([]byte{}, v...)
		return nil
	})

	return value, err
}

func (b *bboltKV) MultiGet(keys ...[]byte) ([][]byte, error) {
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

func (b *bboltKV) Put(key, value []byte) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		if b == nil {
			return fmt.Errorf("bucket %s does not exist", bucketName)
		}

		return b.Put(key, value)
	})
}

func (b *bboltKV) Delete(key []byte) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		if b == nil {
			return fmt.Errorf("bucket %s does not exist", bucketName)
		}

		return b.Delete(key)
	})
}

func (b *bboltKV) NewWriteBatch() WriteBatch {
	return &bboltWriteBatch{
		puts: make([]bboltPut, 0),
	}
}

func (b *bboltKV) CommitWriteBatch(batch WriteBatch) error {
	wb, ok := batch.(*bboltWriteBatch)
	if !ok {
		return errors.New("bbolt: not fed in a proper bbolt write batch")
	}

	return b.db.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		if b == nil {
			return fmt.Errorf("bucket %s does not exist", bucketName)
		}

		for _, p := range wb.puts {
			if err := b.Put(p.key, p.value); err != nil {
				return err
			}
		}

		return nil
	})
}
