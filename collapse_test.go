package wavelet

import (
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
)

func TestProcessRewardWithdrawals(t *testing.T) {
	state := avl.New(store.NewInmem())

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ctx := NewCollapseContext(state)

	// First reward
	ctx.StoreRewardWithdrawalRequest(RewardWithdrawalRequest{
		account: keys.PublicKey(),
		amount:  1,
		round:   1,
	})

	// Second reward
	ctx.StoreRewardWithdrawalRequest(RewardWithdrawalRequest{
		account: keys.PublicKey(),
		amount:  2,
		round:   2,
	})

	// No reward is withdrew
	ctx.processRewardWithdrawals(50)
	assert.Len(t, ctx.rewardWithdrawalRequests, 2)
	bal, _ := ctx.ReadAccountBalance(keys.PublicKey())
	assert.Equal(t, uint64(0), bal)

	// Withdraw only first reward
	ctx.processRewardWithdrawals(51)
	assert.Len(t, ctx.rewardWithdrawalRequests, 1)
	bal, _ = ctx.ReadAccountBalance(keys.PublicKey())
	assert.Equal(t, uint64(1), bal)

	// Withdraw the second reward
	ctx.processRewardWithdrawals(52)
	assert.Len(t, ctx.rewardWithdrawalRequests, 0)
	bal, _ = ctx.ReadAccountBalance(keys.PublicKey())
	assert.Equal(t, uint64(3), bal)

	assert.NoError(t, ctx.Flush())
	bal, _ = ReadAccountBalance(state, keys.PublicKey())
	assert.Equal(t, uint64(3), bal)
}

func TestCollapseContext(t *testing.T) {
	state := avl.New(store.NewInmem())

	ctx := NewCollapseContext(state)

	// The expected account IDs and its order
	var expectedAccountIDs []AccountID

	checkAccountID := func(override bool, write func(id AccountID)) {
		var id AccountID
		if override {
			// Choose a random account from the slice
			if len(ctx.accountIDs) == 0 {
				assert.FailNow(t, "could not choose a random account because accountIDs slice is empty")
			}
			id = ctx.accountIDs[rand.Intn(len(ctx.accountIDs))]
		} else {
			// Generate a random AccountID that does not exist
			for {
				_, err := rand.Read(id[:])
				assert.NoError(t, err)
				_, exist := ctx.accounts[id]
				if !exist {
					break
				}
			}

			expectedAccountIDs = append(expectedAccountIDs, id)
		}

		write(id)

		_, exist := ctx.accounts[id]
		assert.True(t, exist)
		// Check the account IDs and its ordering
		assert.EqualValues(t, expectedAccountIDs, ctx.accountIDs)
	}

	// For each value, we do a write and check the value.
	f := func(override bool) {

		checkAccountID(override, func(id AccountID) {
			ctx.WriteAccountNonce(id, 1)
			nonce, _ := ctx.ReadAccountNonce(id)
			assert.Equal(t, uint64(1), nonce)
		})

		checkAccountID(override, func(id AccountID) {
			ctx.WriteAccountBalance(id, 2)
			bal, _ := ctx.ReadAccountBalance(id)
			assert.Equal(t, uint64(2), bal)
		})

		checkAccountID(override, func(id AccountID) {
			ctx.WriteAccountStake(id, 3)
			stake, _ := ctx.ReadAccountStake(id)
			assert.Equal(t, uint64(3), stake)
		})

		checkAccountID(override, func(id AccountID) {
			ctx.WriteAccountReward(id, 4)
			reward, _ := ctx.ReadAccountReward(id)
			assert.Equal(t, uint64(4), reward)
		})

		checkAccountID(override, func(id AccountID) {
			ctx.WriteAccountContractGasBalance(id, 5)
			gasBal, _ := ctx.ReadAccountContractGasBalance(id)
			assert.Equal(t, uint64(5), gasBal)
		})

		checkAccountID(override, func(id AccountID) {
			var _code [64]byte
			_, err := rand.Read(_code[:])
			assert.NoError(t, err)

			ctx.WriteAccountContractCode(id, _code[:])
			code, _ := ctx.ReadAccountContractCode(id)
			assert.EqualValues(t, _code[:], code[:])
		})

		checkAccountID(override, func(id AccountID) {
			var mem [64]byte
			_, err := rand.Read(mem[:])
			assert.NoError(t, err)

			var globals = [2]int64{1, 2}

			_vmState := &VMState{
				Globals: globals[:],
				Memory:  mem[:],
			}
			ctx.SetContractState(id, _vmState)
			vmState, _ := ctx.GetContractState(id)
			assert.EqualValues(t, _vmState, vmState)
		})
	}

	// Case 1: new account IDs
	f(false)

	// Case 2: existing account IDs
	f(true)
}
