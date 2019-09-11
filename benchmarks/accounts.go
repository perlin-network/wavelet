package main

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/store"
)

func main() {
	dbs := []string{"badger", "bbolt", "level"}

	sizes := []int{
		1, 3, 7, 15, 31, 63, 125, 251, 501, 1000, // 1MB to 1GB
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
			Balance:   rand.Uint64(),
			Stake:     rand.Uint64(),
			Reward:    rand.Uint64(),
		}
	}

	return func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			func() {
				kv, cleanup := store.NewTestKV(b, db, dir)
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
