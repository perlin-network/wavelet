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
	mrand "math/rand"
	"testing"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/store"
)

func runAccountsBenchmark() {
	dbs := []string{"badger", "level"}

	sizes := []int{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
		20, 30, 40, 50, 60, 70, 80, 90, 100,
		200, 300, 400, 500, 600, 700, 800, 900, 1000,
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
				result := testing.Benchmark(benchmarkAccountsCommit(size, db, dir))
				fmt.Printf("%s,%d,%s,%d\n",
					hdt, size, db, result.NsPerOp())
			}
		}
	}
}

type account struct {
	PublicKey [32]byte
	Balance   uint64
	Stake     uint64
	Reward    uint64
}

func benchmarkAccountsCommit(size int, db string, dir string) func(b *testing.B) {
	var code [1024 * 1024]byte
	if _, err := rand.Read(code[:]); err != nil {
		panic(err)
	}

	gen := make([]account, size)
	for i := 0; i < len(gen); i++ {
		var key [32]byte
		if _, err := rand.Read(key[:]); err != nil {
			panic(err)
		}

		gen[i] = account{
			PublicKey: key,
			Balance:   mrand.Uint64(),
			Stake:     mrand.Uint64(),
			Reward:    mrand.Uint64(),
		}
	}

	return func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			func() {
				kv, cleanup, err := store.NewTestKV(db, dir)
				if err != nil {
					b.Fatal(err)
				}

				defer cleanup()

				accounts := wavelet.NewAccounts(kv)
				snapshot := accounts.Snapshot()

				for _, acc := range gen {
					wavelet.WriteAccountBalance(snapshot, acc.PublicKey, acc.Balance)
					wavelet.WriteAccountStake(snapshot, acc.PublicKey, acc.Stake)
					wavelet.WriteAccountReward(snapshot, acc.PublicKey, acc.Reward)
					wavelet.WriteAccountContractCode(snapshot, acc.PublicKey, code[:])
				}

				if err := accounts.Commit(snapshot); err != nil {
					b.Fatal(err)
				}
			}()
		}
	}
}
