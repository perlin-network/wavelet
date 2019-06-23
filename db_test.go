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
