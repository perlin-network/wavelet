package store

import (
	"os"
	"testing"
)

type TestKVConfig struct {
	RemoveExisting bool
}

func defaultKVConfig() TestKVConfig {
	return TestKVConfig{
		RemoveExisting: true,
	}
}

type TestKVOption func(cfg *TestKVConfig)

func WithKeepExisting() TestKVOption {
	return func(cfg *TestKVConfig) {
		cfg.RemoveExisting = false
	}
}

// NewTestKV returns a KV store for testing purposes.
func NewTestKV(t testing.TB, kv string, path string, opts ...TestKVOption) (KV, func()) {
	t.Helper()

	cfg := defaultKVConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	switch kv {
	case "inmem":
		inmemdb := NewInmem()
		return inmemdb, func() {
			_ = inmemdb.Close()
		}

	case "level":
		if cfg.RemoveExisting {
			_ = os.RemoveAll(path)
		}

		leveldb, err := NewLevelDB(path)
		if err != nil {
			t.Fatalf("failed to create LevelDB: %s", err)
		}

		return leveldb, func() {
			_ = leveldb.Close()
			if cfg.RemoveExisting {
				_ = os.RemoveAll(path)
			}
		}

	case "badger":
		if cfg.RemoveExisting {
			_ = os.RemoveAll(path)
		}

		badger, err := NewBadger(path)
		if err != nil {
			t.Fatalf("failed to create Badger: %s", err)
		}

		return badger, func() {
			_ = badger.Close()
			if cfg.RemoveExisting {
				_ = os.RemoveAll(path)
			}
		}

	default:
		panic("unknown kv " + kv)
	}
}
