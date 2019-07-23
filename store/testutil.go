package store

import (
	"os"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
)

// NewTestKV returns a KV store for testing purposes.
func NewTestKV(t testing.TB, kv string, path string) (KV, func()) {
	t.Helper()

	if kv == "inmem" {
		inmemdb := NewInmem()
		return inmemdb, func() {
			if err := inmemdb.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if kv == "level" {
		// Remove existing db
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}

		db, err := NewLevelDB(path)
		if err != nil {
			t.Fatalf("failed to create LevelDB: %s", err)
		}

		return db, func() {
			if err := db.Close(); err != nil && err != leveldb.ErrClosed {
				t.Fatal(err)
			}
			if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
		}
	}

	t.Fatalf("unknown kv %s", kv)
	panic("unknown kv " + kv)
}
