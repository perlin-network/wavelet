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

package store // nolint:dupl

import (
	"crypto/rand"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func BenchmarkBadger(b *testing.B) {
	path := "badger"
	_ = os.RemoveAll(path)

	b.StopTimer()

	db, err := NewBadger(path)
	assert.NoError(b, err)
	defer func() {
		_ = db.Close()
		_ = os.RemoveAll(path)
	}()

	b.StartTimer()
	defer b.StopTimer()

	for i := 0; i < b.N; i++ {
		var randomKey [128]byte
		var randomValue [600]byte

		_, err := rand.Read(randomKey[:])
		assert.NoError(b, err)
		_, err = rand.Read(randomValue[:])
		assert.NoError(b, err)

		err = db.Put(randomKey[:], randomValue[:])
		assert.NoError(b, err)

		value, err := db.Get(randomKey[:])
		assert.NoError(b, err)

		assert.EqualValues(b, randomValue[:], value)
	}
}

func TestBadger_Existence(t *testing.T) {
	path := "badger"
	_ = os.RemoveAll(path)

	db, err := NewBadger(path)
	assert.NoError(t, err)
	defer func() {
		_ = db.Close()
		_ = os.RemoveAll(path)
	}()

	_, err = db.Get([]byte("not_exist"))
	assert.Error(t, err)

	err = db.Put([]byte("exist"), []byte{})
	assert.NoError(t, err)

	val, err := db.Get([]byte("exist"))
	assert.NoError(t, err)
	assert.Equal(t, []byte{}, val)
}

func TestBadger(t *testing.T) {
	path := "badger"
	_ = os.RemoveAll(path)

	db, err := NewBadger(path)
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(path)
	}()

	err = db.Put([]byte("exist"), []byte("value"))
	assert.NoError(t, err)

	wb := db.NewWriteBatch()
	assert.NoError(t, wb.Put([]byte("key_batch1"), []byte("val_batch1")))
	assert.NoError(t, wb.Put([]byte("key_batch2"), []byte("val_batch2")))
	assert.NoError(t, wb.Put([]byte("key_batch3"), []byte("val_batch2")))
	assert.NoError(t, db.CommitWriteBatch(wb))

	assert.NoError(t, db.Close())

	db2, err := NewBadger(path)
	assert.NoError(t, err)

	v, err := db2.Get([]byte("exist"))
	assert.NoError(t, err)
	assert.Equal(t, []byte("value"), v)

	// Check multiget
	mv, err := db2.MultiGet([]byte("key_batch1"), []byte("key_batch2"))
	assert.NoError(t, err)
	assert.Equal(t, [][]byte{[]byte("val_batch1"), []byte("val_batch2")}, mv)

	// Check delete
	assert.NoError(t, db2.Delete([]byte("exist")))

	_, err = db2.Get([]byte("exist"))
	assert.Error(t, err)
}

func TestBadger_WriteBatch(t *testing.T) {
	path := "badger"
	_ = os.RemoveAll(path)

	db, err := NewBadger(path)
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(path)
	}()

	wb := db.NewWriteBatch()
	for i := 0; i < 100000; i++ {
		assert.NoError(t, wb.Put([]byte(fmt.Sprintf("key_batch%d", i+1)), []byte(fmt.Sprintf("val_batch%d", i+1))))
	}

	assert.NoError(t, db.Close())

	db2, err := NewBadger(path)
	assert.NoError(t, err)

	_, err = db2.Get([]byte("key_batch100000"))
	assert.EqualError(t, err, "Key not found")

	wb = db2.NewWriteBatch()
	for i := 0; i < 100000; i++ {
		assert.NoError(t, wb.Put([]byte(fmt.Sprintf("key_batch%d", i+1)), []byte(fmt.Sprintf("val_batch%d", i+1))))
	}

	assert.NoError(t, db2.CommitWriteBatch(wb))
	assert.NoError(t, db2.Close())

	db3, err := NewBadger(path)
	assert.NoError(t, err)

	v, err := db3.Get([]byte("key_batch100000"))
	assert.NoError(t, err)
	assert.EqualValues(t, []byte("val_batch100000"), v)
}
