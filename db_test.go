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
	rws := make([]RewardWithdrawal, 20)
	for i := range rws {
		rand.Read(a[:])

		rw := RewardWithdrawal{
			accountID: a,
			round:     uint64(i + 1),
			amount:    rand.Uint64(),
		}

		rws[i] = rw
	}

	rand.Shuffle(len(rws), func(i, j int) { rws[i], rws[j] = rws[j], rws[i] })

	for _, rw := range rws {
		StoreRewardWithdrawal(tree, rw)
	}

	rws = GetRewardWithdrawals(tree, 7)

	assert.Equal(t, 7, len(rws))
	assert.True(t, sort.SliceIsSorted(rws, func(i, j int) bool { return rws[i].round < rws[j].round }))
}
