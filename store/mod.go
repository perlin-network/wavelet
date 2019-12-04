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
	"errors"
	"io"
)

var (
	ErrNotFound = errors.New("not found")
)

type KV interface {
	io.Closer

	Get(key []byte) ([]byte, error)
	MultiGet(keys ...[]byte) ([][]byte, error)

	Put(key, value []byte) error

	NewWriteBatch() WriteBatch
	CommitWriteBatch(batch WriteBatch) error

	Delete(key []byte) error

	Dir() string
}

// WriteBatch batches a collection of put operations in memory before
// it's committed to disk.
//
// It's not guaranteed that all of the operations are kept in memory before
// the write batch is explicitly committed. It might be possible that the
// database decided commit the batch to disk earlier. For example, if a write
// batch is created, and 1000 put operations are batched, it might happen
// that while batching the 600th operation, the database decides to commit
// the first 599th operations first before proceeding.
type WriteBatch interface {
	Put(key, value []byte) error
	Delete(key []byte) error

	Clear()
	Count() int
	Destroy()
}
