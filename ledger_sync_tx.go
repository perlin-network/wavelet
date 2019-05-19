package wavelet

import (
	"context"
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
)

func txSync(ledger *Ledger) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		ledger.syncingCond.L.Lock()
		for ledger.syncing {
			ledger.syncingCond.Wait()
		}
		ledger.syncingCond.L.Unlock()

		ledger.gossipQueryWG.Add(1)
		defer ledger.gossipQueryWG.Done()

		missing := ledger.graph.MissingTransactions()

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
