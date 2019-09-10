package store

import (
	"os"
	"testing"
)

// NewTestKV returns a KV store for testing purposes.
func NewTestKV(t testing.TB, kv string, path string) (KV, func()) {
	t.Helper()

	switch kv {
	case "inmem":
		inmemdb := NewInmem()
		return inmemdb, func() {
			_ = inmemdb.Close()
		}

	case "level":
		// Remove existing db
		_ = os.RemoveAll(path)

		leveldb, err := NewLevelDB(path)
		if err != nil {
			t.Fatalf("failed to create LevelDB: %s", err)
		}

		return leveldb, func() {
			_ = leveldb.Close()
			_ = os.RemoveAll(path)
		}

	case "badger":
		// Remove existing db
		_ = os.RemoveAll(path)

		badger, err := NewBadger(path)
		if err != nil {
			t.Fatalf("failed to create Badger: %s", err)
		}

		return badger, func() {
			_ = badger.Close()
			_ = os.RemoveAll(path)
		}

	case "bbolt":
		// Remove existing db
		_ = os.RemoveAll(path)

		bbolt, err := NewBbolt(path)
		if err != nil {
			t.Fatalf("failed to create Bbolt: %s", err)
		}

		return bbolt, func() {
			_ = bbolt.Close()
			_ = os.RemoveAll(path)
		}

	default:
		panic("unknown kv " + kv)
	}
}
