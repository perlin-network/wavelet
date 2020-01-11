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

package main

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
)

func runTreeBenchmark() {
	dbs := []string{"badger", "level"}

	sizes := []int{
		1, 3, 12, 46, 168, 607, 2188, 7886, 28418, 102399, // 1KB to 100MB
		128913, 162293, 204314, 257217, 323817, 407661, 513215, 646100, 813392, 1023999, // 100MB to 1GB
	}

	// Generate a CSV
	fmt.Println("hd_type,size,db,time")

	// Adjust this accordingly
	dirs := map[string]string{
		"hdd": "db",
		"ssd": "/tmp/db",
	}

	for _, hdt := range []string{"ssd", "hdd"} {
		for _, size := range sizes {
			for _, db := range dbs {
				dir := dirs[hdt]
				result := testing.Benchmark(benchmarkTreeCommit(size, db, dir))
				fmt.Printf("%s,%d,%s,%d\n",
					hdt, size, db, result.NsPerOp())
			}
		}
	}
}

func benchmarkTreeCommit(size int, db string, dir string) func(b *testing.B) {
	code := make([]byte, 1024*size)
	if _, err := rand.Read(code); err != nil { // nolint:gosec
		panic(err)
	}

	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil { // nolint:gosec
		panic(err)
	}

	return func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			func() {
				kv, cleanup, err := store.NewTestKV(db, dir)
				if err != nil {
					b.Fatal(err)
				}

				defer cleanup()

				tree := avl.New(kv)
				wavelet.WriteAccountContractCode(tree, key, code)

				if err := tree.Commit(); err != nil {
					b.Fatal(err)
				}
			}()
		}
	}
}
