package wavelet

import (
	"github.com/google/btree"
	"github.com/perlin-network/wavelet/conf"
	"math/big"
	"sync"
)

var _ btree.Item = (*mempoolItem)(nil)

type mempoolItem struct {
	index *big.Int
	id    TransactionID
}

func (m mempoolItem) Less(than btree.Item) bool {
	return m.index.Cmp(than.(mempoolItem).index) < 0
}

type Transactions struct {
	sync.RWMutex

	buffer  map[TransactionID]*Transaction
	missing map[TransactionID]struct{}
	index   *btree.BTree
}

func NewTransactions() *Transactions {
	return &Transactions{
		buffer:  make(map[TransactionID]*Transaction),
		missing: make(map[TransactionID]struct{}),
		index:   btree.New(32),
	}
}

// Add adds a transaction into the node, and indexes it into the nodes mempool
// based on the value BLAKE2b(tx.ID || block.ID).
func (t *Transactions) Add(block BlockID, tx Transaction) {
	t.Lock()
	defer t.Unlock()

	t.add(block, tx)
}

func (t *Transactions) BatchAdd(block BlockID, transactions ...Transaction) {
	t.Lock()
	defer t.Unlock()

	for _, tx := range transactions {
		t.add(block, tx)
	}
}

func (t *Transactions) add(block BlockID, tx Transaction) {
	if _, exists := t.buffer[tx.ID]; exists {
		return
	}

	t.index.ReplaceOrInsert(mempoolItem{index: tx.ComputeIndex(block), id: tx.ID})
	t.buffer[tx.ID] = &tx

	delete(t.missing, tx.ID) // In case the transaction was previously missing, mark it as no longer missing.
}

// MarkMissing marks that the node was expected to have archived a transaction with a specified id, but
// does not have it archived and so needs to have said transaction pulled from the nodes peers.
func (t *Transactions) MarkMissing(id TransactionID) {
	t.Lock()
	defer t.Unlock()

	t.markMissing(id)
}

func (t *Transactions) BatchMarkMissing(ids ...TransactionID) {
	t.Lock()
	defer t.Unlock()

	for _, id := range ids {
		t.markMissing(id)
	}
}

func (t *Transactions) markMissing(id TransactionID) {
	t.missing[id] = struct{}{}
}

// ReshufflePending reshuffles all transactions that may be proposed into a new block by recomputing
// the indices of all blocks given an updated block.
//
// It also prunes away transactions that are too stale, based on the index specified of the next
// block. It returns the total number of transactions pruned.
func (t *Transactions) ReshufflePending(next Block) int {
	t.Lock()
	defer t.Unlock()

	pruned := 0

	// Recompute indices of all items in the mempool.

	items := make([]mempoolItem, 0, t.index.Len())

	t.index.Ascend(func(i btree.Item) bool {
		item := i.(mempoolItem)
		tx := t.buffer[item.id]

		if next.Index >= tx.Block+uint64(conf.GetPruningLimit()) {
			delete(t.buffer, tx.ID)
			pruned++

			return true
		}

		item.index = tx.ComputeIndex(next.ID)
		items = append(items, item)

		return true
	})

	// Clear the entire mempool.

	t.index.Clear(false)

	// Re-insert all mempool items with the recomputed indices.

	for _, item := range items {
		t.index.ReplaceOrInsert(item)
	}

	// Go through the entire transactions index and prune away
	// any transactions that are too old.

	for _, tx := range t.buffer {
		if next.Index >= tx.Block+uint64(conf.GetPruningLimit()) {
			delete(t.buffer, tx.ID)
			pruned++
		}
	}

	return pruned
}

// Has returns whether or not the node is archiving some transaction specified
// by an id.
func (t *Transactions) Has(id TransactionID) bool {
	t.RLock()
	defer t.RUnlock()

	_, exists := t.buffer[id]
	return exists
}

// Find searches and returns a transaction by its id if the node has it
// archived.
func (t *Transactions) Find(id TransactionID) *Transaction {
	t.RLock()
	defer t.RUnlock()

	return t.buffer[id]
}

// Len returns the total number of transactions that the node has archived.
func (t *Transactions) Len() int {
	t.RLock()
	defer t.RUnlock()

	return len(t.buffer)
}

// PendingLen returns the number of transactions the node has archived that may be proposed
// into a block.
func (t *Transactions) PendingLen() int {
	t.RLock()
	defer t.RUnlock()

	return t.index.Len()
}

// IteratePending iterates through all transactions that may be proposed into a block.
func (t *Transactions) IteratePending(fn func(*Transaction)) {
	t.RLock()
	defer t.RUnlock()

	t.index.Ascend(func(i btree.Item) bool {
		fn(t.buffer[i.(mempoolItem).id]) // It is guaranteed that the transaction must exist.
		return true
	})
}

// Iterate iterates through all transactions that the node has archived.
func (t *Transactions) Iterate(fn func(*Transaction)) {
	t.RLock()
	defer t.RUnlock()

	for _, tx := range t.buffer {
		fn(tx)
	}
}

// ProposableIDs returns a slice of IDs of transactions that may be wrapped
// into a block that may be proposed to be finalized within the network.
func (t *Transactions) ProposableIDs() []TransactionID {
	t.RLock()
	defer t.RUnlock()

	proposable := make([]TransactionID, 0, t.index.Len())

	t.index.Ascend(func(i btree.Item) bool {
		proposable = append(proposable, i.(mempoolItem).id)
		return true
	})

	return proposable
}

// MissingIDs returns a slice of IDs of transactions which the node would
// like to pull from its peers.
func (t *Transactions) MissingIDs() []TransactionID {
	t.RLock()
	defer t.RUnlock()

	missing := make([]TransactionID, 0, len(t.missing))

	for id := range t.missing {
		missing = append(missing, id)
	}

	return missing
}
