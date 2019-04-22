package wavelet

import (
	"context"
	"fmt"
	"github.com/dgryski/go-xxh3"
	"github.com/pkg/errors"
)

func txSync(ledger *Ledger) func(ctx context.Context) error {
	queries := make(map[uint64]uint8)

	return func(ctx context.Context) error {
		set := make(map[uint64]struct{})

		ledger.graph.missingCond.L.Lock()
		for {
			for id := range ledger.graph.missing {
				checksum := xxh3.XXH3_64bits(id[:])

				if _, queried := queries[checksum]; !queried {
					set[checksum] = struct{}{}
					queries[checksum] = 0
				}
			}

			if len(set) > 0 {
				break
			}

			ledger.graph.missingCond.Wait()
		}
		ledger.graph.missingCond.L.Unlock()

		var missing []uint64

		for checksum := range set {
			missing = append(missing, checksum)
		}

		for checksum, count := range queries {
			if count < 3 { // Query a missing transaction at most 3 times.
				missing = append(missing, checksum)
				count++
			} else {
				delete(queries, checksum)
			}
		}

		fmt.Println("QUERYING", len(missing), "TRANSACTIONS")

		return downloadMissingTransactions(ctx, ledger, missing, queries)
	}
}

func downloadMissingTransactions(ctx context.Context, ledger *Ledger, missing []uint64, queries map[uint64]uint8) error {
	evt := EventDownloadTX{
		Checksums: missing,
		Result:    make(chan []Transaction, 1),
		Error:     make(chan error, 1),
	}

	select {
	case <-ctx.Done():
		return nil
	case ledger.downloadTxOut <- evt:
	}

	var txs []Transaction

	select {
	case <-ctx.Done():
		return nil
	case err := <-evt.Error:
		return errors.Wrap(err, "failed to download missing transactions")
	case txs = <-evt.Result:
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	var terr error
	for _, tx := range txs {
		if err := ledger.addTransaction(tx); err != nil {
			if terr == nil {
				terr = err
			} else {
				terr = errors.Wrap(err, terr.Error())
			}
		} else {
			delete(queries, tx.Checksum)
		}
	}

	return nil
}
