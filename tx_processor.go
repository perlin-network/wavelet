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
	"github.com/gogo/protobuf/sortkeys"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/log"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"sort"
)

type TransactionProcessor func(ctx *TransactionContext) error

type TransactionContext struct {
	round *Round
	tree  *avl.Tree

	balances map[AccountID]uint64
	stakes   map[AccountID]uint64
	rewards  map[AccountID]uint64

	contracts        map[TransactionID][]byte
	contractNumPages map[TransactionID]uint64
	contractPages    map[TransactionID]map[uint64][]byte

	transactions queue.Queue
	tx           *Transaction
}

func NewTransactionContext(round *Round, tree *avl.Tree, tx *Transaction) *TransactionContext {
	ctx := &TransactionContext{
		round: round,
		tree:  tree,

		balances:         make(map[AccountID]uint64),
		stakes:           make(map[AccountID]uint64),
		rewards:          make(map[AccountID]uint64),
		contracts:        make(map[TransactionID][]byte),
		contractNumPages: make(map[TransactionID]uint64),
		contractPages:    make(map[TransactionID]map[uint64][]byte),

		tx: tx,
	}

	ctx.transactions.PushBack(tx)

	return ctx
}

func (c *TransactionContext) Transaction() Transaction {
	return *c.tx
}

func (c *TransactionContext) SendTransaction(tx *Transaction) {
	c.transactions.PushBack(tx)
}

func (c *TransactionContext) ReadAccountBalance(id AccountID) (uint64, bool) {
	if balance, ok := c.balances[id]; ok {
		return balance, true
	}

	balance, exists := ReadAccountBalance(c.tree, id)
	if exists {
		c.WriteAccountBalance(id, balance)
	}

	return balance, exists
}

func (c *TransactionContext) ReadAccountStake(id AccountID) (uint64, bool) {
	if stake, ok := c.stakes[id]; ok {
		return stake, true
	}

	stake, exists := ReadAccountStake(c.tree, id)
	if exists {
		c.WriteAccountStake(id, stake)
	}

	return stake, exists
}

func (c *TransactionContext) ReadAccountReward(id AccountID) (uint64, bool) {
	if reward, ok := c.rewards[id]; ok {
		return reward, true
	}

	reward, exists := ReadAccountReward(c.tree, id)
	if exists {
		c.WriteAccountReward(id, reward)
	}

	return reward, exists
}

func (c *TransactionContext) ReadAccountContractCode(id TransactionID) ([]byte, bool) {
	if code, ok := c.contracts[id]; ok {
		return code, true
	}

	code, exists := ReadAccountContractCode(c.tree, id)
	if exists {
		c.WriteAccountContractCode(id, code)
	}

	return code, exists
}

func (c *TransactionContext) ReadAccountContractNumPages(id AccountID) (uint64, bool) {
	if numPages, ok := c.contractNumPages[id]; ok {
		return numPages, true
	}

	numPages, exists := ReadAccountContractNumPages(c.tree, id)
	if exists {
		c.WriteAccountContractNumPages(id, numPages)
	}

	return numPages, exists
}

func (c *TransactionContext) ReadAccountContractPage(id AccountID, idx uint64) ([]byte, bool) {
	if pages, ok := c.contractPages[id]; ok {
		if page, ok := pages[idx]; ok {
			return page, true
		}
	}

	page, exists := ReadAccountContractPage(c.tree, id, idx)
	if exists {
		c.WriteAccountContractPage(id, idx, page)
	}

	return page, exists
}

func (c *TransactionContext) WriteAccountBalance(id AccountID, balance uint64) {
	c.balances[id] = balance
}

func (c *TransactionContext) WriteAccountStake(id AccountID, stake uint64) {
	c.stakes[id] = stake
}

func (c *TransactionContext) WriteAccountReward(id AccountID, reward uint64) {
	c.rewards[id] = reward
}

func (c *TransactionContext) WriteAccountContractCode(id TransactionID, code []byte) {
	c.contracts[id] = code
}

func (c *TransactionContext) WriteAccountContractNumPages(id TransactionID, numPages uint64) {
	c.contractNumPages[id] = numPages
}

func (c *TransactionContext) WriteAccountContractPage(id TransactionID, idx uint64, page []byte) {
	pages, exist := c.contractPages[id]
	if !exist {
		pages = make(map[uint64][]byte)
		c.contractPages[id] = pages
	}

	pages[idx] = page
}

func (c *TransactionContext) apply(processors map[byte]TransactionProcessor) error {
	for c.transactions.Len() > 0 {
		c.tx = c.transactions.PopFront().(*Transaction)

		processor, exists := processors[c.tx.Tag]
		if !exists {
			return errors.Errorf("wavelet: transaction processor not registered for tag %d", c.tx.Tag)
		}

		err := processor(c)
		if err != nil {
			return errors.Wrap(err, "failed to apply transaction")
		}
	}

	balanceLogger := log.Accounts("balance_updated")
	stakeLogger := log.Accounts("stake_updated")
	pageLogger := log.Accounts("num_pages_updated")
	rewardLogger := log.Accounts("reward_updated")

	// If the transaction processor executed properly, apply changes from
	// the transactions context over to our accounts snapshot.

	balanceKeys := make([][]byte, 0, len(c.balances))
	stakeKeys := make([][]byte, 0, len(c.stakes))
	rewardKeys := make([][]byte, 0, len(c.rewards))
	codeKeys := make([][]byte, 0, len(c.contracts))
	numPagesKeys := make([][]byte, 0, len(c.contractNumPages))
	contractPagesKeys := make([][]byte, 0, len(c.contractPages))

	var buf [8]byte

	for id, balance := range c.balances {
		balanceLogger.Log().
			Hex("account_id", id[:]).
			Uint64("balance", balance).
			Msg("")

		binary.BigEndian.PutUint64(buf[:8], balance)
		balanceKeys = append(balanceKeys, append(id[:], buf[:8]...))
	}

	for id, stake := range c.stakes {
		stakeLogger.Log().
			Hex("account_id", id[:]).
			Uint64("stake", stake).
			Msg("")

		binary.BigEndian.PutUint64(buf[:8], stake)
		stakeKeys = append(stakeKeys, append(id[:], buf[:8]...))
	}

	for id, reward := range c.rewards {
		rewardLogger.Log().
			Hex("account_id", id[:]).
			Uint64("reward", reward).
			Msg("")

		binary.BigEndian.PutUint64(buf[:8], reward)
		rewardKeys = append(rewardKeys, append(id[:], buf[:8]...))
	}

	for id, code := range c.contracts {
		codeKeys = append(codeKeys, append(id[:], code...))
	}

	for id, numPages := range c.contractNumPages {
		pageLogger.Log().
			Hex("account_id", id[:]).
			Uint64("num_pages", numPages).
			Msg("")

		binary.BigEndian.PutUint64(buf[:8], numPages)
		numPagesKeys = append(numPagesKeys, append(id[:], buf[:8]...))
	}

	for id, pages := range c.contractPages {
		indices := make([]uint64, 0, len(pages))
		for idx := range pages {
			indices = append(indices, idx)
		}

		sort.Stable(sortkeys.Uint64Slice(indices))
		contractPagesKeys = append(contractPagesKeys, append(id[:], serializeUint64Slice(indices)...))
	}

	// Sort all keys deterministically such that the order in which the ledgers
	// state is modified is consistent.

	sort.SliceStable(balanceKeys, func(i, j int) bool {
		return bytes.Compare(balanceKeys[i], balanceKeys[j]) < 0
	})

	sort.SliceStable(stakeKeys, func(i, j int) bool {
		return bytes.Compare(stakeKeys[i], stakeKeys[j]) < 0
	})

	sort.SliceStable(rewardKeys, func(i, j int) bool {
		return bytes.Compare(rewardKeys[i], rewardKeys[j]) < 0
	})

	sort.SliceStable(codeKeys, func(i, j int) bool {
		return bytes.Compare(codeKeys[i], codeKeys[j]) < 0
	})

	sort.SliceStable(numPagesKeys, func(i, j int) bool {
		return bytes.Compare(numPagesKeys[i], numPagesKeys[j]) < 0
	})

	sort.SliceStable(contractPagesKeys, func(i, j int) bool {
		return bytes.Compare(contractPagesKeys[i], contractPagesKeys[j]) < 0
	})

	for _, key := range balanceKeys {
		var id AccountID
		copy(id[:], key[:SizeAccountID])

		balance := binary.BigEndian.Uint64(key[SizeAccountID:])

		WriteAccountBalance(c.tree, id, balance)
	}

	for _, key := range stakeKeys {
		var id AccountID
		copy(id[:], key[:SizeAccountID])

		stake := binary.BigEndian.Uint64(key[SizeAccountID:])

		WriteAccountStake(c.tree, id, stake)
	}

	for _, key := range rewardKeys {
		var id AccountID
		copy(id[:], key[:SizeAccountID])

		reward := binary.BigEndian.Uint64(key[SizeAccountID:])

		WriteAccountReward(c.tree, id, reward)
		StoreRewardWithdrawalRequest(c.tree, RewardWithdrawalRequest{
			accountID: id,
			amount:    reward,
			round:     c.round.Index,
		})
	}

	for _, key := range codeKeys {
		var id AccountID
		copy(id[:], key[:SizeAccountID])

		WriteAccountContractCode(c.tree, id, key[SizeAccountID:])
	}

	for _, key := range numPagesKeys {
		var id AccountID
		copy(id[:], key[:SizeAccountID])

		numPages := binary.BigEndian.Uint64(key[SizeAccountID:])

		WriteAccountContractNumPages(c.tree, id, numPages)
	}

	for _, key := range contractPagesKeys {
		var id AccountID
		copy(id[:], key[:SizeAccountID])

		indices := deserializeUint64Slice(key[SizeAccountID:])

		for _, idx := range indices {
			WriteAccountContractPage(c.tree, id, idx, c.contractPages[id][idx])
		}
	}

	return nil
}

func serializeUint64Slice(slice []uint64) []byte {
	buf := make([]byte, 4+8*len(slice))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(slice)))

	for i, val := range slice {
		binary.BigEndian.PutUint64(buf[4+i*8:4+i*8+8], val)
	}

	return buf
}

func deserializeUint64Slice(buf []byte) []uint64 {
	slice := make([]uint64, binary.BigEndian.Uint32(buf[:4]))

	for i := range slice {
		slice[i] = binary.BigEndian.Uint64(buf[4+i*8 : 4+i*8+8])
	}

	return slice
}
