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
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/lru"
	"github.com/perlin-network/wavelet/sys"
	queue2 "github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

// rewardValidators deducts a transaction fee from a transactions creator, and transfers the fee
// to a rewardee which is determined by a validator reward scheme given the selected ancestry
// of the transaction.
//
// If no rewardee is selected, then the transaction fee is simply burned. A reference to
// a transaction is expected when calling this function to prevent any additional requirements
// of looking up said transaction within the graph.
func rewardValidators(g *Graph, ctx *CollapseContext, tx *Transaction, logging bool) error {
	fee := sys.TransactionFeeAmount

	creatorBalance, _ := ctx.ReadAccountBalance(tx.Creator)

	if creatorBalance < fee {
		return errors.Errorf("stake: creator %x does not have enough PERLs to pay transaction fees (comprised of %d PERLs)", tx.Creator, fee)
	}

	ctx.WriteAccountBalance(tx.Creator, creatorBalance-fee)

	var candidates []*Transaction
	var stakes []uint64
	var totalStake uint64

	visited := make(map[TransactionID]struct{})

	queue := AcquireQueue()
	defer ReleaseQueue(queue)

	for _, parentID := range tx.ParentIDs {
		if parent := g.FindTransaction(parentID); parent != nil {
			queue.PushBack(parent)
		}

		visited[parentID] = struct{}{}
	}

	hasher, _ := blake2b.New256(nil)

	var depthCounter uint64
	var lastDepth = tx.Depth

	for queue.Len() > 0 {
		popped := queue.PopFront().(*Transaction)

		if popped.Depth != lastDepth {
			lastDepth = popped.Depth
			depthCounter++
		}

		// If we exceed the max eligible depth we search for candidate
		// validators to reward from, stop traversing.

		if depthCounter >= conf.GetMaxDepthDiff() {
			break
		}

		// Filter for all ancestral transactions not from the same sender,
		// and within the desired graph depth.

		if popped.Sender != tx.Sender {
			stake, _ := ctx.ReadAccountStake(popped.Sender)

			if stake > sys.MinimumStake {
				candidates = append(candidates, popped)
				stakes = append(stakes, stake)

				totalStake += stake

				// Record entropy source.
				if _, err := hasher.Write(popped.ID[:]); err != nil {
					return errors.Wrap(err, "stake: failed to hash transaction ID for entropy source")
				}
			}
		}

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; !seen {
				if parent := g.FindTransaction(parentID); parent != nil {
					queue.PushBack(parent)
				}

				visited[parentID] = struct{}{}
			}
		}
	}

	// If there are no eligible rewardee candidates, do not reward anyone.

	if len(candidates) == 0 || len(stakes) == 0 || totalStake == 0 {
		return nil
	}

	entropy := hasher.Sum(nil)
	acc, threshold := float64(0), float64(binary.LittleEndian.Uint64(entropy)%uint64(0xffff))/float64(0xffff)

	var rewardee *Transaction

	// Model a weighted uniform distribution by a random variable X, and select
	// whichever validator has a weight X ≥ X' as a reward recipient.

	for i, tx := range candidates {
		acc += float64(stakes[i]) / float64(totalStake)

		if acc >= threshold {
			rewardee = tx
			break
		}
	}

	// If there is no selected transaction that deserves a reward, give the
	// reward to the last reward candidate.

	if rewardee == nil {
		rewardee = candidates[len(candidates)-1]
	}

	rewardeeBalance, _ := ctx.ReadAccountReward(rewardee.Sender)
	ctx.WriteAccountReward(rewardee.Sender, rewardeeBalance+fee)

	if logging {
		logger := log.Stake("reward_validator")
		logger.Info().
			Hex("creator", tx.Creator[:]).
			Hex("recipient", rewardee.Sender[:]).
			Hex("creator_tx_id", tx.ID[:]).
			Hex("rewardee_tx_id", rewardee.ID[:]).
			Hex("entropy", entropy).
			Float64("acc", acc).
			Float64("threshold", threshold).Msg("Rewarded validator.")
	}

	return nil
}

func collapseTransactions(g *Graph, accounts *Accounts, round uint64, current *Round, start, end Transaction, logging bool) (*collapseResults, error) {
	res := &collapseResults{snapshot: accounts.Snapshot()}
	res.snapshot.SetViewID(round)

	visited := map[TransactionID]struct{}{start.ID: {}}

	queue := queue2.New()
	queue.PushBack(&end)

	order := queue2.New()

	for queue.Len() > 0 {
		popped := queue.PopFront().(*Transaction)

		if popped.Depth <= start.Depth {
			continue
		}

		order.PushBack(popped)

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; seen {
				continue
			}

			visited[parentID] = struct{}{}

			parent := g.FindTransaction(parentID)

			if parent == nil {
				g.MarkTransactionAsMissing(parentID, popped.Depth)
				return nil, errors.Errorf("missing ancestor %x to correctly collapse down ledger state from critical transaction %x", parentID, end.ID)
			}

			queue.PushBack(parent)
		}
	}

	res.applied = make([]*Transaction, 0, order.Len())
	res.rejected = make([]*Transaction, 0, order.Len())
	res.rejectedErrors = make([]error, 0, order.Len())

	ctx := NewCollapseContext(res.snapshot)

	// Apply transactions in reverse order from the end of the round
	// all the way down to the beginning of the round.

	for order.Len() > 0 {
		popped := order.PopBack().(*Transaction)

		// Update nonce.

		nonce, exists := ctx.ReadAccountNonce(popped.Creator)
		if !exists {
			ctx.WriteAccountsLen(ctx.ReadAccountsLen() + 1)
		}
		ctx.WriteAccountNonce(popped.Creator, nonce+1)

		// FIXME(kenta): FOR TESTNET ONLY. FAUCET DOES NOT GET ANY PERLs DEDUCTED.
		if hex.EncodeToString(popped.Creator[:]) != sys.FaucetAddress {
			if err := rewardValidators(g, ctx, popped, logging); err != nil {
				res.rejected = append(res.rejected, popped)
				res.rejectedErrors = append(res.rejectedErrors, err)
				res.rejectedCount += popped.LogicalUnits()

				continue
			}
		}

		if err := ctx.ApplyTransaction(current, popped); err != nil {
			res.rejected = append(res.rejected, popped)
			res.rejectedErrors = append(res.rejectedErrors, err)
			res.rejectedCount += popped.LogicalUnits()

			fmt.Println("error applying transaction", err)

			continue
		}

		// Update statistics.

		res.applied = append(res.applied, popped)
		res.appliedCount += popped.LogicalUnits()
	}

	startDepth, endDepth := start.Depth+1, end.Depth

	for _, tx := range g.GetTransactionsByDepth(&startDepth, &endDepth) {
		res.ignoredCount += tx.LogicalUnits()
	}

	res.ignoredCount -= res.appliedCount + res.rejectedCount

	ctx.processRewardWithdrawals(round)

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

func (c *CollapseContext) processRewardWithdrawals(round uint64) {
	if round < uint64(sys.RewardWithdrawalsRoundLimit) {
		return
	}

	roundLimit := round - uint64(sys.RewardWithdrawalsRoundLimit)

	var leftovers []RewardWithdrawalRequest

	for i, rw := range c.rewardWithdrawalRequests {
		if c.rewardWithdrawalRequests[i].round > roundLimit {
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
		return errors.Errorf("stale state, the state has been modified. got merkle %x but expected %x.", c.tree.Checksum(), c.checksum)
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
// After you've finished, you MUST call CollapseContext.Flush() to actually write the states into the tree.
func (c *CollapseContext) ApplyTransaction(round *Round, tx *Transaction) error {
	if err := applyTransaction(round, c, tx, &contractExecutorState{
		GasPayer: tx.Creator,
	}); err != nil {
		return err
	}

	return nil
}
