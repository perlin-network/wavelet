package wavelet

import (
	"io"
	"math/big"
	"sync"

	"github.com/google/btree"
	"github.com/perlin-network/wavelet/conf"
	"github.com/willf/bloom"
)

var _ btree.Item = (*mempoolItem)(nil)

type mempoolItem struct {
	index *big.Int
	id    TransactionID
}

func (m mempoolItem) Less(than btree.Item) bool {
	return m.index.Cmp(than.(mempoolItem).index) < 0
}

type Mempool struct {
	lock sync.RWMutex

	transactions map[TransactionID]*Transaction
	index        *btree.BTree
	filter       *bloom.BloomFilter
}

func NewMempool() *Mempool {
	return &Mempool{
		transactions: make(map[TransactionID]*Transaction),
		index:        btree.New(32),
		filter:       bloom.New(conf.GetBloomFilterM(), conf.GetBloomFilterK()),
	}
}

// Len returns the total number of transactions the node is aware of.
// It is safe to call this function concurrently.
func (m *Mempool) Len() int {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return len(m.transactions)
}

// PendingLen returns the number of transactions that are pending in the mempool.
// It is safe to call this function concurrently.
func (m *Mempool) PendingLen() int {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.index.Len()
}

// Find attempts to search for and return a transaction by its ID in the node, or
// otherwise returns nil.
//
// It is safe to call this function concurrently.
func (m *Mempool) Find(id TransactionID) *Transaction {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.transactions[id]
}

// Add batch-adds a set of transactions to the mempool, and computes their indices in the mempool
// based on the ID of the latest block the node is aware of.
func (m *Mempool) Add(blockID BlockID, txs ...Transaction) {
	m.lock.Lock()

	for _, tx := range txs {
		if _, exists := m.transactions[tx.ID]; exists {
			// TODO return error ?
			continue
		}

		m.transactions[tx.ID] = &tx
		m.filter.Add(tx.ID[:])

		item := mempoolItem{
			index: tx.ComputeIndex(blockID),
			id:    tx.ID,
		}
		m.index.ReplaceOrInsert(item)
	}

	m.lock.Unlock()
}

// WriteTransactionIDs writes the marshaled bloom filter onto
// the specified io.Writer.
func (m *Mempool) WriteTransactionIDs(w io.Writer) (int64, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.filter.WriteTo(w)
}

// Ascend iterates through the mempool in ascending order.
// It stops iterating when the iterator function returns false.
func (m *Mempool) Ascend(iter func(tx Transaction) bool) {
	m.lock.RLock()
	m.index.Ascend(func(i btree.Item) bool {
		id := i.(mempoolItem).id
		return iter(*m.transactions[id])
	})
	m.lock.RUnlock()
}

// Ascend iterates through the mempool in ascending order, starting from
// index 0 up to maxIndex. It stops iterating when the iterator function
// returns false.
func (m *Mempool) AscendLessThan(maxIndex *big.Int, iter func(tx Transaction) bool) {
	m.lock.RLock()
	m.index.AscendLessThan(mempoolItem{index: maxIndex}, func(i btree.Item) bool {
		id := i.(mempoolItem).id
		return iter(*m.transactions[id])
	})
	m.lock.RUnlock()
}

// Prune removes the transactions inside the specified block from the mempool.
// It returns the number of transactions that are pruned.
func (m *Mempool) Prune(block Block) int {
	var pruned int

	m.lock.Lock()

	// Remove transactions from the mempool
	for _, txID := range block.Transactions {
		if _, ok := m.transactions[txID]; ok {
			delete(m.transactions, txID)
			m.index.Delete(mempoolItem{id: txID})
			pruned++
		}
	}

	// Rebuild transaction ids bloom filter
	m.filter.ClearAll()

	for id := range m.transactions {
		m.filter.Add(id[:])
	}

	m.lock.Unlock()

	return pruned
}
