package wavelet

import (
	"io"
	"math/big"
	"sync"

	"github.com/google/btree"
	"github.com/perlin-network/wavelet/conf"
	"github.com/willf/bloom"
)

var _ btree.Item = (*MempoolItem)(nil)

type MempoolItem struct {
	index *big.Int
	id    TransactionID
}

func (m MempoolItem) Less(than btree.Item) bool {
	return m.index.Cmp(than.(MempoolItem).index) < 0
}

type Mempool struct {
	transactions   map[TransactionID]*Transaction
	mempool        *btree.BTree
	lock           sync.RWMutex
	transactionIDs *bloom.BloomFilter
}

func NewMempool() *Mempool {
	return &Mempool{
		transactions:   make(map[TransactionID]*Transaction),
		mempool:        btree.New(32),
		transactionIDs: bloom.New(conf.GetBloomFilterM(), conf.GetBloomFilterK()),
	}
}

func (m *Mempool) Add(blockID BlockID, txs ...Transaction) {
	m.lock.Lock()

	for _, tx := range txs {
		if _, exists := m.transactions[tx.ID]; exists {
			// TODO return error ?
			continue
		}

		m.transactions[tx.ID] = &tx
		m.transactionIDs.Add(tx.ID[:])

		item := MempoolItem{
			index: tx.ComputeIndex(blockID),
			id:    tx.ID,
		}
		m.mempool.ReplaceOrInsert(item)
	}

	m.lock.Unlock()
}

// TODO find a better name or a better way to implement this ?
func (m *Mempool) ReadLock(f func(transactions map[TransactionID]*Transaction)) {
	m.lock.RLock()

	f(m.transactions)

	m.lock.RUnlock()
}

func (m *Mempool) WriteBloomFilter(w io.Writer) (int64, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.transactionIDs.WriteTo(w)
}

func (m *Mempool) Ascend(iter func(tx Transaction) bool) {
	m.lock.RLock()
	m.mempool.Ascend(func(i btree.Item) bool {
		id := i.(MempoolItem).id
		return iter(*m.transactions[id])
	})
	m.lock.RUnlock()
}

func (m *Mempool) AscendLessThan(maxIndex *big.Int, iter func(tx Transaction) bool) {
	m.lock.RLock()
	m.mempool.AscendLessThan(MempoolItem{index: maxIndex}, func(i btree.Item) bool {
		id := i.(MempoolItem).id
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
			m.mempool.Delete(MempoolItem{id: txID})
			pruned++
		}
	}

	// Rebuild transaction ids bloom filter
	m.transactionIDs.ClearAll()

	for id := range m.transactions {
		m.transactionIDs.Add(id[:])
	}

	m.lock.Unlock()

	return pruned
}
