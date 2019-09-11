package main

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
)

func main() {
	dbs := []string{"badger", "bbolt", "level"}

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
	code := make([]byte, 1024)
	if _, err := rand.Read(code); err != nil {
		panic(err)
	}

	keys := make([][32]byte, size)
	for i := 0; i < size; i++ {
		// Use random keys to speed up generation
		if _, err := rand.Read(keys[i][:]); err != nil {
			panic(err)
		}
	}

	return func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			func() {
				kv, cleanup := store.NewTestKV(b, db, dir)
				defer cleanup()

				tree := avl.New(kv)
				for _, key := range keys {
					wavelet.WriteAccountContractCode(tree, key, code)
				}

				if err := tree.Commit(); err != nil {
					b.Fatal(err)
				}
			}()
		}
	}
}
