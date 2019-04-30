package store

import (
	"io"
)

type KV interface {
	io.Closer

	Get(key []byte) ([]byte, error)
	MultiGet(keys ...[]byte) ([][]byte, error)

	Put(key, value []byte) error

	NewWriteBatch() WriteBatch
	CommitWriteBatch(batch WriteBatch) error

	Delete(key []byte) error
}

type WriteBatch interface {
	Put(key, value []byte)

	Clear()
	Count() int
	Destroy()
}
