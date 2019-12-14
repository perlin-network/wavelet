package store

import (
	"fmt"
	"os"
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
func NewTestKV(kv string, path string, opts ...TestKVOption) (KV, func(), error) {
	cfg := defaultKVConfig()

	for _, opt := range opts {
		opt(&cfg)
	}

	switch kv {
	case "inmem":
		inmemdb := NewInmem()

		return inmemdb, func() { _ = inmemdb.Close() }, nil
	case "level": //nolint:goconst
		if cfg.RemoveExisting {
			_ = os.RemoveAll(path)
		}

		leveldb, err := NewLevelDB(path)
		if err != nil {
			return nil, nil, err
		}

		cleanup := func() {
			_ = leveldb.Close()

			if cfg.RemoveExisting {
				_ = os.RemoveAll(path)
			}
		}

		return leveldb, cleanup, nil
	case "badger": // nolint:goconst
		if cfg.RemoveExisting {
			_ = os.RemoveAll(path)
		}

		badger, err := NewBadger(path)
		if err != nil {
			return nil, nil, err
		}

		cleanup := func() {
			_ = badger.Close()

			if cfg.RemoveExisting {
				_ = os.RemoveAll(path)
			}
		}

		return badger, cleanup, nil
	default:
		return nil, nil, fmt.Errorf("unknown kv %s", kv)
	}
}
