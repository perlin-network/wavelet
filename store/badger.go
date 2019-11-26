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
	"os"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/pkg/errors"
)

type badgerWriteBatch struct {
	batch *badger.WriteBatch
}

func (b *badgerWriteBatch) Put(key, value []byte) error {
	k := append([]byte{}, key...)
	v := append([]byte{}, value...)
	return b.batch.Set(k, v)
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

	// Stops gc goroutine when db is closed.
	closeCh chan struct{}
	closeWg sync.WaitGroup
}

func NewBadger(dir string) (*badgerKV, error) { // nolint:golint
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// Explicitly specify compression. Because the default compression with CGO is ZSTD, and without CGO it's Snappy.
	db, err := badger.Open(badger.DefaultOptions(dir).WithLogger(nullLog{}).WithCompression(options.Snappy))
	if err != nil {
		return nil, err
	}

	b := &badgerKV{
		dir:     dir,
		db:      db,
		closeCh: make(chan struct{}),
	}

	b.gc(1 * time.Minute)

	return b, nil
}

func (b *badgerKV) Close() error {
	close(b.closeCh)
	b.closeWg.Wait()

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

	if value == nil {
		value = []byte{}
	}

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

func (b *badgerKV) Dir() string {
	return b.dir
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

func (b *badgerKV) gc(interval time.Duration) {
	b.closeWg.Add(1)

	go func() {
		defer b.closeWg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-b.closeCh:
				return
			case <-ticker.C:
				for {
					// Check for close signal, in case it's closed during GC loop.
					select {
					case <-b.closeCh:
						return
					default:
					}

					// When there's no more GC, an error will be returned.
					err := b.db.RunValueLogGC(0.05)
					if err != nil {
						break
					}
				}
			}
		}
	}()
}

type nullLog struct{}

func (l nullLog) Errorf(f string, v ...interface{})   {}
func (l nullLog) Warningf(f string, v ...interface{}) {}
func (l nullLog) Infof(f string, v ...interface{})    {}
func (l nullLog) Debugf(f string, v ...interface{})   {}
