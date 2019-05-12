package wavelet

import (
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"testing"
)

func benchCollapseTransactions(size int, b *testing.B) {
	keys, err := skademlia.NewKeys(1, 1)
	if err != nil {
		b.Fatal(err)
	}

	ledger := NewLedger(keys, store.NewInmem(), nil)

	var last *Transaction
	for i := 0; i < size; i++ {
		tx := NewTransaction(keys, sys.TagNop, nil)

		tx, err = ledger.attachSenderToTransaction(tx)
		if err != nil {
			b.Fatal(err)
		}

		if err := ledger.graph.AddTransaction(tx); err != nil {
			b.Fatal(err)
		}

		last = &tx
	}

	for i := 0; i < b.N; i++ {
		_, _, _, _, err := ledger.collapseTransactions(1, last, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCollapseTransactions100(b *testing.B) {benchCollapseTransactions(100, b)}
func BenchmarkCollapseTransactions1000(b *testing.B) {benchCollapseTransactions(1000, b)}
func BenchmarkCollapseTransactions5000(b *testing.B) {benchCollapseTransactions(5000, b)}