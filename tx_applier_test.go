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

//import (
//	"io/ioutil"
//	"math/rand"
//	"testing"
//
//	"github.com/perlin-network/noise/skademlia"
//	"github.com/perlin-network/wavelet/avl"
//	"github.com/perlin-network/wavelet/store"
//	"github.com/perlin-network/wavelet/sys"
//	"github.com/stretchr/testify/assert"
//)
//
//type testAccount struct {
//	keys   *skademlia.Keypair
//	effect struct {
//		Balance uint64
//		Stake   uint64
//	}
//}
//
//func TestApplyTransaction_Single(t *testing.T) {
//	t.Parallel()
//
//	const InitialBalance = 100000000
//
//	state := avl.New(store.NewInmem())
//	var initialRoot Transaction
//
//	viewID := uint64(0)
//	state.SetViewID(viewID)
//
//	accounts := make(map[AccountID]*testAccount)
//	accountIDs := make([]AccountID, 0)
//
//	for i := 0; i < 60; i++ {
//		keys, err := skademlia.NewKeys(1, 1)
//		assert.NoError(t, err)
//		account := &testAccount{
//			keys: keys,
//		}
//		if i == 0 {
//			initialRoot = AttachSenderToTransaction(keys, NewTransaction(sys.TagNop, nil))
//		}
//		WriteAccountBalance(state, keys.PublicKey(), InitialBalance)
//		account.effect.Balance = InitialBalance
//
//		accounts[keys.PublicKey()] = account
//		accountIDs = append(accountIDs, keys.PublicKey())
//	}
//
//	rng := rand.New(rand.NewSource(42))
//
//	block := NewBlock(viewID, state.Checksum(), 0, Transaction{}, initialRoot)
//
//	for i := 0; i < 10000; i++ {
//		switch rng.Intn(2) {
//		case 0:
//			amount := rng.Uint64()%100 + 1
//
//			account := accounts[accountIDs[rng.Intn(len(accountIDs))]]
//			account.effect.Stake += amount
//			account.effect.Balance -= amount
//
//			tx := AttachSenderToTransaction(account.keys, NewTransaction(sys.TagStake, buildPlaceStakePayload(amount).Marshal()))
//			assert.NoError(t, ApplyTransaction(state, &round, &tx))
//		case 1:
//			amount := rng.Uint64()%100 + 1
//
//			fromAccount := accounts[accountIDs[rng.Intn(len(accountIDs))]]
//			toAccount := accounts[accountIDs[rng.Intn(len(accountIDs))]]
//
//			if fromAccount.keys.PublicKey() == toAccount.keys.PublicKey() {
//				continue
//			}
//
//			toAccountID := toAccount.keys.PublicKey()
//
//			fromAccount.effect.Balance -= amount
//			toAccount.effect.Balance += amount
//
//			tx := AttachSenderToTransaction(fromAccount.keys, NewTransaction(sys.TagTransfer, buildTransferPayload(toAccountID, amount).Marshal()))
//			assert.NoError(t, ApplyTransaction(state, &round, &tx))
//		default:
//			panic("unreachable")
//		}
//	}
//
//	for id, account := range accounts {
//		stake, _ := ReadAccountStake(state, id)
//		assert.Equal(t, stake, account.effect.Stake)
//
//		balance, _ := ReadAccountBalance(state, id)
//		assert.Equal(t, balance, account.effect.Balance)
//	}
//}
//
//func TestApplyTransaction_Collapse(t *testing.T) {
//	t.Parallel()
//
//	const InitialBalance = 100000000
//
//	stateStore := store.NewInmem()
//	state := avl.New(stateStore)
//	var initialRoot Transaction
//
//	viewID := uint64(0)
//	state.SetViewID(viewID)
//
//	var graph *Graph
//
//	accounts := make(map[AccountID]*testAccount)
//	accountIDs := make([]AccountID, 0)
//	for i := 0; i < 60; i++ {
//		keys, err := skademlia.NewKeys(1, 1)
//		assert.NoError(t, err)
//		account := &testAccount{
//			keys: keys,
//		}
//		if i == 0 {
//			initialRoot = AttachSenderToTransaction(keys, NewTransaction(sys.TagNop, nil))
//			graph = NewGraph(WithRoot(initialRoot))
//		}
//		WriteAccountBalance(state, keys.PublicKey(), InitialBalance)
//		account.effect.Balance = InitialBalance
//
//		accounts[keys.PublicKey()] = account
//		accountIDs = append(accountIDs, keys.PublicKey())
//	}
//
//	assert.NotNil(t, graph)
//
//	rng := rand.New(rand.NewSource(42))
//	block := NewBlock(viewID, state.Checksum(), 0, Transaction{}, initialRoot)
//
//	accountState := NewAccounts(stateStore)
//	assert.NoError(t, accountState.Commit(state))
//
//	var criticalCount int
//
//	for criticalCount < 100 {
//		amount := rng.Uint64()%100 + 1
//		account := accounts[accountIDs[rng.Intn(len(accountIDs))]]
//		account.effect.Stake += amount
//
//		tx := AttachSenderToTransaction(account.keys, NewTransaction(sys.TagStake, buildPlaceStakePayload(amount).Marshal()), graph.FindEligibleParents()...)
//		assert.NoError(t, graph.AddTransaction(tx))
//
//		if tx.IsCritical(4) {
//			results, err := collapseTransactions(graph, accountState, viewID+1, &round, round.End, tx, false)
//			assert.NoError(t, err)
//			err = accountState.Commit(results.snapshot)
//			assert.NoError(t, err)
//			state = results.snapshot
//			round = NewRound(viewID+1, state.Checksum(), uint32(results.appliedCount+results.rejectedCount), round.End, tx)
//			viewID += 1
//
//			for id, account := range accounts {
//				stake, _ := ReadAccountStake(state, id)
//				assert.Equal(t, stake, account.effect.Stake)
//			}
//			criticalCount++
//		}
//	}
//}
//
//func TestApplyTransferTransaction(t *testing.T) {
//	t.Parallel()
//
//	state := avl.New(store.NewInmem())
//	block := NewBlock(0, state.Checksum(), 0, Transaction{}, Transaction{})
//
//	alice, err := skademlia.NewKeys(1, 1)
//	assert.NoError(t, err)
//	bob, err := skademlia.NewKeys(1, 1)
//	assert.NoError(t, err)
//
//	aliceID := alice.PublicKey()
//	bobID := bob.PublicKey()
//
//	// Case 1 - Success
//	WriteAccountBalance(state, aliceID, 1)
//
//	tx := AttachSenderToTransaction(alice, NewTransaction(sys.TagTransfer, buildTransferPayload(bobID, 1).Marshal()))
//	assert.NoError(t, ApplyTransaction(state, &round, &tx))
//
//	// Case 2 - Not enough balance
//	tx = AttachSenderToTransaction(alice, NewTransaction(sys.TagTransfer, buildTransferPayload(bobID, 1).Marshal()))
//	assert.Error(t, ApplyTransaction(state, &round, &tx))
//
//	// Case 3 - Self-transfer without enough balance
//	tx = AttachSenderToTransaction(alice, NewTransaction(sys.TagTransfer, buildTransferPayload(aliceID, 1).Marshal()))
//	assert.Error(t, ApplyTransaction(state, &round, &tx))
//}
//
//func TestApplyStakeTransaction(t *testing.T) {
//	t.Parallel()
//
//	state := avl.New(store.NewInmem())
//	block := NewBlock(0, state.Checksum(), 0, Transaction{}, Transaction{})
//
//	account, err := skademlia.NewKeys(1, 1)
//	assert.NoError(t, err)
//
//	accountID := account.PublicKey()
//
//	// Case 1 - Placement success
//	WriteAccountBalance(state, accountID, 100)
//
//	tx := AttachSenderToTransaction(account, NewTransaction(sys.TagStake, buildPlaceStakePayload(100).Marshal()))
//	assert.NoError(t, ApplyTransaction(state, &round, &tx))
//
//	// Case 2 - Not enough balance
//	tx = AttachSenderToTransaction(account, NewTransaction(sys.TagStake, buildPlaceStakePayload(100).Marshal()))
//	assert.Error(t, ApplyTransaction(state, &round, &tx))
//
//	// Case 3 - Withdrawal success
//	tx = AttachSenderToTransaction(account, NewTransaction(sys.TagStake, buildWithdrawStakePayload(100).Marshal()))
//	assert.NoError(t, ApplyTransaction(state, &round, &tx))
//
//	finalBalance, _ := ReadAccountBalance(state, accountID)
//	assert.Equal(t, finalBalance, uint64(100))
//}
//
//func TestApplyBatchTransaction(t *testing.T) {
//	t.Parallel()
//
//	state := avl.New(store.NewInmem())
//	block := NewBlock(0, state.Checksum(), 0, Transaction{}, Transaction{})
//
//	alice, err := skademlia.NewKeys(1, 1)
//	assert.NoError(t, err)
//	bob, err := skademlia.NewKeys(1, 1)
//	assert.NoError(t, err)
//
//	aliceID := alice.PublicKey()
//	bobID := bob.PublicKey()
//
//	WriteAccountBalance(state, aliceID, 100)
//
//	// initial stake
//	tx := AttachSenderToTransaction(alice, NewTransaction(sys.TagStake, buildPlaceStakePayload(100).Marshal()))
//	err = ApplyTransaction(state, &round, &tx)
//	assert.NoError(t, err)
//
//	// this implies order
//	var batch Batch
//	assert.NoError(t, batch.AddStake(buildWithdrawStakePayload(100)))
//	assert.NoError(t, batch.AddTransfer(buildTransferPayload(bobID, 100)))
//
//	tx = AttachSenderToTransaction(alice, NewTransaction(sys.TagBatch, batch.Marshal()))
//	assert.NoError(t, ApplyTransaction(state, &round, &tx))
//
//	finalBobBalance, _ := ReadAccountBalance(state, bobID)
//	assert.Equal(t, finalBobBalance, uint64(100))
//}
//
//func TestApplyContractTransaction(t *testing.T) {
//	t.Parallel()
//
//	state := avl.New(store.NewInmem())
//	block := NewBlock(0, state.Checksum(), 0, Transaction{}, Transaction{})
//
//	account, err := skademlia.NewKeys(1, 1)
//	assert.NoError(t, err)
//
//	accountID := account.PublicKey()
//
//	code, err := ioutil.ReadFile("testdata/transfer_back.wasm")
//	assert.NoError(t, err)
//
//	// Case 1 - balance < gas_fee
//	WriteAccountBalance(state, accountID, 99999)
//	tx := AttachSenderToTransaction(account, NewTransaction(sys.TagContract, buildContractSpawnPayload(100000, 0, code).Marshal()))
//	assert.Error(t, ApplyTransaction(state, &round, &tx))
//
//	// Case 2 - Success
//	WriteAccountBalance(state, accountID, 100000)
//	tx = AttachSenderToTransaction(account, NewTransaction(sys.TagContract, buildContractSpawnPayload(100000, 0, code).Marshal()))
//	assert.NoError(t, ApplyTransaction(state, &round, &tx))
//
//	finalBalance, _ := ReadAccountBalance(state, accountID)
//	assert.Condition(t, func() bool { return finalBalance > 0 && finalBalance < 100000 })
//
//	contractID := tx.ID
//
//	// Try to deposit gas
//	WriteAccountBalance(state, accountID, 1000000000)
//	tx = AttachSenderToTransaction(account, NewTransaction(sys.TagTransfer, buildTransferWithInvocationPayload(contractID, 0, 0, nil, nil, 1337).Marshal()))
//
//	assert.NoError(t, ApplyTransaction(state, &round, &tx))
//
//	gasDeposit, _ := ReadAccountContractGasBalance(state, contractID)
//	assert.EqualValues(t, 1337, gasDeposit)
//
//	// Try to transfer some money
//	WriteAccountBalance(state, accountID, 1000000000)
//	tx = AttachSenderToTransaction(account, NewTransaction(sys.TagTransfer, buildTransferWithInvocationPayload(contractID, 200000000, 500000, []byte("on_money_received"), nil, 0).Marshal()))
//
//	assert.NoError(t, ApplyTransaction(state, &round, &tx))
//
//	finalBalance, _ = ReadAccountBalance(state, accountID)
//	assert.True(t, finalBalance > 1000000000-100000000-500000 && finalBalance < 1000000000-100000000)
//
//	// Try to invoke with contract gas balance
//	WriteAccountBalance(state, accountID, 200000000)
//	WriteAccountContractGasBalance(state, contractID, 1000000000)
//
//	tx = AttachSenderToTransaction(account, NewTransaction(sys.TagTransfer, buildTransferWithInvocationPayload(contractID, 200000000, 500000, []byte("on_money_received"), nil, 0).Marshal()))
//	assert.NoError(t, ApplyTransaction(state, &round, &tx))
//
//	finalBalance, _ = ReadAccountBalance(state, accountID)
//	assert.Equal(t, uint64(100000000), finalBalance)
//	finalGasBalance, _ := ReadAccountContractGasBalance(state, contractID)
//	assert.True(t, finalGasBalance > 0 && finalGasBalance < 1000000000)
//
//	// Try to invoke with both balances
//	WriteAccountBalance(state, accountID, 300000000)
//	WriteAccountContractGasBalance(state, contractID, 10)
//
//	tx = AttachSenderToTransaction(account, NewTransaction(sys.TagTransfer, buildTransferWithInvocationPayload(contractID, 200000000, 500000, []byte("on_money_received"), nil, 0).Marshal()))
//	assert.NoError(t, ApplyTransaction(state, &round, &tx))
//
//	finalBalance, _ = ReadAccountBalance(state, accountID)
//	assert.True(t, finalBalance > 200000000-500000 && finalBalance < 200000000)
//	finalGasBalance, _ = ReadAccountContractGasBalance(state, contractID)
//	assert.Equal(t, finalGasBalance, uint64(0))
//
//	// Now it should fail
//	WriteAccountBalance(state, accountID, 200000000)
//	WriteAccountContractGasBalance(state, contractID, 0)
//
//	tx = AttachSenderToTransaction(account, NewTransaction(sys.TagTransfer, buildTransferWithInvocationPayload(contractID, 200000000, 500000, []byte("on_money_received"), nil, 0).Marshal()))
//	assert.Error(t, ApplyTransaction(state, &round, &tx))
//
//	code, err = ioutil.ReadFile("testdata/recursive_invocation.wasm")
//	assert.NoError(t, err)
//
//	WriteAccountBalance(state, accountID, 100000000)
//	tx = AttachSenderToTransaction(account, NewTransaction(sys.TagContract, buildContractSpawnPayload(100000, 0, code).Marshal()))
//	assert.NoError(t, ApplyTransaction(state, &round, &tx))
//
//	recursiveInvocationContractID := tx.ID
//
//	WriteAccountBalance(state, accountID, 6000000)
//	tx = AttachSenderToTransaction(account, NewTransaction(sys.TagTransfer, buildTransferWithInvocationPayload(recursiveInvocationContractID, 0, 5000000, []byte("bomb"), recursiveInvocationContractID[:], 0).Marshal()))
//	assert.NoError(t, ApplyTransaction(state, &round, &tx))
//
//	finalBalance, _ = ReadAccountBalance(state, accountID)
//	assert.True(t, finalBalance >= 1000000 && finalBalance < 2000000) // GasLimit specified in contract is 1000000
//}
//
//func buildTransferWithInvocationPayload(dest AccountID, amount uint64, gasLimit uint64, funcName []byte, param []byte, gasDeposit uint64) Transfer {
//
//	return Transfer{
//		Recipient:  dest,
//		Amount:     amount,
//		GasLimit:   gasLimit,
//		GasDeposit: gasDeposit,
//		FuncName:   funcName,
//		FuncParams: param,
//	}
//}
//
//func buildContractSpawnPayload(gasLimit, gasDeposit uint64, code []byte) Contract {
//	return Contract{
//		GasLimit:   gasLimit,
//		GasDeposit: gasDeposit,
//		Code:       code,
//	}
//}
//
//func buildTransferPayload(dest AccountID, amount uint64) Transfer {
//	return Transfer{
//		Recipient: dest,
//		Amount:    amount,
//	}
//}
//
//func buildPlaceStakePayload(amount uint64) Stake {
//	return Stake{
//		Opcode: sys.PlaceStake,
//		Amount: amount,
//	}
//}
//
//func buildWithdrawStakePayload(amount uint64) Stake {
//	return Stake{
//		Opcode: sys.WithdrawStake,
//		Amount: amount,
//	}
//}
