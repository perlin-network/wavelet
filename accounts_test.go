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

// +build unit

package wavelet

import (
	"bytes"
	"testing"
	"testing/quick"

	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
)

func TestSmartContract(t *testing.T) {
	fn := func(id TransactionID, code [2 * 1024]byte) bool {
		accounts := NewAccounts(store.NewInmem())
		tree := accounts.Snapshot()

		returned, available := ReadAccountContractCode(tree, id)
		if returned != nil || available == true {
			return false
		}

		WriteAccountContractCode(tree, id, code[:])

		returned, available = ReadAccountContractCode(tree, id)
		if !bytes.Equal(code[:], returned) || available == false {
			return false
		}

		return true
	}

	assert.NoError(t, quick.Check(fn, nil))
}

//func BenchmarkAccountsCommit(b *testing.B) {
//	dbs := []string{"level"}
//
//	sizes := []int{}
//	for i := 0; i < 10; i++ {
//		sizes = append(sizes, (i+1)*50)
//	}
//
//	for _, db := range dbs {
//		for _, size := range sizes {
//			b.Run(fmt.Sprintf("%s/%dMB", db, size), func(b *testing.B) {
//				benchmarkAccountsCommit(b, size, db)
//			})
//		}
//	}
//}
//
//func benchmarkAccountsCommit(b *testing.B, size int, db string) {
//	rand.Seed(time.Now().UnixNano())
//
//	var code [1024 * 1024]byte
//	if _, err := rand.Read(code[:]); err != nil {
//		b.Fatal(err)
//	}
//
//	// Generate accounts
//	gen := make([]account, size)
//	for i := 0; i < len(gen); i++ {
//		// Use random keys to speed up generation
//		var key [32]byte
//		if _, err := rand.Read(key[:]); err != nil {
//			b.Fatal(err)
//		}
//
//		gen[i] = account{
//			PublicKey: key,
//			Balance:   rand.Uint64(),
//			Stake:     rand.Uint64(),
//			Reward:    rand.Uint64(),
//		}
//	}
//
//	for n := 0; n < b.N; n++ {
//		func() {
//			kv, cleanup := store.NewTestKV(b, db, "db_"+randString(10))
//			defer cleanup()
//
//			accounts := NewAccounts(kv)
//			snapshot := accounts.Snapshot()
//
//			for _, acc := range gen {
//				WriteAccountBalance(snapshot, acc.PublicKey, acc.Balance)
//				WriteAccountStake(snapshot, acc.PublicKey, acc.Stake)
//				WriteAccountReward(snapshot, acc.PublicKey, acc.Reward)
//				WriteAccountContractCode(snapshot, acc.PublicKey, code[:])
//			}
//
//			if err := accounts.Commit(snapshot); err != nil {
//				b.Fatal(err)
//			}
//		}()
//	}
//}
//
//var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
//
//func randString(n int) string {
//	b := make([]rune, n)
//	for i := range b {
//		b[i] = letterRunes[rand.Intn(len(letterRunes))]
//	}
//	return string(b)
//}
