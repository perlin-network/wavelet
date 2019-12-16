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
