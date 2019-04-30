package wavelet

import (
	"context"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"sort"
	"time"
)

// query atomically maintains the ledgers graph, and divides the graph from the bottom up into rounds.
//
// It maintains the ledgers Snowball instance while dividing up rounds, to see what the network believes
// is the preferred critical transaction selected to finalize the current ledgers round, and also be the
// root transaction for the next new round.
//
// It should be called repetitively as fast as possible in an infinite for loop, in a separate goroutine
// away from any other goroutines associated to the ledger.
func query(ledger *Ledger) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		ledger.syncingCond.L.Lock()
		for ledger.syncing {
			ledger.syncingCond.Wait()
		}
		ledger.syncingCond.L.Unlock()

		ledger.mu.RLock()
		nextRound := ledger.round
		lastRound := ledger.rounds[ledger.round-1]
		ledger.mu.RUnlock()

		if err := findCriticalTransactionToPrefer(ledger, lastRound.Root); err != nil {
			return err
		}

		snapshot := ledger.accounts.snapshot()

		evt := EventQuery{
			Round:  ledger.snowball.Preferred(),
			Result: make(chan []VoteQuery, 1),
			Error:  make(chan error, 1),
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(1 * time.Second):
			return errors.New("query queue is full")
		case ledger.queryOut <- evt:
		}

		var votes []VoteQuery

		select {
		case <-ctx.Done():
			return nil
		case err := <-evt.Error:
			return errors.Wrap(err, "query got event error")
		case votes = <-evt.Result:
		}

		if len(votes) == 0 {
			return nil
		}

		rounds := make(map[common.RoundID]Round)
		accounts := make(map[common.AccountID]struct{}, len(votes))

		for _, vote := range votes {
			if vote.Preferred.Index == nextRound && vote.Preferred.ID != common.ZeroRoundID && vote.Preferred.Root.ID != common.ZeroTransactionID {
				rounds[vote.Preferred.ID] = vote.Preferred
			}

			accounts[vote.Voter] = struct{}{}
		}

		var elected *Round
		counts := make(map[common.RoundID]float64)

		weights := computeStakeDistribution(snapshot, accounts)

		for _, vote := range votes {
			if vote.Preferred.Index == nextRound && vote.Preferred.ID != common.ZeroRoundID && vote.Preferred.Root.ID != common.ZeroRoundID {
				counts[vote.Preferred.ID] += weights[vote.Voter]

				if counts[vote.Preferred.ID] >= sys.SnowballAlpha {
					elected = &vote.Preferred
					break
				}
			}
		}

		if elected == nil {
			return nil
		}

		ledger.mu.Lock()
		defer ledger.mu.Unlock()

		ledger.snowball.Tick(elected)

		if ledger.snowball.Decided() {
			newRound := ledger.snowball.Preferred()
			newRoot := &newRound.Root

			state, err := ledger.collapseTransactions(newRound.Index, newRoot, true)

			if err != nil {
				return errors.Wrap(err, "got an error finalizing a round")
			}

			if state.Checksum() != newRound.Merkle {
				return errors.Errorf("expected finalized rounds merkle root to be %x, but got %x", newRound.Merkle, state.Checksum())
			}

			if err = ledger.accounts.commit(state); err != nil {
				return errors.Wrap(err, "failed to commit collapsed state to our database")
			}

			ledger.snowball.Reset()
			ledger.graph.Reset(newRound)

			ledger.prune(newRound)

			logger := log.Consensus("round_end")
			logger.Info().
				Uint64("num_tx", ledger.getNumTransactions(ledger.round-1)).
				Uint64("old_round", lastRound.Index).
				Uint64("new_round", newRound.Index).
				Uint8("old_difficulty", lastRound.Root.ExpectedDifficulty(byte(sys.MinDifficulty))).
				Uint8("new_difficulty", newRound.Root.ExpectedDifficulty(byte(sys.MinDifficulty))).
				Hex("new_root", newRound.Root.ID[:]).
				Hex("old_root", lastRound.Root.ID[:]).
				Hex("new_merkle_root", newRound.Merkle[:]).
				Hex("old_merkle_root", lastRound.Merkle[:]).
				Msg("Finalized consensus round, and initialized a new round.")

			ledger.rounds[ledger.round] = *newRound
			ledger.round++
		}

		return nil
	}
}

// findCriticalTransactionPrefer finds a critical transaction to initially prefer first, if Snowball
// does not prefer any transaction just yet. It returns an error if no suitable critical transaction
// may be found in the current round.
func findCriticalTransactionToPrefer(ledger *Ledger, oldRoot Transaction) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	if ledger.snowball.Preferred() != nil {
		return nil
	}

	difficulty := oldRoot.ExpectedDifficulty(byte(sys.MinDifficulty))

	// Find all eligible critical transactions for the current round.
	eligible := ledger.graph.FindEligibleCriticals(oldRoot.Depth, difficulty)

	if len(eligible) == 0 { // If there are no critical transactions for the round yet, discontinue.
		return ErrNonePreferred
	}

	// Sort critical transactions by their depth, and pick the critical transaction
	// with the smallest depth as the nodes initial preferred transaction.
	//
	// The final selected critical transaction might change after a couple of
	// rounds with Snowball.

	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].Depth < eligible[j].Depth
	})

	proposed := eligible[0]

	state, err := ledger.collapseTransactions(ledger.round, proposed, false)

	if err != nil {
		return errors.Wrap(err, "could not collapse first critical transaction we could find")
	}

	initial := NewRound(ledger.round, state.Checksum(), *proposed)
	ledger.snowball.Prefer(&initial)

	return nil
}
