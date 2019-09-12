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
