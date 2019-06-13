package debounce

import "time"

type Config struct {
	action func([][]byte)
	period time.Duration

	keys        []string
	bufferLimit int
}

type ConfigOption func(cfg *Config)

func parseOptions(opts []ConfigOption) *Config {
	cfg := &Config{
		action:      func([][]byte) {},
		period:      50 * time.Millisecond,
		bufferLimit: 16384,
	}

	for _, cs := range opts {
		cs(cfg)
	}

	return cfg
}

func WithAction(action func([][]byte)) ConfigOption {
	return func(cfg *Config) {
		cfg.action = action
	}
}

func WithPeriod(period time.Duration) ConfigOption {
	return func(cfg *Config) {
		cfg.period = period
	}
}

func WithBufferLimit(limit int) ConfigOption {
	return func(cfg *Config) {
		cfg.bufferLimit = limit
	}
}

func WithKeys(keys ...string) ConfigOption {
	return func(cfg *Config) {
		cfg.keys = keys
	}
}
