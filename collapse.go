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

func rewardValidators(g *Graph, snapshot *avl.Tree, root *Transaction, tx *Transaction, logging bool) error {
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

func collapseTransactions(g *Graph, accounts *Accounts, round uint64, latestRound *Round, root *Transaction, end *Transaction, logging bool) (*collapseResults, error) {
	nonceSeen := make(map[[SizeAccountID + 8]byte]struct{})

	res := &collapseResults{snapshot: accounts.Snapshot()}
	res.snapshot.SetViewID(round)

	visited := map[TransactionID]struct{}{root.ID: {}}

	queue := queue2.New()
	queue.PushBack(end)

	order := queue2.New()

	for queue.Len() > 0 {
		popped := queue.PopFront().(*Transaction)

		if popped.Depth <= root.Depth {
			continue
		}

		if popped.Round != round {
			continue
		}

		// Check that the nonce is used only once per account.
		{
			var key [SizeAccountID + 8]byte
			copy(key[:SizeAccountID], popped.Creator[:])
			binary.LittleEndian.PutUint64(key[SizeAccountID:], popped.Nonce)
			if _, seen := nonceSeen[key]; seen {
				continue
			}
			nonceSeen[key] = struct{}{}
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
			if err := rewardValidators(g, res.snapshot, root, popped, logging); err != nil {
				res.rejected = append(res.rejected, popped)
				res.rejectedErrors = append(res.rejectedErrors, err)
				res.rejectedCount += popped.LogicalUnits()

				continue
			}
		}

		if err := ApplyTransaction(latestRound, res.snapshot, popped); err != nil {
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

	startDepth, endDepth := root.Depth+1, end.Depth

	for _, tx := range g.GetTransactionsByDepth(&startDepth, &endDepth) {
		res.ignoredCount += tx.LogicalUnits()
	}

	res.ignoredCount -= res.appliedCount + res.rejectedCount

	if round >= uint64(sys.RewardWithdrawalsRoundLimit) {
		processRewardWithdrawals(round, res.snapshot)
	}

	return res, nil
}
