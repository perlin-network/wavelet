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
	missing map[TransactionID]uint64
	index   *btree.BTree

	height uint64 // The latest block height the node is aware of.
}

func NewTransactions(height uint64) *Transactions {
	return &Transactions{
		buffer:  make(map[TransactionID]*Transaction),
		missing: make(map[TransactionID]uint64),
		index:   btree.New(32),

		height: height,
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
	if t.height >= tx.Block+uint64(conf.GetPruningLimit()) {
		return
	}

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
	t.missing[id] = t.height
}

// ReshufflePending reshuffles all transactions that may be proposed into a new block by recomputing
// the indices of all blocks given an updated block.
//
// It also prunes away transactions that are too stale, based on the index specified of the next
// block. It returns the total number of transactions pruned.
func (t *Transactions) ReshufflePending(next Block) int {
	t.Lock()
	defer t.Unlock()

	// Delete mempool entries for transactions in the finalized block.

	pruned := len(next.Transactions)

	lookup := make(map[TransactionID]struct{})

	for _, id := range next.Transactions {
		lookup[id] = struct{}{}
	}

	// Recompute indices of all items in the mempool.

	items := make([]mempoolItem, 0, t.index.Len())

	t.index.Ascend(func(i btree.Item) bool {
		item := i.(mempoolItem)

		if _, finalized := lookup[item.id]; finalized {
			return true
		}

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

	// Prune any IDs of transactions that we are trying to pull from our peers
	// that have not managed to be pulled for `PruningLimit` blocks.

	for id, height := range t.missing {
		if next.Index >= height+uint64(conf.GetPruningLimit()) {
			delete(t.missing, id)
		}
	}

	// Have all IDs of transactions missing from now on be marked to be missing from
	// a new block height.

	t.height = next.Index

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

// Iterate iterates through all transactions that the node has archived.
func (t *Transactions) Iterate(fn func(*Transaction) bool) {
	t.RLock()
	defer t.RUnlock()

	for _, tx := range t.buffer {
		if !fn(tx) {
			return
		}
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
