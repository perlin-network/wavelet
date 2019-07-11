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
	"bytes"
	"encoding/binary"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
)

type TxApplierTestAccount struct {
	keys   *skademlia.Keypair
	effect struct {
		Balance uint64
		Stake   uint64
	}
}

func TestApplyTransaction_Single(t *testing.T) {
	const InitialBalance = 100000000

	state := avl.New(store.NewInmem())
	var initialRoot *Transaction

	viewID := uint64(0)
	state.SetViewID(viewID)

	accounts := make(map[AccountID]*TxApplierTestAccount)
	accountIDs := make([]AccountID, 0)
	for i := 0; i < 60; i++ {
		keys, err := skademlia.NewKeys(1, 1)
		assert.NoError(t, err)
		account := &TxApplierTestAccount{
			keys: keys,
		}
		if i == 0 {
			initialRoot = AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
		}
		WriteAccountBalance(state, keys.PublicKey(), InitialBalance)
		account.effect.Balance = InitialBalance

		accounts[keys.PublicKey()] = account
		accountIDs = append(accountIDs, keys.PublicKey())
	}

	rng := rand.New(rand.NewSource(42))

	round := NewRound(viewID, state.Checksum(), 0, &Transaction{}, initialRoot)

	for i := 0; i < 10000; i++ {
		switch rng.Intn(2) {
		case 0:
			amount := rng.Uint64()%100 + 1
			account := accounts[accountIDs[rng.Intn(len(accountIDs))]]
			account.effect.Stake += amount
			account.effect.Balance -= amount

			var intBuf [8]byte
			payload := bytes.NewBuffer(nil)
			payload.WriteByte(sys.PlaceStake)
			binary.LittleEndian.PutUint64(intBuf[:8], uint64(amount))
			payload.Write(intBuf[:8])

			tx := AttachSenderToTransaction(account.keys, NewTransaction(account.keys, sys.TagStake, payload.Bytes()))
			err := ApplyTransaction(&round, state, tx)
			assert.NoError(t, err)
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

			payload := bytes.NewBuffer(nil)
			payload.Write(toAccountID[:])
			var intBuf [8]byte
			binary.LittleEndian.PutUint64(intBuf[:], amount)
			payload.Write(intBuf[:])

			tx := AttachSenderToTransaction(fromAccount.keys, NewTransaction(fromAccount.keys, sys.TagTransfer, payload.Bytes()))
			err := ApplyTransaction(&round, state, tx)
			assert.NoError(t, err)
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

func TestApplyTransaction_Collapse(t *testing.T) {
	const InitialBalance = 100000000

	stateStore := store.NewInmem()
	state := avl.New(stateStore)
	var initialRoot *Transaction

	viewID := uint64(0)
	state.SetViewID(viewID)

	var graph *Graph

	accounts := make(map[AccountID]*TxApplierTestAccount)
	accountIDs := make([]AccountID, 0)
	for i := 0; i < 60; i++ {
		keys, err := skademlia.NewKeys(1, 1)
		assert.NoError(t, err)
		account := &TxApplierTestAccount{
			keys: keys,
		}
		if i == 0 {
			initialRoot = AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
			graph = NewGraph(WithRoot(initialRoot))
		}
		WriteAccountBalance(state, keys.PublicKey(), InitialBalance)
		account.effect.Balance = InitialBalance

		accounts[keys.PublicKey()] = account
		accountIDs = append(accountIDs, keys.PublicKey())
	}

	rng := rand.New(rand.NewSource(42))
	round := NewRound(viewID, state.Checksum(), 0, &Transaction{}, initialRoot)
	accountState := NewAccounts(stateStore)
	accountState.Commit(state)

	var criticalCount int

	for criticalCount < 100 {
		amount := rng.Uint64()%100 + 1
		account := accounts[accountIDs[rng.Intn(len(accountIDs))]]
		account.effect.Stake += amount

		var intBuf [8]byte
		payload := bytes.NewBuffer(nil)
		payload.WriteByte(sys.PlaceStake)
		binary.LittleEndian.PutUint64(intBuf[:8], uint64(amount))
		payload.Write(intBuf[:8])

		tx := AttachSenderToTransaction(account.keys, NewTransaction(account.keys, sys.TagStake, payload.Bytes()), graph.FindEligibleParents()...)
		err := graph.AddTransaction(tx)
		assert.NoError(t, err)
		if tx.IsCritical(4) {
			results, err := CollapseTransactions(graph, accountState, viewID+1, &round, round.End, tx, false)
			assert.NoError(t, err)
			err = accountState.Commit(results.snapshot)
			assert.NoError(t, err)
			state = results.snapshot
			round = NewRound(viewID+1, state.Checksum(), uint64(results.appliedCount), round.End, tx)
			viewID += 1

			for id, account := range accounts {
				stake, _ := ReadAccountStake(state, id)
				assert.Equal(t, stake, account.effect.Stake)
			}
			criticalCount++
		}
	}
}

func TestApplyTransferTransaction(t *testing.T) {
	state := avl.New(store.NewInmem())
	round := NewRound(0, state.Checksum(), 0, &Transaction{}, &Transaction{})
	alice, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)
	bob, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	aliceID := alice.PublicKey()
	bobID := bob.PublicKey()

	// Case 1 - Success
	WriteAccountBalance(state, aliceID, 1)

	tx := AttachSenderToTransaction(alice, NewTransaction(alice, sys.TagTransfer, buildTransferPayload(bobID, 1)))
	err = ApplyTransaction(&round, state, tx)
	assert.NoError(t, err)

	// Case 2 - Not enough balance
	tx = AttachSenderToTransaction(alice, NewTransaction(alice, sys.TagTransfer, buildTransferPayload(bobID, 1)))
	err = ApplyTransaction(&round, state, tx)
	assert.Error(t, err)

	// Case 3 - Self-transfer without enough balance
	tx = AttachSenderToTransaction(alice, NewTransaction(alice, sys.TagTransfer, buildTransferPayload(aliceID, 1)))
	err = ApplyTransaction(&round, state, tx)
	assert.Error(t, err)
}

func TestApplyStakeTransaction(t *testing.T) {
	state := avl.New(store.NewInmem())
	round := NewRound(0, state.Checksum(), 0, &Transaction{}, &Transaction{})
	account, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	accountID := account.PublicKey()

	// Case 1 - Placement success
	WriteAccountBalance(state, accountID, 100)

	tx := AttachSenderToTransaction(account, NewTransaction(account, sys.TagStake, buildPlaceStakePayload(100)))
	err = ApplyTransaction(&round, state, tx)
	assert.NoError(t, err)

	// Case 2 - Not enough balance
	tx = AttachSenderToTransaction(account, NewTransaction(account, sys.TagStake, buildPlaceStakePayload(100)))
	err = ApplyTransaction(&round, state, tx)
	assert.Error(t, err)

	// Case 3 - Withdrawal success
	tx = AttachSenderToTransaction(account, NewTransaction(account, sys.TagStake, buildWithdrawStakePayload(100)))
	err = ApplyTransaction(&round, state, tx)
	assert.NoError(t, err)

	finalBalance, _ := ReadAccountBalance(state, accountID)
	assert.Equal(t, finalBalance, uint64(100))
}

func TestApplyBatchTransaction(t *testing.T) {
	state := avl.New(store.NewInmem())
	round := NewRound(0, state.Checksum(), 0, &Transaction{}, &Transaction{})
	alice, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)
	bob, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	aliceID := alice.PublicKey()
	bobID := bob.PublicKey()

	WriteAccountBalance(state, aliceID, 100)

	// initial stake
	tx := AttachSenderToTransaction(alice, NewTransaction(alice, sys.TagStake, buildPlaceStakePayload(100)))
	err = ApplyTransaction(&round, state, tx)
	assert.NoError(t, err)

	// this implies order
	tx = AttachSenderToTransaction(alice, NewBatchTransaction(
		alice,
		[]byte{byte(sys.TagStake), byte(sys.TagTransfer)},
		[][]byte{buildWithdrawStakePayload(100), buildTransferPayload(bobID, 100)},
	))
	err = ApplyTransaction(&round, state, tx)
	assert.NoError(t, err)

	finalBobBalance, _ := ReadAccountBalance(state, bobID)
	assert.Equal(t, finalBobBalance, uint64(100))
}

func buildTransferPayload(dest AccountID, amount uint64) []byte {
	payload := bytes.NewBuffer(nil)
	payload.Write(dest[:])
	var intBuf [8]byte
	binary.LittleEndian.PutUint64(intBuf[:], amount)
	payload.Write(intBuf[:])
	return payload.Bytes()
}

func buildPlaceStakePayload(amount uint64) []byte {
	var intBuf [8]byte
	payload := bytes.NewBuffer(nil)
	payload.WriteByte(sys.PlaceStake)
	binary.LittleEndian.PutUint64(intBuf[:8], uint64(amount))
	payload.Write(intBuf[:8])
	return payload.Bytes()
}

func buildWithdrawStakePayload(amount uint64) []byte {
	var intBuf [8]byte
	payload := bytes.NewBuffer(nil)
	payload.WriteByte(sys.WithdrawStake)
	binary.LittleEndian.PutUint64(intBuf[:8], uint64(amount))
	payload.Write(intBuf[:8])
	return payload.Bytes()
}
