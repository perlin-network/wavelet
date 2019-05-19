package wavelet

import (
	"context"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"time"
)

type transition func(ctx context.Context, ledger *Ledger) (transition, error)

func stateSync(ledger *Ledger) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		state, err := checkIfOutOfSync(ctx, ledger)

		for {
			if err != nil {
				return err
			}

			if state == nil {
				return nil
			}

			state, err = state(ctx, ledger)
		}
	}
}

func checkIfOutOfSync(ctx context.Context, ledger *Ledger) (transition, error) {
	ledger.mu.RLock()
	currentRound := ledger.round - 1
	ledger.mu.RUnlock()

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
		return nil, ErrNonePreferred
	}

	rounds := make(map[common.RoundID]Round)
	accounts := make(map[common.AccountID]struct{}, len(votes))

	var maxRoundIndex uint64

	for _, vote := range votes {
		if vote.Round.ID != common.ZeroRoundID && vote.Round.End.ID != common.ZeroTransactionID {
			if maxRoundIndex < vote.Round.Index {
				maxRoundIndex = vote.Round.Index
			}

			rounds[vote.Round.ID] = vote.Round
		}

		accounts[vote.Voter] = struct{}{}
	}

	if currentRound+sys.SyncRoundDifference >= maxRoundIndex {
		return nil, ErrNonePreferred
	}

	var elected *Round
	counts := make(map[common.RoundID]float64)

	weights := computeStakeDistribution(snapshot, accounts)

	for _, vote := range votes {
		if vote.Round.ID != common.ZeroRoundID && vote.Round.End.ID != common.ZeroTransactionID {
			counts[vote.Round.ID] += weights[vote.Voter]

			if counts[vote.Round.ID] >= sys.SnowballAlpha {
				elected = &vote.Round
				break
			}
		}
	}

	if elected == nil {
		return nil, ErrNonePreferred
	}

	ledger.resolver.Tick(elected)

	if ledger.resolver.Decided() {
		defer ledger.resolver.Reset()

		proposedRound := ledger.resolver.Preferred()

		// The round number we came to consensus to being the latest within the network
		// is less than or equal to ours. Go back to square one.

		if currentRound+sys.SyncRoundDifference >= proposedRound.Index {
			return nil, ErrNonePreferred
		}

		return downloadStateInChunks(proposedRound), nil
	}

	return nil, nil
}

func downloadStateInChunks(newRound *Round) transition {
	return func(ctx context.Context, ledger *Ledger) (t transition, e error) {
		logger := log.Sync("syncing")
		logger.Info().
			Uint64("current_round", ledger.round-1).
			Uint64("proposed_round", newRound.Index).
			Msg("Noticed that we are out of sync; downloading latest state tree from our peer(s).")

		ledger.syncingCond.L.Lock()
		ledger.syncing = true
		ledger.syncingCond.Broadcast()
		ledger.syncingCond.L.Unlock()

		ledger.gossipQueryWG.Wait()

		defer func() {
			ledger.syncingCond.L.Lock()
			ledger.syncing = false
			ledger.syncingCond.Broadcast()
			ledger.syncingCond.L.Unlock()
		}()

		evt := EventSyncInit{
			RoundID: ledger.round,
			Result:  make(chan []SyncInitMetadata, 1),
			Error:   make(chan error, 1),
		}

		select {
		case <-ctx.Done():
			return nil, nil
		case ledger.syncInitOut <- evt:
		}

		var votes []SyncInitMetadata

		select {
		case <-ctx.Done():
			return nil, nil
		case err := <-evt.Error:
			return nil, errors.Wrap(err, "failed to get sync init response")
		case v := <-evt.Result:
			votes = v
		}

		votesByViewID := make(map[uint64][]SyncInitMetadata)

		for _, vote := range votes {
			if vote.RoundID > 0 && len(vote.ChunkHashes) > 0 {
				votesByViewID[vote.RoundID] = append(votesByViewID[vote.RoundID], vote)
			}
		}

		var selected []SyncInitMetadata

		for _, v := range votesByViewID {
			if len(v) >= len(votes)*2/3 {
				selected = votes
				break
			}
		}

		if selected == nil { // There is no consensus on which round to sync towards; cancel the sync.
			return nil, errors.New("no consensus on which round to sync towards")
		}

		var sources []ChunkSource

		for i := 0; ; i++ {
			hashCount := make(map[[blake2b.Size256]byte][]*skademlia.ID)
			hashInRange := false

			for _, vote := range selected {
				if i >= len(vote.ChunkHashes) {
					continue
				}

				hashCount[vote.ChunkHashes[i]] = append(hashCount[vote.ChunkHashes[i]], vote.PeerID)
				hashInRange = true
			}

			if !hashInRange {
				break
			}

			consistent := false

			for hash, peers := range hashCount {
				if len(peers) >= len(selected)*2/3 && len(peers) > 0 {
					sources = append(sources, ChunkSource{Hash: hash, PeerIDs: peers})

					consistent = true
					break
				}
			}

			if !consistent {
				return nil, errors.New("chunk IDs are not consistent")
			}
		}

		evtc := EventSyncDiff{
			Sources: sources,
			Result:  make(chan [][]byte, 1),
			Error:   make(chan error, 1),
		}

		select {
		case <-ctx.Done():
			return nil, nil
		case <-time.After(1 * time.Second):
			return nil, errors.New("timed out while waiting for sync chunk queue to empty up")
		case ledger.syncDiffOut <- evtc:
		}

		var chunks [][]byte

		select {
		case <-ctx.Done():
			return nil, nil
		case err := <-evtc.Error:
			return nil, err
		case c := <-evtc.Result:
			chunks = c
		}

		var diff []byte

		for _, chunk := range chunks {
			diff = append(diff, chunk...)
		}

		snapshot := ledger.accounts.snapshot()

		if err := snapshot.ApplyDiff(diff); err != nil {
			return nil, errors.Wrap(err, "failed to apply diff to state")
		}

		if snapshot.Checksum() != newRound.Merkle {
			return nil, errors.Errorf("applying the diff yielded a merkle root of %x, but the root recorded a merkle root of %x", snapshot.Checksum(), newRound.Merkle)
		}

		if err := ledger.accounts.commit(snapshot); err != nil {
			return nil, errors.Wrap(err, "failed to commit collapsed state to our database")
		}

		ledger.mu.Lock()
		defer ledger.mu.Unlock()

		ledger.snowball.Reset()
		ledger.graph.Reset(newRound)

		ledger.prune(newRound)

		oldRound := ledger.rounds[ledger.round-1]

		// Sync successful.
		logger = log.Sync("apply")
		logger.Info().
			Int("num_chunks", len(chunks)).
			Uint64("old_round", oldRound.Index).
			Uint64("new_round", newRound.Index).
			Uint8("old_difficulty", oldRound.ExpectedDifficulty(sys.MinDifficulty, sys.DifficultyScaleFactor)).
			Uint8("new_difficulty", newRound.ExpectedDifficulty(sys.MinDifficulty, sys.DifficultyScaleFactor)).
			Hex("new_root", newRound.End.ID[:]).
			Hex("old_root", oldRound.End.ID[:]).
			Hex("new_merkle_root", newRound.Merkle[:]).
			Hex("old_merkle_root", oldRound.Merkle[:]).
			Msg("Successfully built a new state tree out of chunk(s) we have received from peers.")

		ledger.rounds[newRound.Index] = *newRound
		ledger.round = newRound.Index + 1

		if err := storeRound(ledger.kv, ledger.round, *newRound); err != nil {
			return nil, errors.Wrap(err, "failed to store round")
		}

		return nil, nil
	}
}
