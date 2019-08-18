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
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	queue2 "github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

func processRewardWithdrawals(round uint64, snapshot *avl.Tree) {
	rws := GetRewardWithdrawalRequests(snapshot, round-uint64(sys.RewardWithdrawalsRoundLimit))

	for _, rw := range rws {
		balance, _ := ReadAccountBalance(snapshot, rw.account)
		WriteAccountBalance(snapshot, rw.account, balance+rw.amount)

		snapshot.Delete(rw.Key())
	}
}

// rewardValidators deducts a transaction fee from a transactions creator, and transfers the fee
// to a rewardee which is determined by a validator reward scheme given the selected ancestry
// of the transaction.
//
// If no rewardee is selected, then the transaction fee is simply burned. A reference to
// a transaction is expected when calling this function to prevent any additional requirements
// of looking up said transaction within the graph.
func rewardValidators(g *Graph, snapshot *avl.Tree, tx *Transaction, logging bool) error {
	fee := sys.TransactionFeeAmount

	creatorBalance, _ := ReadAccountBalance(snapshot, tx.Creator)

	if creatorBalance < fee {
		return errors.Errorf("stake: creator %x does not have enough PERLs to pay transaction fees (comprised of %d PERLs)", tx.Creator, fee)
	}

	WriteAccountBalance(snapshot, tx.Creator, creatorBalance-fee)

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

		if depthCounter >= sys.MaxDepthDiff {
			break
		}

		// Filter for all ancestral transactions not from the same sender,
		// and within the desired graph depth.

		if popped.Sender != tx.Sender {
			stake, _ := ReadAccountStake(snapshot, popped.Sender)

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

	rewardeeBalance, _ := ReadAccountReward(snapshot, rewardee.Sender)
	WriteAccountReward(snapshot, rewardee.Sender, rewardeeBalance+fee)

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

	applyCtx := &ApplyContext{
		Contracts: make(map[AccountID]*VMState),
	}

	// Apply transactions in reverse order from the end of the round
	// all the way down to the beginning of the round.

	for order.Len() > 0 {
		popped := order.PopBack().(*Transaction)

		// Update nonce.

		nonce, exists := ReadAccountNonce(res.snapshot, popped.Creator)
		if !exists {
			WriteAccountsLen(res.snapshot, ReadAccountsLen(res.snapshot)+1)
		}
		WriteAccountNonce(res.snapshot, popped.Creator, nonce+1)

		// FIXME(kenta): FOR TESTNET ONLY. FAUCET DOES NOT GET ANY PERLs DEDUCTED.
		if hex.EncodeToString(popped.Creator[:]) != sys.FaucetAddress {
			if err := rewardValidators(g, res.snapshot, popped, logging); err != nil {
				res.rejected = append(res.rejected, popped)
				res.rejectedErrors = append(res.rejectedErrors, err)
				res.rejectedCount += popped.LogicalUnits()

				continue
			}
		}

		if err := ApplyTransaction(current, res.snapshot, popped, applyCtx); err != nil {
			res.rejected = append(res.rejected, popped)
			res.rejectedErrors = append(res.rejectedErrors, err)
			res.rejectedCount += popped.LogicalUnits()

			fmt.Println(err)

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

	if round >= uint64(sys.RewardWithdrawalsRoundLimit) {
		processRewardWithdrawals(round, res.snapshot)
	}

	for id, state := range applyCtx.Contracts {
		SaveContractMemorySnapshot(res.snapshot, id, state.Memory)
		SaveContractGlobals(res.snapshot, id, state.Globals)
	}

	return res, nil
}
