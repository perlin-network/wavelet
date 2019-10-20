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

	// TODO: extract transactions and filter out of mempool
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

// Reshuffle reshuffles the mempool for the incoming block.
func (m *Mempool) Reshuffle(prevBlock Block, nextBlock Block) {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Remove transactions contained in the next block
	for _, id := range nextBlock.Transactions {
		tx := m.transactions[id]
		if tx == nil {
			continue
		}

		m.index.Delete(mempoolItem{index: tx.ComputeIndex(prevBlock.ID)})
	}

	// Reindex all transactions based on the new block
	var pruned uint64

	items := make([]mempoolItem, 0, m.index.Len())
	m.index.Ascend(func(i btree.Item) bool {
		item := i.(mempoolItem)
		tx := m.transactions[item.id]

		// Prune old tx
		if nextBlock.Index >= tx.Block+uint64(conf.GetPruningLimit()) {
			delete(m.transactions, item.id)
			pruned++
			return true
		}

		item.index = tx.ComputeIndex(nextBlock.ID)
		items = append(items, item)
		return true
	})

	m.index.Clear(false)

	for _, item := range items {
		m.index.ReplaceOrInsert(item)
	}

	// Rebuild filter if there is at least 1 pruned tx
	if pruned > 0 {
		m.filter.ClearAll()

		for id := range m.transactions {
			m.filter.Add(id[:])
		}
	}
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
func (m *Mempool) AscendPending(iter func(txID TransactionID) bool) {
	m.lock.RLock()
	m.index.Ascend(func(i btree.Item) bool {
		return iter(i.(mempoolItem).id)
	})
	m.lock.RUnlock()
}

// Iterate through the all the transactions in non deterministic order.
func (m *Mempool) Iter(iter func(tx Transaction) bool) {
	m.lock.RLock()
	for _, tx := range m.transactions {
		if !iter(*tx) {
			break
		}
	}
	m.lock.RUnlock()
}

// Ascend iterates through the mempool in ascending order, starting from
// index 0 up to maxIndex. It stops iterating when the iterator function
// returns false.
func (m *Mempool) AscendPendingLessThan(maxIndex *big.Int, iter func(txID TransactionID) bool) {
	m.lock.RLock()
	m.index.AscendLessThan(mempoolItem{index: maxIndex}, func(i btree.Item) bool {
		return iter(i.(mempoolItem).id)
	})
	m.lock.RUnlock()
}
