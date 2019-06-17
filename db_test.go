package wavelet

import (
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"sort"
	"testing"
)

func TestRewardWithdrawals(t *testing.T) {
	storage, err := store.NewLevelDB("temp")
	if !assert.NoError(t, err) {
		return
	}

	var a AccountID
	for i := 10; i > 0; i-- {
		rand.Read(a[:])

		rw := RewardWithdrawal{
			accountID: a,
			round: uint64(i),
			amount: rand.Uint64(),
		}

		if !assert.NoError(t, StoreRewardWithdrawal(storage, rw)) {
			return
		}

		defer func() {
			_ = storage.Delete(rw.Key())
		}()
	}

	rws, err := GetRewardWithdrawals(storage, 7)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, 7, len(rws))
	assert.True(t, sort.SliceIsSorted(rws, func(i, j int) bool {return rws[i].round < rws[j].round}))
}