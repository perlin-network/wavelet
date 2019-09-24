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

	default:
		panic("unknown kv " + kv)
	}
}
