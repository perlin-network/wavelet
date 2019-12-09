// The MIT License (MIT)
//
// Copyright (c) 2017 Joshua J Baker
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

package btree

import (
	"bytes"
	"fmt"
	"math/rand"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func randKeys(n int) (keys [][]byte) {
	format := fmt.Sprintf("%%0%dd", len(fmt.Sprintf("%d", n-1)))

	for _, i := range rand.Perm(n) {
		keys = append(keys, []byte(fmt.Sprintf(format, i)))
	}

	return
}

func bytesEquals(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}

	for i := 0; i < len(a); i++ {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}

	return true
}

func TestDescend(t *testing.T) {
	var tr BTree

	var count int

	tr.Descend([]byte("1"), func(key []byte, value interface{}) bool {
		count++
		return true
	})

	if count > 0 {
		t.Fatalf("expected 0, got %v", count)
	}

	var keys [][]byte

	for i := 0; i < 1000; i += 10 {
		keys = append(keys, []byte(fmt.Sprintf("%03d", i)))
		tr.Set(keys[len(keys)-1], nil)
	}

	var exp [][]byte

	tr.Reverse(func(key []byte, _ interface{}) bool {
		exp = append(exp, key)
		return true
	})

	for i := 999; i >= 0; i-- {
		i := i

		key := []byte(fmt.Sprintf("%03d", i))

		var all [][]byte

		tr.Descend(key, func(key []byte, value interface{}) bool {
			all = append(all, key)
			return true
		})

		for len(exp) > 0 && bytes.Compare(key, exp[0]) < 0 {
			exp = exp[1:]
		}

		var count int

		tr.Descend(key, func(key []byte, value interface{}) bool {
			if count == (i+1)%maxItems {
				return false
			}
			count++
			return true
		})

		if count > len(exp) {
			t.Fatalf("expected 1, got %v", count)
		}

		if !bytesEquals(exp, all) {
			fmt.Printf("exp: %v\n", exp)
			fmt.Printf("all: %v\n", all)
			t.Fatal("mismatch")
		}
	}
}

func TestAscend(t *testing.T) {
	var tr BTree

	var count int

	tr.Ascend([]byte("1"), func(key []byte, value interface{}) bool {
		count++
		return true
	})

	if count > 0 {
		t.Fatalf("expected 0, got %v", count)
	}

	var keys [][]byte

	for i := 0; i < 1000; i += 10 {
		keys = append(keys, []byte(fmt.Sprintf("%03d", i)))
		tr.Set(keys[len(keys)-1], nil)
	}

	exp := keys

	for i := -1; i < 1000; i++ {
		i := i

		var key []byte

		if i == -1 {
			key = nil
		} else {
			key = []byte(fmt.Sprintf("%03d", i))
		}

		var all [][]byte

		tr.Ascend(key, func(key []byte, value interface{}) bool {
			all = append(all, key)
			return true
		})

		for len(exp) > 0 && bytes.Compare(key, exp[0]) > 0 {
			exp = exp[1:]
		}

		var count int

		tr.Ascend(key, func(key []byte, value interface{}) bool {
			if count == (i+1)%maxItems {
				return false
			}
			count++
			return true
		})

		if count > len(exp) {
			t.Fatalf("expected 1, got %v", count)
		}

		if !bytesEquals(exp, all) {
			t.Fatal("mismatch")
		}
	}
}

// nolint: gocyclo, gocognit
func TestBTree(t *testing.T) {
	N := 10000

	var tr BTree

	keys := randKeys(N)

	// insert all items
	for _, key := range keys {
		value, replaced := tr.Set(key, key)

		if replaced {
			t.Fatal("expected false")
		}

		if value != nil {
			t.Fatal("expected nil")
		}
	}

	// check length
	if tr.Len() != len(keys) {
		t.Fatalf("expected %v, got %v", len(keys), tr.Len())
	}

	// get each value
	for _, key := range keys {
		value, gotten := tr.Get(key)

		if !gotten {
			t.Fatal("expected true")
		}

		if value == nil || !bytes.Equal(value.([]byte), key) {
			t.Fatalf("expected '%v', got '%v'", key, value)
		}
	}

	// scan all items
	var last []byte

	all := make(map[string]interface{})

	tr.Scan(func(key []byte, value interface{}) bool {
		if bytes.Compare(key, last) <= 0 {
			t.Fatal("out of order")
		}

		if !bytes.Equal(value.([]byte), key) {
			t.Fatalf("mismatch")
		}

		last = key
		all[string(key)] = value

		return true
	})

	if len(all) != len(keys) {
		t.Fatalf("expected '%v', got '%v'", len(keys), len(all))
	}

	// reverse all items
	var prev []byte

	all = make(map[string]interface{})

	tr.Reverse(func(key []byte, value interface{}) bool {
		if prev != nil && bytes.Compare(key, prev) >= 0 {
			t.Fatal("out of order")
		}

		if !bytes.Equal(value.([]byte), key) {
			t.Fatalf("mismatch")
		}

		prev = key
		all[string(key)] = value

		return true
	})

	if len(all) != len(keys) {
		t.Fatalf("expected '%v', got '%v'", len(keys), len(all))
	}

	// try to get an invalid item
	value, gotten := tr.Get([]byte("invalid"))

	if gotten {
		t.Fatal("expected false")
	}

	if value != nil {
		t.Fatal("expected nil")
	}

	// scan and quit at various steps
	for i := 0; i < 100; i++ {
		i := i

		var j int

		tr.Scan(func(key []byte, value interface{}) bool {
			if j == i {
				return false
			}

			j++

			return true
		})
	}

	// reverse and quit at various steps
	for i := 0; i < 100; i++ {
		i := i

		var j int

		tr.Reverse(func(key []byte, value interface{}) bool {
			if j == i {
				return false
			}
			j++
			return true
		})
	}

	// delete half the items
	for _, key := range keys[:len(keys)/2] {
		value, deleted := tr.Delete(key)

		if !deleted {
			t.Fatal("expected true")
		}

		if value == nil || !bytes.Equal(value.([]byte), key) {
			t.Fatalf("expected '%v', got '%v'", key, value)
		}
	}

	// check length
	if tr.Len() != len(keys)/2 {
		t.Fatalf("expected %v, got %v", len(keys)/2, tr.Len())
	}

	// try delete half again
	for _, key := range keys[:len(keys)/2] {
		value, deleted := tr.Delete(key)

		if deleted {
			t.Fatal("expected false")
		}

		if value != nil {
			t.Fatalf("expected nil")
		}
	}

	// try delete half again
	for _, key := range keys[:len(keys)/2] {
		value, deleted := tr.Delete(key)

		if deleted {
			t.Fatal("expected false")
		}

		if value != nil {
			t.Fatalf("expected nil")
		}
	}

	// check length
	if tr.Len() != len(keys)/2 {
		t.Fatalf("expected %v, got %v", len(keys)/2, tr.Len())
	}

	// scan items
	last = nil
	all = make(map[string]interface{})

	tr.Scan(func(key []byte, value interface{}) bool {
		if bytes.Compare(key, last) <= 0 {
			t.Fatal("out of order")
		}

		if !bytes.Equal(value.([]byte), key) {
			t.Fatalf("mismatch")
		}

		last = key
		all[string(key)] = value

		return true
	})

	if len(all) != len(keys)/2 {
		t.Fatalf("expected '%v', got '%v'", len(keys), len(all))
	}

	// replace second half
	for _, key := range keys[len(keys)/2:] {
		value, replaced := tr.Set(key, key)

		if !replaced {
			t.Fatal("expected true")
		}

		if value == nil || !bytes.Equal(value.([]byte), key) {
			t.Fatalf("expected '%v', got '%v'", key, value)
		}
	}

	// delete next half the items
	for _, key := range keys[len(keys)/2:] {
		value, deleted := tr.Delete(key)

		if !deleted {
			t.Fatal("expected true")
		}

		if value == nil || !bytes.Equal(value.([]byte), key) {
			t.Fatalf("expected '%v', got '%v'", key, value)
		}
	}

	// check length
	if tr.Len() != 0 {
		t.Fatalf("expected %v, got %v", 0, tr.Len())
	}

	// do some stuff on an empty tree
	value, gotten = tr.Get(keys[0])

	if gotten {
		t.Fatal("expected false")
	}

	if value != nil {
		t.Fatal("expected nil")
	}

	tr.Scan(func(key []byte, value interface{}) bool {
		t.Fatal("should not be reached")
		return true
	})

	tr.Reverse(func(key []byte, value interface{}) bool {
		t.Fatal("should not be reached")
		return true
	})

	var deleted bool

	value, deleted = tr.Delete([]byte("invalid"))

	if deleted {
		t.Fatal("expected false")
	}

	if value != nil {
		t.Fatal("expected nil")
	}
}

func BenchmarkTidwallSequentialSet(b *testing.B) {
	var tr BTree

	keys := randKeys(b.N)

	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i], keys[j]) < 0
	})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tr.Set(keys[i], nil)
	}
}

func BenchmarkTidwallSequentialGet(b *testing.B) {
	var tr BTree

	keys := randKeys(b.N)

	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i], keys[j]) < 0
	})

	for i := 0; i < b.N; i++ {
		tr.Set(keys[i], nil)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tr.Get(keys[i])
	}
}

func BenchmarkTidwallRandomSet(b *testing.B) {
	var tr BTree

	keys := randKeys(b.N)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tr.Set(keys[i], nil)
	}
}

func BenchmarkTidwallRandomGet(b *testing.B) {
	var tr BTree

	keys := randKeys(b.N)

	for i := 0; i < b.N; i++ {
		tr.Set(keys[i], nil)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tr.Get(keys[i])
	}
}

func TestBTreeOne(t *testing.T) {
	var tr BTree

	tr.Set([]byte("1"), "1")
	tr.Delete([]byte("1"))
	tr.Set([]byte("1"), "1")
	tr.Delete([]byte("1"))
	tr.Set([]byte("1"), "1")
	tr.Delete([]byte("1"))
}

func TestBTree256(t *testing.T) {
	var tr BTree

	var n int

	for j := 0; j < 2; j++ {
		for _, i := range rand.Perm(256) {
			tr.Set([]byte(fmt.Sprintf("%d", i)), i)

			n++

			if tr.Len() != n {
				t.Fatalf("expected 256, got %d", n)
			}
		}

		for _, i := range rand.Perm(256) {
			v, ok := tr.Get([]byte(fmt.Sprintf("%d", i)))

			if !ok {
				t.Fatal("expected true")
			}

			if v.(int) != i {
				t.Fatalf("expected %d, got %d", i, v.(int))
			}
		}

		for _, i := range rand.Perm(256) {
			tr.Delete([]byte(fmt.Sprintf("%d", i)))

			n--

			if tr.Len() != n {
				t.Fatalf("expected 256, got %d", n)
			}
		}

		for _, i := range rand.Perm(256) {
			_, ok := tr.Get([]byte(fmt.Sprintf("%d", i)))

			if ok {
				t.Fatal("expected false")
			}
		}
	}
}

func TestBTreeRandom(t *testing.T) {
	var count uint32

	T := runtime.NumCPU()
	D := time.Second
	N := 1000

	bkeys := make([][]byte, N)
	for i, key := range rand.Perm(N) {
		bkeys[i] = []byte(strconv.Itoa(key))
	}

	var wg sync.WaitGroup

	wg.Add(T)

	for i := 0; i < T; i++ {
		go func() {
			defer wg.Done()

			start := time.Now()

			for {
				r := rand.New(rand.NewSource(time.Now().UnixNano()))

				keys := make([][]byte, len(bkeys))
				copy(keys, bkeys)

				testBTreeRandom(t, r, keys, &count)
				if time.Since(start) > D {
					break
				}
			}
		}()
	}

	wg.Wait()
}

func shuffle(r *rand.Rand, keys [][]byte) {
	for i := range keys {
		j := r.Intn(i + 1)
		keys[i], keys[j] = keys[j], keys[i]
	}
}

func testBTreeRandom(t *testing.T, r *rand.Rand, keys [][]byte, count *uint32) {
	var tr BTree

	keys = keys[:rand.Intn(len(keys))]

	shuffle(r, keys)

	for i := 0; i < len(keys); i++ {
		prev, ok := tr.Set(keys[i], keys[i])

		if ok || prev != nil {
			t.Fatalf("expected nil")
		}
	}

	shuffle(r, keys)

	for i := 0; i < len(keys); i++ {
		prev, ok := tr.Get(keys[i])

		if !ok || !bytes.Equal(prev.([]byte), keys[i]) {
			t.Fatalf("expected '%v', got '%v'", keys[i], prev)
		}
	}

	shuffle(r, keys)

	for i := 0; i < len(keys); i++ {
		prev, ok := tr.Delete(keys[i])

		if !ok || !bytes.Equal(prev.([]byte), keys[i]) {
			t.Fatalf("expected '%v', got '%v'", keys[i], prev)
		}

		prev, ok = tr.Get(keys[i])

		if ok || prev != nil {
			t.Fatalf("expected nil")
		}
	}

	atomic.AddUint32(count, 1)
}
