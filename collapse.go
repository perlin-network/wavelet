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
	"github.com/perlin-network/wavelet/sys"
	queue2 "github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
)

func processRewardWithdrawals(round uint64, snapshot *avl.Tree) {
	rws := GetRewardWithdrawalRequests(snapshot, round-uint64(sys.RewardWithdrawalsRoundLimit))

	for _, rw := range rws {
		balance, _ := ReadAccountBalance(snapshot, rw.account)
		WriteAccountBalance(snapshot, rw.account, balance+rw.amount)

		snapshot.Delete(rw.Key())
	}
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

	ctx := NewCollapseContext()

	var (
		totalStake uint64
		totalFee   uint64
	)

	stakes := make(map[AccountID]uint64)

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

		if hex.EncodeToString(popped.Creator[:]) != sys.FaucetAddress {
			fee := popped.Fee()
			creatorBalance, _ := ReadAccountBalance(res.snapshot, popped.Creator)
			if creatorBalance < fee {
				res.rejected = append(res.rejected, popped)
				res.rejectedErrors = append(
					res.rejectedErrors,
					errors.Errorf("stake: creator %x does not have enough PERLs to pay transaction fees (comprised of %d PERLs)", popped.Creator, fee),
				)
				res.rejectedCount += popped.LogicalUnits()

				continue
			}

			WriteAccountBalance(res.snapshot, popped.Creator, creatorBalance-fee)
			totalFee += fee

			stake := uint64(0)
			if s, _ := ReadAccountStake(res.snapshot, popped.Sender); s > sys.MinimumStake {
				stake = s
			}

			if stake >= sys.MinimumStake {
				if _, ok := stakes[popped.Sender]; !ok {
					stakes[popped.Sender] = stake
				} else {
					stakes[popped.Sender] += stake
				}
				totalStake += stake
			}
		}

		if err := ApplyTransaction(current, res.snapshot, popped, ctx); err != nil {
			res.rejected = append(res.rejected, popped)
			res.rejectedErrors = append(res.rejectedErrors, err)
			res.rejectedCount += popped.LogicalUnits()

			logger := log.Node()
			logger.Error().Err(err).Msg("error applying transaction")

			continue
		}

		// Update statistics.

		res.applied = append(res.applied, popped)
		res.appliedCount += popped.LogicalUnits()
	}

	if totalStake > 0 {
		for sender, stake := range stakes {
			rewardeeBalance, _ := ReadAccountReward(res.snapshot, sender)

			reward := float64(totalFee) * (float64(stake) / float64(totalStake))

			WriteAccountReward(res.snapshot, sender, rewardeeBalance+uint64(reward))
		}
	}

	startDepth, endDepth := start.Depth+1, end.Depth

	for _, tx := range g.GetTransactionsByDepth(&startDepth, &endDepth) {
		res.ignoredCount += tx.LogicalUnits()
	}

	res.ignoredCount -= res.appliedCount + res.rejectedCount

	if round >= uint64(sys.RewardWithdrawalsRoundLimit) {
		processRewardWithdrawals(round, res.snapshot)
	}

	ctx.Flush(res.snapshot)

	return res, nil
}
