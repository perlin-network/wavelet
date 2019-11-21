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

// +build !integration,unit

package wavelet

import (
	"encoding/binary"
	"io/ioutil"
	"math/rand"
	"sync/atomic"
	"testing"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
)

type testAccount struct {
	keys   *skademlia.Keypair
	effect struct {
		Balance uint64
		Stake   uint64
	}
}

func TestApplyTransaction_Single(t *testing.T) {
	t.Parallel()

	const InitialBalance = 100000000

	state := avl.New(store.NewInmem())

	viewID := uint64(0)
	state.SetViewID(viewID)

	accounts := make(map[AccountID]*testAccount)
	accountIDs := make([]AccountID, 0)

	for i := 0; i < 60; i++ {
		keys, err := skademlia.NewKeys(1, 1)
		assert.NoError(t, err)
		account := &testAccount{
			keys: keys,
		}

		WriteAccountBalance(state, keys.PublicKey(), InitialBalance)
		account.effect.Balance = InitialBalance

		accounts[keys.PublicKey()] = account
		accountIDs = append(accountIDs, keys.PublicKey())
	}

	rng := rand.New(rand.NewSource(42))

	block, err := NewBlock(viewID, state.Checksum())
	if !assert.NoError(t, err) {
		return
	}

	var nonce uint64

	for i := 0; i < 10000; i++ {
		switch rng.Intn(2) {
		case 0:
			amount := rng.Uint64()%100 + 1

			account := accounts[accountIDs[rng.Intn(len(accountIDs))]]
			account.effect.Stake += amount
			account.effect.Balance -= amount

			payload, err := buildPlaceStakePayload(amount).Marshal()
			if !assert.NoError(t, err) {
				return
			}

			tx := buildSignedTransaction(
				account.keys, sys.TagStake,
				atomic.AddUint64(&nonce, 1), block.Index+1,
				payload,
			)

			assert.NoError(t, ApplyTransaction(state, &block, &tx))

		case 1:
			amount := rng.Uint64()%100 + 1

			fromAccount := accounts[accountIDs[rng.Intn(len(accountIDs))]]
			toAccount := accounts[accountIDs[rng.Intn(len(accountIDs))]]

			if fromAccount.keys.PublicKey() == toAccount.keys.PublicKey() {
				continue
			}

			toAccountID := toAccount.keys.PublicKey()

			fromAccount.effect.Balance -= amount
			toAccount.effect.Balance += amount

			payload, err := buildTransferPayload(toAccountID, amount).Marshal()
			if !assert.NoError(t, err) {
				return
			}

			tx := buildSignedTransaction(
				fromAccount.keys, sys.TagTransfer,
				atomic.AddUint64(&nonce, 1), block.Index+1,
				payload,
			)

			assert.NoError(t, ApplyTransaction(state, &block, &tx))

		default:
			panic("unreachable")
		}
	}

	for id, account := range accounts {
		stake, _ := ReadAccountStake(state, id)
		assert.Equal(t, stake, account.effect.Stake)

		balance, _ := ReadAccountBalance(state, id)
		assert.Equal(t, balance, account.effect.Balance)
	}
}

func TestApplyTransferTransaction(t *testing.T) {
	t.Parallel()

	state := avl.New(store.NewInmem())
	block, err := NewBlock(0, state.Checksum())
	if !assert.NoError(t, err) {
		return
	}

	alice, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)
	bob, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	aliceID := alice.PublicKey()
	bobID := bob.PublicKey()

	var nonce uint64

	// Case 1 - Success
	WriteAccountBalance(state, aliceID, 1)

	payload, err := buildTransferPayload(bobID, 1).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	tx := buildSignedTransaction(
		alice, sys.TagTransfer,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.NoError(t, ApplyTransaction(state, &block, &tx))

	// Case 2 - Not enough balance
	payload, err = buildTransferPayload(bobID, 1).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	tx = buildSignedTransaction(
		alice, sys.TagTransfer,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.Error(t, ApplyTransaction(state, &block, &tx))

	// Case 3 - Self-transfer without enough balance
	payload, err = buildTransferPayload(aliceID, 1).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	tx = buildSignedTransaction(
		alice, sys.TagTransfer,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.Error(t, ApplyTransaction(state, &block, &tx))
}

func TestApplyStakeTransaction(t *testing.T) {
	t.Parallel()

	state := avl.New(store.NewInmem())
	block, err := NewBlock(0, state.Checksum())
	if !assert.NoError(t, err) {
		return
	}

	account, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	accountID := account.PublicKey()
	var nonce uint64

	// Case 1 - Placement success
	WriteAccountBalance(state, accountID, 100)

	payload, err := buildPlaceStakePayload(100).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	tx := buildSignedTransaction(
		account, sys.TagStake,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.NoError(t, ApplyTransaction(state, &block, &tx))

	// Case 2 - Not enough balance
	payload, err = buildPlaceStakePayload(100).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	tx = buildSignedTransaction(
		account, sys.TagStake,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.Error(t, ApplyTransaction(state, &block, &tx))

	// Case 3 - Withdrawal success
	payload, err = buildWithdrawStakePayload(100).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	tx = buildSignedTransaction(
		account, sys.TagStake,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.NoError(t, ApplyTransaction(state, &block, &tx))

	finalBalance, _ := ReadAccountBalance(state, accountID)
	assert.Equal(t, finalBalance, uint64(100))
}

func TestApplyBatchTransaction(t *testing.T) {
	t.Parallel()

	state := avl.New(store.NewInmem())
	block, err := NewBlock(0, state.Checksum())
	if !assert.NoError(t, err) {
		return
	}

	alice, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)
	bob, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	aliceID := alice.PublicKey()
	bobID := bob.PublicKey()
	var nonce uint64

	WriteAccountBalance(state, aliceID, 100)

	payload, err := buildPlaceStakePayload(100).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	// initial stake
	tx := buildSignedTransaction(
		alice, sys.TagStake,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	err = ApplyTransaction(state, &block, &tx)
	assert.NoError(t, err)

	// this implies order
	var batch Batch
	assert.NoError(t, batch.AddStake(buildWithdrawStakePayload(100)))
	assert.NoError(t, batch.AddTransfer(buildTransferPayload(bobID, 100)))

	payload, err = batch.Marshal()
	if !assert.NoError(t, err) {
		return
	}

	tx = buildSignedTransaction(
		alice, sys.TagBatch,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.NoError(t, ApplyTransaction(state, &block, &tx))

	finalBobBalance, _ := ReadAccountBalance(state, bobID)
	assert.Equal(t, finalBobBalance, uint64(100))
}

func TestApplyContractTransaction(t *testing.T) {
	t.Parallel()

	state := avl.New(store.NewInmem())
	block, err := NewBlock(0, state.Checksum())
	if !assert.NoError(t, err) {
		return
	}

	account, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	accountID := account.PublicKey()
	var nonce uint64

	code, err := ioutil.ReadFile("testdata/transfer_back.wasm")
	assert.NoError(t, err)

	// Case 1 - balance < gas_fee
	payload, err := buildContractSpawnPayload(100000, 0, code).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	WriteAccountBalance(state, accountID, 99999)
	tx := buildSignedTransaction(
		account, sys.TagContract,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.Error(t, ApplyTransaction(state, &block, &tx))

	// Case 2 - Success
	payload, err = buildContractSpawnPayload(100000, 0, code).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	WriteAccountBalance(state, accountID, 100000)
	tx = buildSignedTransaction(
		account, sys.TagContract,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.NoError(t, ApplyTransaction(state, &block, &tx))

	finalBalance, _ := ReadAccountBalance(state, accountID)
	assert.Condition(t, func() bool { return finalBalance > 0 && finalBalance < 100000 })

	contractID := tx.ID

	// Try to deposit gas
	payload, err = buildTransferWithInvocationPayload(contractID, 0, 0, nil, nil, 1337).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	WriteAccountBalance(state, accountID, 1000000000)
	tx = buildSignedTransaction(
		account, sys.TagTransfer,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)

	assert.NoError(t, ApplyTransaction(state, &block, &tx))

	gasDeposit, _ := ReadAccountContractGasBalance(state, contractID)
	assert.EqualValues(t, 1337, gasDeposit)

	// Try to transfer some money
	payload, err = buildTransferWithInvocationPayload(contractID, 200000000, 500000, []byte("on_money_received"), nil, 0).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	WriteAccountBalance(state, accountID, 1000000000)
	tx = buildSignedTransaction(
		account, sys.TagTransfer,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)

	assert.NoError(t, ApplyTransaction(state, &block, &tx))

	finalBalance, _ = ReadAccountBalance(state, accountID)
	assert.True(t, finalBalance > 1000000000-100000000-500000 && finalBalance < 1000000000-100000000)

	// Try to invoke with contract gas balance
	WriteAccountBalance(state, accountID, 200000000)
	WriteAccountContractGasBalance(state, contractID, 1000000000)

	payload, err = buildTransferWithInvocationPayload(contractID, 200000000, 500000, []byte("on_money_received"), nil, 0).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	tx = buildSignedTransaction(
		account, sys.TagTransfer,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.NoError(t, ApplyTransaction(state, &block, &tx))

	finalBalance, _ = ReadAccountBalance(state, accountID)
	assert.Equal(t, uint64(100000000), finalBalance)
	finalGasBalance, _ := ReadAccountContractGasBalance(state, contractID)
	assert.True(t, finalGasBalance > 0 && finalGasBalance < 1000000000)

	// Try to invoke with both balances
	WriteAccountBalance(state, accountID, 300000000)
	WriteAccountContractGasBalance(state, contractID, 10)

	payload, err = buildTransferWithInvocationPayload(contractID, 200000000, 500000, []byte("on_money_received"), nil, 0).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	tx = buildSignedTransaction(
		account, sys.TagTransfer,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.NoError(t, ApplyTransaction(state, &block, &tx))

	finalBalance, _ = ReadAccountBalance(state, accountID)
	assert.True(t, finalBalance > 200000000-500000 && finalBalance < 200000000)
	finalGasBalance, _ = ReadAccountContractGasBalance(state, contractID)
	assert.Equal(t, finalGasBalance, uint64(0))

	// Now it should fail
	WriteAccountBalance(state, accountID, 200000000)
	WriteAccountContractGasBalance(state, contractID, 0)

	payload, err = buildTransferWithInvocationPayload(contractID, 200000000, 500000, []byte("on_money_received"), nil, 0).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	tx = buildSignedTransaction(
		account, sys.TagTransfer,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.Error(t, ApplyTransaction(state, &block, &tx))

	code, err = ioutil.ReadFile("testdata/recursive_invocation.wasm")
	assert.NoError(t, err)

	payload, err = buildContractSpawnPayload(100000, 0, code).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	WriteAccountBalance(state, accountID, 100000000)
	tx = buildSignedTransaction(
		account, sys.TagContract,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.NoError(t, ApplyTransaction(state, &block, &tx))

	recursiveInvocationContractID := tx.ID

	payload, err = buildTransferWithInvocationPayload(recursiveInvocationContractID, 0, 5000000, []byte("bomb"), recursiveInvocationContractID[:], 0).Marshal()
	if !assert.NoError(t, err) {
		return
	}

	WriteAccountBalance(state, accountID, 6000000)
	tx = buildSignedTransaction(
		account, sys.TagTransfer,
		atomic.AddUint64(&nonce, 1), block.Index+1,
		payload,
	)
	assert.NoError(t, ApplyTransaction(state, &block, &tx))

	finalBalance, _ = ReadAccountBalance(state, accountID)
	assert.True(t, finalBalance >= 1000000 && finalBalance < 2000000) // GasLimit specified in contract is 1000000
}

func buildTransferWithInvocationPayload(dest AccountID, amount uint64, gasLimit uint64, funcName []byte, param []byte, gasDeposit uint64) Transfer {
	return Transfer{
		Recipient:  dest,
		Amount:     amount,
		GasLimit:   gasLimit,
		GasDeposit: gasDeposit,
		FuncName:   funcName,
		FuncParams: param,
	}
}

func buildContractSpawnPayload(gasLimit, gasDeposit uint64, code []byte) Contract {
	return Contract{
		GasLimit:   gasLimit,
		GasDeposit: gasDeposit,
		Code:       code,
	}
}

func buildTransferPayload(dest AccountID, amount uint64) Transfer {
	return Transfer{
		Recipient: dest,
		Amount:    amount,
	}
}

func buildPlaceStakePayload(amount uint64) Stake {
	return Stake{
		Opcode: sys.PlaceStake,
		Amount: amount,
	}
}

func buildWithdrawStakePayload(amount uint64) Stake {
	return Stake{
		Opcode: sys.WithdrawStake,
		Amount: amount,
	}
}

func buildSignedTransaction(keys *skademlia.Keypair, tag sys.Tag, nonce uint64, block uint64, payload []byte) Transaction {
	var nonceBuf [8]byte
	binary.BigEndian.PutUint64(nonceBuf[:], nonce)

	var blockBuf [8]byte
	binary.BigEndian.PutUint64(blockBuf[:], block)

	signature := edwards25519.Sign(
		keys.PrivateKey(),
		append(nonceBuf[:], append(blockBuf[:], append([]byte{byte(tag)}, payload...)...)...),
	)

	return NewSignedTransaction(
		keys.PublicKey(), nonce, block,
		tag, payload, signature,
	)
}
