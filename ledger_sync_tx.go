package wavelet

import (
	"context"
	"fmt"
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
)

func txSync(ledger *Ledger) func(ctx context.Context) error {
	queries := make(map[common.TransactionID]uint8)

	return func(ctx context.Context) error {
		set := make(map[common.TransactionID]struct{})

		ledger.graph.missingCond.L.Lock()
		for {
			ledger.mu.RLock()
			for id := range ledger.graph.missing {
				if _, queried := queries[id]; !queried {
					set[id] = struct{}{}
					queries[id] = 0
				}
			}
			ledger.mu.RUnlock()

			if len(set) > 0 {
				break
			}

			ledger.graph.missingCond.Wait()
		}
		ledger.graph.missingCond.L.Unlock()

		var missing []common.TransactionID

		for id := range set {
			missing = append(missing, id)
		}

		for id, count := range queries {
			if count < 3 { // Query a missing transaction at most 3 times.
				missing = append(missing, id)
				count++
			} else {
				delete(queries, id)
			}
		}

		fmt.Println("QUERYING", len(missing), "TRANSACTIONS")

		return downloadMissingTransactions(ctx, ledger, missing, queries)
	}
}

func downloadMissingTransactions(ctx context.Context, ledger *Ledger, missing []common.TransactionID, queries map[common.TransactionID]uint8) error {
	evt := EventDownloadTX{
		IDs:    missing,
		Result: make(chan []Transaction, 1),
		Error:  make(chan error, 1),
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
			delete(queries, tx.ID)
		}
	}

	return nil
}
