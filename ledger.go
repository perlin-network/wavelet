package wavelet

import (
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

type Ledger struct {
	accounts

	view graph
	kv   store.KV

	processors map[byte]TransactionProcessor
}

func NewLedger(kv store.KV, genesis *Transaction) *Ledger {
	ledger := &Ledger{
		accounts: newAccounts(kv),

		view: newGraph(genesis),
		kv:   kv,
	}

	return ledger
}

func (l *Ledger) RegisterProcessor(tag byte, processor TransactionProcessor) {
	l.processors[tag] = processor
}

// finalizeTransactions takes all transactions recorded in the graph view so far, and
// applies all valid ones to a snapshot of all accounts stored in the ledger.
//
// It returns an updated accounts snapshot after applying all finalized transactions.
func (l *Ledger) finalizeTransactions() accounts {
	snapshot := l.accounts.snapshotAccounts()

	visited := make(map[[blake2b.Size256]byte]struct{})
	queue := queue.New()

	queue.PushBack(l.view.root)

	for queue.Len() > 0 {
		popped := queue.PopFront().(*Transaction)

		for _, childrenID := range popped.children {
			if _, seen := visited[childrenID]; !seen {
				queue.PushBack(l.view.transactions[childrenID])
			}
		}

		visited[popped.ID] = struct{}{}

		// If any errors occur while applying our transaction to our accounts
		// snapshot, silently log it and continue applying other transactions.
		if err := l.applyTransaction(snapshot, popped); err != nil {
			log.Warn().Err(err).Msg("Got an error while applying transactions.")
		}
	}

	return snapshot
}

func (l *Ledger) applyTransaction(accounts accounts, tx *Transaction) error {
	if !accounts.snapshot {
		return errors.New("wavelet: to keep things safe, pass in an accounts instance that is a snapshot")
	}

	processor, exists := l.processors[tx.Tag]
	if !exists {
		return errors.Errorf("wavelet: transaction processor not registered for tag %d", tx.Tag)
	}

	ctx := newTransactionContext(accounts, tx)

	err := ctx.Apply(processor)
	if err != nil {
		return errors.Wrap(err, "wavelet: could not apply transaction")
	}

	return nil
}
