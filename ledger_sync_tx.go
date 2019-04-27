package wavelet

import (
	"context"
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
)

func txSync(ledger *Ledger) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		var missing []common.TransactionID

		ledger.mu.RLock()
		for id := range ledger.graph.missing {
			missing = append(missing, id)
		}
		ledger.mu.RUnlock()

		if len(missing) == 0 {
			return ErrNonePreferred
		}

		return downloadMissingTransactions(ctx, ledger, missing)
	}
}

func downloadMissingTransactions(ctx context.Context, ledger *Ledger, missing []common.TransactionID) error {
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
		}
	}

	return nil
}
