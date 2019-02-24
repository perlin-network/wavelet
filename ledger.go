package wavelet

import (
	"github.com/perlin-network/wavelet/store"
)

type Ledger struct {
	accounts

	view graph
	kv   store.KV
}

func NewLedger(kv store.KV, genesis *Transaction) *Ledger {
	ledger := &Ledger{
		accounts: newAccounts(kv),

		view: newGraph(genesis),
		kv:   kv,
	}

	return ledger
}

// finalizeTransactions takes all transactions recorded in the graph view so far, and
// applies them one by one.
func (l *Ledger) finalizeTransactions() {
	//visited := make(map[[blake2b.Size256]byte]struct{})
	//queue := queue.New()
	//
	//queue.PushBack(l.view.root)
	//
	//for queue.Len() > 0 {
	//	_ = queue.PopFront().(*Transaction)
	//
	//	// TODO(kenta): apply transactions one by one to the current graph view
	//}
}
