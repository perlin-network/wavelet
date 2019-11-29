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
	"encoding/hex"

	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/lru"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
)

func collapseTransactions(txs []*Transaction, block *Block,
	accounts *Accounts) (*collapseResults, error) {

	res := &collapseResults{snapshot: accounts.Snapshot()}
	res.snapshot.SetViewID(block.Index)

	res.applied = make([]*Transaction, 0, len(txs))
	res.rejected = make([]*Transaction, 0, len(txs))
	res.rejectedErrors = make([]error, 0, len(txs))

	ctx := NewCollapseContext(res.snapshot)

	var (
		totalStake uint64
		totalFee   uint64
	)

	stakes := make(map[AccountID]uint64)

	// Apply transactions in reverse order from the end of the round
	// all the way down to the beginning of the round.
	for _, tx := range txs {
		// Update nonce.

		nonce, exists := ctx.ReadAccountNonce(tx.Sender)
		if !exists {
			ctx.WriteAccountsLen(ctx.ReadAccountsLen() + 1)
		}
		ctx.WriteAccountNonce(tx.Sender, nonce+1)

		if hex.EncodeToString(tx.Sender[:]) != sys.FaucetAddress {
			fee := tx.Fee()

			senderBalance, _ := ctx.ReadAccountBalance(tx.Sender)
			if senderBalance < fee {
				res.rejected = append(res.rejected, tx)
				res.rejectedErrors = append(
					res.rejectedErrors,
					errors.Errorf("stake: sender %x does not have enough PERLs"+
						" to pay transaction fees (comprised of %d PERLs)",
						tx.Sender, fee),
				)
				res.rejectedCount += tx.LogicalUnits()

				continue
			}

			ctx.WriteAccountBalance(tx.Sender, senderBalance-fee)
			totalFee += fee

			stake, _ := ctx.ReadAccountStake(tx.Sender)

			if stake >= sys.MinimumStake {
				if _, ok := stakes[tx.Sender]; !ok {
					stakes[tx.Sender] = stake
				} else {
					stakes[tx.Sender] += stake
				}
				totalStake += stake
			}
		}

		if err := ctx.ApplyTransaction(block, tx); err != nil {
			res.rejected = append(res.rejected, tx)
			res.rejectedErrors = append(res.rejectedErrors, err)
			res.rejectedCount += tx.LogicalUnits()

			log.ErrNode(err, "error applying transaction")
			continue
		}

		// Update statistics.

		res.applied = append(res.applied, tx)
		res.appliedCount += tx.LogicalUnits()
	}

	if totalStake > 0 {
		for sender, stake := range stakes {
			rewardeeBalance, _ := ctx.ReadAccountReward(sender)

			reward := float64(totalFee) * (float64(stake) / float64(totalStake))
			ctx.WriteAccountReward(sender, rewardeeBalance+uint64(reward))
		}
	}

	ctx.processRewardWithdrawals(block.Index)

	if err := ctx.Flush(); err != nil {
		return res, err
	}

	return res, nil
}

// WARNING: While using this, the tree must not be modified.
type CollapseContext struct {
	tree     *avl.Tree
	checksum [16]byte

	accountLen uint64

	// To preserve order of state insertions of accounts
	accountIDs []AccountID
	accounts   map[AccountID]struct{}

	writes map[AccountID]struct{}

	balances            map[AccountID]uint64
	stakes              map[AccountID]uint64
	rewards             map[AccountID]uint64
	nonces              map[AccountID]uint64
	contracts           map[TransactionID][]byte
	contractGasBalances map[TransactionID]uint64
	contractVMs         map[AccountID]*VMState

	rewardWithdrawalRequests []RewardWithdrawalRequest

	VMCache *lru.LRU
}

func NewCollapseContext(tree *avl.Tree) *CollapseContext {
	ctx := &CollapseContext{
		tree: tree,
	}

	ctx.init()

	return ctx
}

func (c *CollapseContext) init() {
	c.checksum = c.tree.Checksum()

	c.accountLen = ReadAccountsLen(c.tree)

	c.accounts = make(map[AccountID]struct{})
	c.balances = make(map[AccountID]uint64)
	c.stakes = make(map[AccountID]uint64)
	c.rewards = make(map[AccountID]uint64)
	c.nonces = make(map[AccountID]uint64)
	c.contracts = make(map[TransactionID][]byte)
	c.contractGasBalances = make(map[TransactionID]uint64)
	c.contractVMs = make(map[AccountID]*VMState)

	c.VMCache = lru.NewLRU(4)
}

func (c *CollapseContext) ReadAccountsLen() uint64 {
	return c.accountLen
}

func (c *CollapseContext) WriteAccountsLen(size uint64) {
	c.accountLen = size
}

func (c *CollapseContext) ReadAccountNonce(id AccountID) (uint64, bool) {
	if nonce, ok := c.nonces[id]; ok {
		return nonce, true
	}

	nonce, exists := ReadAccountNonce(c.tree, id)
	if exists {
		c.nonces[id] = nonce
	}

	return nonce, exists
}

func (c *CollapseContext) ReadAccountBalance(id AccountID) (uint64, bool) {
	if balance, ok := c.balances[id]; ok {
		return balance, true
	}

	balance, exists := ReadAccountBalance(c.tree, id)
	if exists {
		c.balances[id] = balance
	}

	return balance, exists
}

func (c *CollapseContext) ReadAccountStake(id AccountID) (uint64, bool) {
	if stake, ok := c.stakes[id]; ok {
		return stake, true
	}

	stake, exists := ReadAccountStake(c.tree, id)
	if exists {
		c.stakes[id] = stake
	}

	return stake, exists
}

func (c *CollapseContext) ReadAccountReward(id AccountID) (uint64, bool) {
	if reward, ok := c.rewards[id]; ok {
		return reward, true
	}

	reward, exists := ReadAccountReward(c.tree, id)
	if exists {
		c.rewards[id] = reward
	}

	return reward, exists
}

func (c *CollapseContext) ReadAccountContractGasBalance(id TransactionID) (uint64, bool) {
	if gasBalance, ok := c.contractGasBalances[id]; ok {
		return gasBalance, true
	}

	gasBalance, exists := ReadAccountContractGasBalance(c.tree, id)
	if exists {
		c.contractGasBalances[id] = gasBalance
	}

	return gasBalance, exists
}

func (c *CollapseContext) ReadAccountContractCode(id TransactionID) ([]byte, bool) {
	if code, ok := c.contracts[id]; ok {
		return code, true
	}

	code, exists := ReadAccountContractCode(c.tree, id)
	if exists {
		c.contracts[id] = code
	}

	return code, exists
}

func (c *CollapseContext) GetContractState(id AccountID) (*VMState, bool) {
	vm, exists := c.contractVMs[id]
	return vm, exists
}

func (c *CollapseContext) addAccount(id AccountID) {
	if _, ok := c.accounts[id]; ok {
		return
	}

	c.accounts[id] = struct{}{}
	c.accountIDs = append(c.accountIDs, id)
}

func (c *CollapseContext) WriteAccountNonce(id AccountID, nonce uint64) {
	c.addAccount(id)
	c.nonces[id] = nonce
}

func (c *CollapseContext) WriteAccountBalance(id AccountID, balance uint64) {
	c.addAccount(id)
	c.balances[id] = balance
}

func (c *CollapseContext) WriteAccountStake(id AccountID, stake uint64) {
	c.addAccount(id)
	c.stakes[id] = stake
}

func (c *CollapseContext) WriteAccountReward(id AccountID, reward uint64) {
	c.addAccount(id)
	c.rewards[id] = reward
}

func (c *CollapseContext) WriteAccountContractGasBalance(id TransactionID, gasBalance uint64) {
	c.addAccount(id)
	c.contractGasBalances[id] = gasBalance
}

func (c *CollapseContext) WriteAccountContractCode(id TransactionID, code []byte) {
	c.addAccount(id)
	c.contracts[id] = code
}

func (c *CollapseContext) SetContractState(id AccountID, state *VMState) {
	c.addAccount(id)
	c.contractVMs[id] = state
}

func (c *CollapseContext) StoreRewardWithdrawalRequest(rw RewardWithdrawalRequest) {
	c.rewardWithdrawalRequests = append(c.rewardWithdrawalRequests, rw)
}

func (c *CollapseContext) processRewardWithdrawals(blockIndex uint64) {
	if blockIndex < uint64(sys.RewardWithdrawalsBlockLimit) {
		return
	}

	blockLimit := blockIndex - uint64(sys.RewardWithdrawalsBlockLimit)

	var leftovers []RewardWithdrawalRequest

	for i, rw := range c.rewardWithdrawalRequests {
		if c.rewardWithdrawalRequests[i].blockIndex > blockLimit {
			leftovers = append(leftovers, rw)
			continue
		}

		balance, _ := c.ReadAccountBalance(rw.account)
		c.WriteAccountBalance(rw.account, balance+rw.amount)
	}

	c.rewardWithdrawalRequests = leftovers
}

// Write the changes into the tree.
func (c *CollapseContext) Flush() error {
	if c.checksum != c.tree.Checksum() {
		return errors.Errorf("stale state, the state has been modified. "+
			"got merkle %x but expected %x.", c.tree.Checksum(), c.checksum)
	}

	WriteAccountsLen(c.tree, c.accountLen)

	for _, id := range c.accountIDs {
		if bal, ok := c.balances[id]; ok {
			WriteAccountBalance(c.tree, id, bal)
		}

		if stake, ok := c.stakes[id]; ok {
			WriteAccountStake(c.tree, id, stake)
		}

		if reward, ok := c.rewards[id]; ok {
			WriteAccountReward(c.tree, id, reward)
		}

		if nonce, ok := c.nonces[id]; ok {
			WriteAccountNonce(c.tree, id, nonce)
		}

		if gasBal, ok := c.contractGasBalances[id]; ok {
			WriteAccountContractGasBalance(c.tree, id, gasBal)
		}

		if code, ok := c.contracts[id]; ok {
			WriteAccountContractCode(c.tree, id, code)
		}

		if vm, ok := c.contractVMs[id]; ok {
			SaveContractMemorySnapshot(c.tree, id, vm.Memory)
			SaveContractGlobals(c.tree, id, vm.Globals)
		}
	}

	return nil
}

// Apply a transaction by writing the states into memory.
// After you've finished, you MUST call CollapseContext.Flush() to actually
// write the states into the tree.
func (c *CollapseContext) ApplyTransaction(block *Block, tx *Transaction) error {
	if err := applyTransaction(block, c, tx,
		&contractExecutorState{GasPayer: tx.Sender}); err != nil {

		return err
	}

	return nil
}
