package wavelet

import (
	"github.com/google/btree"
	"golang.org/x/crypto/blake2b"
	"math/big"
	"sync"
)

var _ btree.Item = (*MempoolItem)(nil)

type MempoolItem struct {
	index *big.Int
	id    [blake2b.Size256]byte
}

func (m MempoolItem) Less(than btree.Item) bool {
	return m.index.Cmp(than.(MempoolItem).index) < 0
}

type Mempool struct {
	transactions map[[blake2b.Size256]byte]*Transaction
	mempool      *btree.BTree
	lock         sync.RWMutex
}

func NewMempool() *Mempool {
	return &Mempool{
		transactions: make(map[[blake2b.Size256]byte]*Transaction),
		mempool:      btree.New(32),
	}
}

func (m *Mempool) Add(tx Transaction, blockID [blake2b.Size256]byte) error {
	m.lock.Lock()

	if _, exists := m.transactions[tx.ID]; exists {
		m.lock.Unlock()
		return ErrAlreadyExists
	} else {
		m.transactions[tx.ID] = &tx

		item := MempoolItem{
			index: tx.ComputeIndex(blockID),
			id:    tx.ID,
		}
		m.mempool.ReplaceOrInsert(item)
	}

	m.lock.Unlock()

	return nil
}

func (m *Mempool) Get(txIDs []TransactionID) []*Transaction {
	var txs = make([]*Transaction, 0, len(txIDs))

	m.lock.RLock()

	for _, id := range txIDs {
		tx := m.transactions[id]
		// TODO if a transaction is missing
		if tx != nil {
			txs = append(txs, tx)
		}
	}

	m.lock.RUnlock()

	return txs
}
