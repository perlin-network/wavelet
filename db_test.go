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
	for i := 10; i > 0; i-- {
		rand.Read(a[:])

		rw := RewardWithdrawal{
			accountID: a,
			round:     uint64(i),
			amount:    rand.Uint64(),
		}

		StoreRewardWithdrawal(tree, rw)

		defer func() {
			tree.Delete(rw.Key())
		}()
	}

	rws := GetRewardWithdrawals(tree, 7)

	assert.Equal(t, 7, len(rws))
	assert.True(t, sort.SliceIsSorted(rws, func(i, j int) bool { return rws[i].round < rws[j].round }))
}
