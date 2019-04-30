package wavelet

import (
	"context"
	"github.com/perlin-network/wavelet/log"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

func recv(ledger *Ledger) func(ctx context.Context) error {
	diffs := newLRU(1024) // In total will take up 1024 * 4MB.

	return func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return nil
		case evt := <-ledger.gossipIn:
			err := ledger.addTransaction(evt.TX)

			evt.Vote <- err
			return err
		case evt := <-ledger.queryIn:
			r := evt.Round

			f := func() (*Round, error) {
				ledger.mu.RLock()
				defer ledger.mu.RUnlock()

				if r.Index < ledger.round { // Respond with the round we decided beforehand.
					round, available := ledger.rounds[r.Index]

					if !available {
						return nil, errors.Errorf("got requested with round %d, but do not have it available", r.Index)
					}

					return &round, nil
				}

				return nil, nil
			}

			round, err := f()

			if err != nil {
				evt.Error <- err
				return nil
			}

			if round != nil {
				evt.Response <- round
				return nil
			}

			if err := ledger.addTransaction(r.Root); err != nil { // Add the root in the round to our graph.
				evt.Error <- err
				return nil
			}

			evt.Response <- ledger.snowball.Preferred() // Send back our preferred round info, if we have any.

			return nil
		case evt := <-ledger.outOfSyncIn:
			ledger.mu.RLock()
			defer ledger.mu.RUnlock()

			if round, exists := ledger.rounds[ledger.round-1]; exists {
				evt.Response <- &round
			} else {
				evt.Response <- nil
			}

			return nil
		case evt := <-ledger.syncInitIn:
			ledger.mu.RLock()
			data := SyncInitMetadata{RoundID: ledger.round}
			ledger.mu.RUnlock()

			diff := ledger.accounts.snapshot().DumpDiff(evt.RoundID)

			for i := 0; i < len(diff); i += SyncChunkSize {
				end := i + SyncChunkSize

				if end > len(diff) {
					end = len(diff)
				}

				hash := blake2b.Sum256(diff[i:end])

				diffs.put(hash, diff[i:end])
				data.ChunkHashes = append(data.ChunkHashes, hash)
			}

			evt.Response <- data

			return nil
		case evt := <-ledger.syncDiffIn:
			if chunk, found := diffs.load(evt.ChunkHash); found {
				chunk := chunk.([]byte)

				providedHash := blake2b.Sum256(chunk)

				logger := log.Sync("provide_chunk")
				logger.Info().
					Hex("requested_hash", evt.ChunkHash[:]).
					Hex("provided_hash", providedHash[:]).
					Msg("Responded to sync chunk request.")

				evt.Response <- chunk
			} else {
				evt.Response <- nil
			}

			return nil
		case evt := <-ledger.downloadTxIn:
			var txs []Transaction

			for _, id := range evt.IDs {
				if tx := ledger.graph.GetTransaction(id); tx != nil {
					txs = append(txs, *tx)
				}
			}

			evt.Response <- txs
			return nil
		case evt := <-ledger.forwardTxIn:
			return ledger.addTransaction(evt.TX)
		}
	}
}
