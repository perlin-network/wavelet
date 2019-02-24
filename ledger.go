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
