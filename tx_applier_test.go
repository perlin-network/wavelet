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

	state := avl.New(store.NewInmem())
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

	var criticalCount int

	for criticalCount < 1000 {
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
			results, err := graph.CollapseTransactions(state, viewID+1, &round, round.End, tx, false)
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
