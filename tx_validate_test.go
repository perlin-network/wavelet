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
	"crypto/rand"
	"testing"

	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
)

func TestValidateStakeTransaction(t *testing.T) {
	state := avl.New(store.NewInmem())

	keys, err := skademlia.NewKeys(1, 1)
	if !assert.NoError(t, err) {
		return
	}

	WriteAccountBalance(state, keys.PublicKey(), 42)
	WriteAccountStake(state, keys.PublicKey(), 42)
	WriteAccountReward(state, keys.PublicKey(), 42)

	t.Run("place stake", func(t *testing.T) {
		payload, err := buildPlaceStakePayload(5004).Marshal()
		if !assert.NoError(t, err) {
			return
		}

		tx := buildSignedTransaction(keys, sys.TagStake, 1, 1, payload)

		assert.Error(t, ValidateTransaction(state, tx))
	})

	t.Run("withdraw stake", func(t *testing.T) {
		payload, err := buildWithdrawStakePayload(5004).Marshal()
		if !assert.NoError(t, err) {
			return
		}

		tx := buildSignedTransaction(keys, sys.TagStake, 1, 1, payload)

		assert.Error(t, ValidateTransaction(state, tx))
	})

	t.Run("withdraw reward - not enough reward", func(t *testing.T) {
		payload, err := buildWithdrawRewardPayload(5004).Marshal()
		if !assert.NoError(t, err) {
			return
		}

		tx := buildSignedTransaction(keys, sys.TagStake, 1, 1, payload)

		assert.Error(t, ValidateTransaction(state, tx))
	})

	t.Run("withdraw reward - lower than minimum", func(t *testing.T) {
		// This error will be triggered by ParseStake() because ParseStake() also checks for the minimum reward.

		payload, err := buildWithdrawRewardPayload(sys.MinimumRewardWithdraw - 1).Marshal()
		if !assert.NoError(t, err) {
			return
		}

		tx := buildSignedTransaction(keys, sys.TagStake, 1, 1, payload)

		assert.Error(t, ValidateTransaction(state, tx))
	})
}

func TestValidateTransferTransaction(t *testing.T) {
	state := avl.New(store.NewInmem())

	keys, err := skademlia.NewKeys(1, 1)
	if !assert.NoError(t, err) {
		return
	}

	WriteAccountBalance(state, keys.PublicKey(), 42)

	t.Run("sender not exist", func(t *testing.T) {
		keys, err := skademlia.NewKeys(1, 1)
		if !assert.NoError(t, err) {
			return
		}

		var recipient AccountID
		_, err = rand.Read(recipient[:])
		assert.NoError(t, err)

		payload, err := buildTransferPayload(recipient, 1).Marshal()
		if !assert.NoError(t, err) {
			return
		}

		tx := buildSignedTransaction(keys, sys.TagTransfer, 1, 1, payload)

		assert.Error(t, ValidateTransaction(state, tx))
	})

	t.Run("contract transaction on wallet address", func(t *testing.T) {
		sender, err := skademlia.NewKeys(1, 1)
		if !assert.NoError(t, err) {
			return
		}
		WriteAccountBalance(state, sender.PublicKey(), 10)

		payload, err := buildTransferWithInvocationPayload(keys.PublicKey(), 1, 10, []byte{}, []byte{}, 10).Marshal()
		if !assert.NoError(t, err) {
			return
		}

		tx := buildSignedTransaction(sender, sys.TagTransfer, 1, 1, payload)

		assert.Error(t, ValidateTransaction(state, tx))
	})

	t.Run("sender not enough balance - wallet tx", func(t *testing.T) {
		var recipient AccountID
		_, err = rand.Read(recipient[:])
		assert.NoError(t, err)

		payload, err := buildTransferPayload(recipient, 5004).Marshal()
		if !assert.NoError(t, err) {
			return
		}

		tx := buildSignedTransaction(keys, sys.TagTransfer, 1, 1, payload)
		assert.Error(t, ValidateTransaction(state, tx))
	})

	t.Run("sender not enough balance - contract tx", func(t *testing.T) {
		var contractID AccountID
		_, err := rand.Read(contractID[:])
		if !assert.NoError(t, err) {
			return
		}

		var contractCode [32]byte
		_, err = rand.Read(contractCode[:])
		if !assert.NoError(t, err) {
			return
		}

		WriteAccountContractCode(state, contractID, contractCode[:])

		payload, err := buildTransferWithInvocationPayload(contractID, 5004, 1, []byte{}, []byte{}, 1).Marshal()
		if !assert.NoError(t, err) {
			return
		}

		tx := buildSignedTransaction(keys, sys.TagTransfer, 1, 1, payload)

		assert.Error(t, ValidateTransaction(state, tx))
	})
}

func TestValidateContractTransaction(t *testing.T) {
	state := avl.New(store.NewInmem())

	keys, err := skademlia.NewKeys(1, 1)
	if !assert.NoError(t, err) {
		return
	}

	WriteAccountBalance(state, keys.PublicKey(), 42)

	var contractCode [32]byte
	_, err = rand.Read(contractCode[:])
	if !assert.NoError(t, err) {
		return
	}

	payload, err := buildContractSpawnPayload(1, 1, contractCode[:]).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	tx := buildSignedTransaction(keys, sys.TagContract, 1, 1, payload)
	contractID := tx.ID

	WriteAccountContractCode(state, contractID, contractCode[:])

	t.Run("contract  already exists", func(t *testing.T) {
		payload, err := buildContractSpawnPayload(1, 1, contractCode[:]).Marshal()
		if !assert.NoError(t, err) {
			return
		}

		tx := buildSignedTransaction(keys, sys.TagContract, 1, 1, payload)

		assert.Error(t, ValidateTransaction(state, tx))
	})

	t.Run("sender not enough balance", func(t *testing.T) {
		var contractCode [32]byte
		_, err = rand.Read(contractCode[:])
		if !assert.NoError(t, err) {
			return
		}

		payload, err := buildContractSpawnPayload(5004, 5004, contractCode[:]).Marshal()
		if !assert.NoError(t, err) {
			return
		}

		tx := buildSignedTransaction(keys, sys.TagContract, 1, 1, payload)

		assert.Error(t, ValidateTransaction(state, tx))
	})
}

func TestValidateBatchTransaction(t *testing.T) {
	state := avl.New(store.NewInmem())

	keys, err := skademlia.NewKeys(1, 1)
	if !assert.NoError(t, err) {
		return
	}

	var batch Batch
	assert.NoError(t, batch.AddStake(buildWithdrawStakePayload(100)))
	assert.NoError(t, batch.AddTransfer(buildTransferPayload(AccountID{}, 100)))

	payload, err := batch.Marshal()
	if !assert.NoError(t, err) {
		return
	}

	tx := buildSignedTransaction(keys, sys.TagBatch, 1, 1, payload)

	assert.Error(t, ValidateTransaction(state, tx))
}
