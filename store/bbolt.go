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

func (b *bboltWriteBatch) Put(key, value []byte) error {
	k := append([]byte{}, key...)
	v := append([]byte{}, value...)
	b.puts = append(b.puts, bboltPut{k, v})
	return nil
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
