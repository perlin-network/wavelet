package wavelet

import (
	"context"
	"fmt"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
)

type transition func(ctx context.Context, ledger *Ledger) (transition, error)

func stateSync(ledger *Ledger) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		initial := checkIfOutOfSync

		for state, err := initial(ctx, ledger); state != nil; state, err = state(ctx, ledger) {
			if err != nil {
				return err
			}
		}

		return nil
	}
}

func checkIfOutOfSync(ctx context.Context, ledger *Ledger) (transition, error) {
	snapshot := ledger.accounts.snapshot()

	evt := EventOutOfSyncCheck{
		Result: make(chan []VoteOutOfSync, 1),
		Error:  make(chan error, 1),
	}

	select {
	case <-ctx.Done():
		return nil, nil
	case ledger.outOfSyncOut <- evt:
	}

	var votes []VoteOutOfSync

	select {
	case <-ctx.Done():
		return nil, nil
	case err := <-evt.Error:
		return nil, errors.Wrap(ErrNonePreferred, err.Error())
	case votes = <-evt.Result:
	}

	if len(votes) == 0 {
		return nil, nil
	}

	rounds := make(map[common.RoundID]Round)
	accounts := make(map[common.AccountID]struct{}, len(votes))

	for _, vote := range votes {
		if vote.Round.ID != common.ZeroRoundID && vote.Round.Root.ID != common.ZeroTransactionID {
			rounds[vote.Round.ID] = vote.Round
		}

		accounts[vote.Voter] = struct{}{}
	}

	var elected *Round
	counts := make(map[common.RoundID]float64)

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	weights := computeStakeDistribution(snapshot, accounts)

	for _, vote := range votes {
		if vote.Round.Root.ID != common.ZeroTransactionID {
			counts[vote.Round.ID] += weights[vote.Voter]

			if counts[vote.Round.ID] >= sys.SnowballSyncAlpha {
				elected = &vote.Round
				break
			}
		}
	}

	ledger.resolver.Tick(elected)

	if ledger.resolver.Decided() {
		proposedRound := ledger.resolver.Preferred()

		// The round number we came to consensus to being the latest within the network
		// is less than or equal to ours. Go back to square one.

		if ledger.round-1 >= proposedRound.Index {
			ledger.resolver.Reset()
			return nil, ErrNonePreferred
		}

		fmt.Printf("We are out of sync; our current round is %d, and our peers proposed round is %d.\n", ledger.round-1, proposedRound.Index)

		return downloadStateInChunks, ErrNonePreferred
	}

	return nil, nil
}

func downloadStateInChunks(ctx context.Context, ledger *Ledger) (transition, error) {
	return nil, nil
}
