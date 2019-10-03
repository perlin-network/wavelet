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

package wavelet

import (
	"encoding/binary"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"sort"
	"testing"
)

func TestRewardWithdrawals(t *testing.T) {
	tree := avl.New(store.NewInmem())

	var a AccountID
	rws := make([]RewardWithdrawalRequest, 20)
	for i := range rws {
		rand.Read(a[:])

		rw := RewardWithdrawalRequest{
			account: a,
			round:   uint64(i + 1),
			amount:  rand.Uint64(),
		}

		rws[i] = rw
	}

	rand.Shuffle(len(rws), func(i, j int) { rws[i], rws[j] = rws[j], rws[i] })

	for _, rw := range rws {
		StoreRewardWithdrawalRequest(tree, rw)
	}

	rws = GetRewardWithdrawalRequests(tree, 7)

	assert.Equal(t, 7, len(rws))
	assert.True(t, sort.SliceIsSorted(rws, func(i, j int) bool { return rws[i].round < rws[j].round }))
}

func TestReadUnderAccounts(t *testing.T) {
	stateStore := store.NewInmem()
	state := avl.New(stateStore)

	var id AccountID
	_, err := rand.Read(id[:])
	assert.NoError(t, err)
	WriteAccountBalance(state, id, 1000)

	var expectedBal [8]byte
	binary.LittleEndian.PutUint64(expectedBal[:], uint64(1000))

	bal, exist := readUnderAccounts(state, id, keyAccountBalance[:])
	assert.True(t, exist)
	assert.Equal(t, expectedBal[:], bal)
}

func TestWriteUnderAccounts(t *testing.T) {
	stateStore := store.NewInmem()
	state := avl.New(stateStore)

	var id AccountID
	_, err := rand.Read(id[:])
	assert.NoError(t, err)

	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(1000))

	writeUnderAccounts(state, id, keyAccountBalance[:], buf[:])

	bal, exist := ReadAccountBalance(state, id)
	assert.True(t, exist)
	assert.Equal(t, uint64(1000), bal)
}

func TestReadAccountContractPage(t *testing.T) {
	stateStore := store.NewInmem()
	state := avl.New(stateStore)

	var id TransactionID
	_, err := rand.Read(id[:])
	assert.NoError(t, err)

	var page [512]byte
	_, err = rand.Read(page[:])
	assert.NoError(t, err)

	WriteAccountContractPage(state, id, 0, page[:])

	actualPage, exist := ReadAccountContractPage(state, id, 0)
	assert.True(t, exist)
	assert.Equal(t, page[:], actualPage)
}

func TestWriteAccountContractPage(t *testing.T) {
	stateStore := store.NewInmem()
	state := avl.New(stateStore)

	var id TransactionID
	_, err := rand.Read(id[:])
	assert.NoError(t, err)

	var page [512]byte
	_, err = rand.Read(page[:])
	assert.NoError(t, err)

	WriteAccountContractPage(state, id, 0, page[:])

	_, exist := ReadAccountContractPage(state, id, 0)
	assert.True(t, exist)
}

func BenchmarkReadUnderAccounts(b *testing.B) {
	stateStore := store.NewInmem()
	state := avl.New(stateStore)

	var id AccountID
	_, err := rand.Read(id[:])
	assert.NoError(b, err)

	WriteAccountBalance(state, id, 1000)

	var expectedBal [8]byte
	binary.LittleEndian.PutUint64(expectedBal[:], uint64(1000))

	b.ReportAllocs()
	b.ResetTimer()
	var bal []byte
	var exist bool
	for n := 0; n < b.N; n++ {
		bal, exist = readUnderAccounts(state, id, keyAccountBalance[:])
	}

	assert.True(b, exist)
	assert.Equal(b, expectedBal[:], bal)
}

func BenchmarkWriteUnderAccounts(b *testing.B) {
	stateStore := store.NewInmem()
	state := avl.New(stateStore)

	var id AccountID
	_, err := rand.Read(id[:])
	assert.NoError(b, err)

	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(1000))

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		writeUnderAccounts(state, id, keyAccountBalance[:], buf[:])
	}

	bal, exist := ReadAccountBalance(state, id)
	assert.True(b, exist)
	assert.Equal(b, uint64(1000), bal)
}

func BenchmarkReadAccountContractPage(b *testing.B) {
	stateStore := store.NewInmem()
	state := avl.New(stateStore)

	var id TransactionID
	_, err := rand.Read(id[:])
	assert.NoError(b, err)

	var expectedPage [512]byte
	_, err = rand.Read(expectedPage[:])
	assert.NoError(b, err)

	WriteAccountContractPage(state, id, 0, expectedPage[:])

	var page []byte
	var exist bool

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		page, exist = ReadAccountContractPage(state, id, 0)
	}

	assert.True(b, exist)
	assert.Equal(b, expectedPage[:], page)
}

func BenchmarkWriteAccountContractPage(b *testing.B) {
	stateStore := store.NewInmem()
	state := avl.New(stateStore)

	var id TransactionID
	_, err := rand.Read(id[:])
	assert.NoError(b, err)

	var page [512]byte
	_, err = rand.Read(page[:])
	assert.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		WriteAccountContractPage(state, id, 0, page[:])
	}

	_, exist := ReadAccountContractPage(state, id, 0)
	assert.True(b, exist)
}
