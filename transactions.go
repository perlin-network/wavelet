package wavelet

import (
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/google/btree"
	"github.com/perlin-network/wavelet/conf"
	"github.com/pkg/errors"
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

	buffer  TransactionMap
	missing TransactionMap
	index   *btree.BTree

	height uint64 // The latest block height the node is aware of.
}

func NewTransactions(height uint64) *Transactions {
	return &Transactions{
		buffer:  NewTransactionMap(),
		missing: NewTransactionMap(),
		index:   btree.New(32),

		height: height,
	}
}

// Add adds a transaction into the node, and indexes it into the nodes mempool
// based on the value BLAKE2b(tx.ID || block.ID).
func (t *Transactions) Add(block BlockID, tx Transaction, verifySignature bool) {
	if verifySignature && !tx.VerifySignature() {
		return
	}

	t.Lock()
	defer t.Unlock()

	t.add(block, tx)
}

func (t *Transactions) BatchAdd(block BlockID, transactions []Transaction, verifySignature bool) {
	if verifySignature {
		filtered := transactions[:0]

		for i := range transactions {
			if transactions[i].VerifySignature() {
				filtered = append(filtered, transactions[i])
			}
		}

		transactions = filtered
	}

	t.Lock()
	defer t.Unlock()

	for _, tx := range transactions {
		t.add(block, tx)
	}
}

func (t *Transactions) add(block BlockID, tx Transaction) {
	if atomic.LoadUint64(&t.height) >= tx.Block+uint64(conf.GetPruningLimit()) {
		t.missing.Delete(tx.ID)

		return
	}

	if _, exists := t.buffer.Get(tx.ID); exists {
		return
	}

	t.index.ReplaceOrInsert(mempoolItem{index: tx.ComputeIndex(block), id: tx.ID})
	t.buffer.Put(tx.ID, &tx)

	t.missing.Delete(tx.ID) // In case the transaction was previously missing, mark it as no longer missing.
}

// MarkMissing marks that the node was expected to have archived a transaction with a specified id, but
// does not have it archived and so needs to have said transaction pulled from the nodes peers.
func (t *Transactions) MarkMissing(id TransactionID) bool {
	// t.Lock()
	// defer t.Unlock()

	return t.markMissing(id)
}

// BatchMarkMissing is the same as MarkMissing, but it accepts a list of transaction IDs.
// It returns false if at least 1 transaction ID is found missing.
func (t *Transactions) BatchMarkMissing(ids ...TransactionID) bool {
	// t.Lock()
	// defer t.Unlock()

	var missing bool

	for _, id := range ids {
		if t.markMissing(id) {
			missing = true
		}
	}

	return missing
}

func (t *Transactions) markMissing(id TransactionID) bool {
	_, exists := t.buffer.Get(id)

	if exists {
		return false
	}

	t.missing.Put(id, atomic.LoadUint64(&t.height))

	return true
}

// ReshufflePending reshuffles all transactions that may be proposed into a new block by recomputing
// the indices of all blocks given an updated block.
//
// It also prunes away transactions that are too stale, based on the index specified of the next
// block. It returns the total number of transactions pruned.
func (t *Transactions) ReshufflePending(next Block) []TransactionID {
	t.Lock()
	defer t.Unlock()

	// Delete mempool entries for transactions in the finalized block.

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

		txe, _ := t.buffer.Get(item.id)
		tx := txe.(*Transaction)

		if next.Index < tx.Block+uint64(conf.GetPruningLimit()) {
			item.index = tx.ComputeIndex(next.ID)
			items = append(items, item)
		}

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
	pruned := []TransactionID{}

	t.buffer.Iterate(func(key TransactionID, value interface{}) bool {
		tx := value.(*Transaction)

		if next.Index >= tx.Block+uint64(conf.GetPruningLimit()) {
			t.buffer.Delete(tx.ID)
			pruned = append(pruned, tx.ID)
		}

		return true
	})

	// Prune any IDs of transactions that we are trying to pull from our peers
	// that have not managed to be pulled for `PruningLimit` blocks.

	t.missing.Iterate(func(key TransactionID, value interface{}) bool {
		height := value.(uint64)

		if next.Index >= height+uint64(conf.GetPruningLimit()) {
			t.missing.Delete(key)
		}

		return true
	})

	// Have all IDs of transactions missing from now on be marked to be missing from
	// a new block height.

	atomic.StoreUint64(&t.height, next.Index)

	return pruned
}

// Has returns whether or not the node is archiving some transaction specified
// by an id.
func (t *Transactions) Has(id TransactionID) bool {
	// t.RLock()
	// defer t.RUnlock()

	_, exists := t.buffer.Get(id)
	return exists
}

func (t *Transactions) HasPending(block BlockID, id TransactionID) bool {
	txe, exists := t.buffer.Get(id)
	if !exists {
		return false
	}

	t.RLock()
	defer t.RUnlock()

	tx := txe.(*Transaction)
	return t.index.Has(mempoolItem{index: tx.ComputeIndex(block), id: id})
}

// Find searches and returns a transaction by its id if the node has it
// archived.
func (t *Transactions) Find(id TransactionID) *Transaction {
	// t.RLock()
	// defer t.RUnlock()

	txe, exists := t.buffer.Get(id)
	if !exists {
		return nil
	}

	return txe.(*Transaction)
}

// BatchFind returns an error if one of the id does not exist.
func (t *Transactions) BatchFind(ids []TransactionID) ([]*Transaction, error) {
	// t.RLock()
	// defer t.RUnlock()

	txs := make([]*Transaction, 0, len(ids))

	for i := range ids {
		txe, exist := t.buffer.Get(ids[i])
		if !exist {
			return nil, errors.Wrapf(ErrMissingTx, "%x", ids[i])
		}

		txs = append(txs, txe.(*Transaction))
	}

	return txs, nil
}

// Len returns the total number of transactions that the node has archived.
func (t *Transactions) Len() int {
	// t.RLock()
	// defer t.RUnlock()

	return int(t.buffer.Len())
}

// PendingLen returns the number of transactions the node has archived that may be proposed
// into a block.
func (t *Transactions) PendingLen() int {
	// t.RLock()
	// defer t.RUnlock()

	return t.index.Len()
}

// MissingLen returns the number of transactions that the node is looking to pull from
// its peers.
func (t *Transactions) MissingLen() int {
	// t.RLock()
	// defer t.RUnlock()

	return int(t.missing.Len())
}

// Iterate iterates through all transactions that the node has archived.
func (t *Transactions) Iterate(fn func(*Transaction) bool) {
	// t.RLock()
	// defer t.RUnlock()

	t.buffer.Iterate(func(key TransactionID, value interface{}) bool {
		tx := value.(*Transaction)
		return fn(tx)
	})
}

// ProposableIDs returns a slice of IDs of transactions that may be wrapped
// into a block that may be proposed to be finalized within the network.
func (t *Transactions) ProposableIDs() []TransactionID {
	t.RLock()
	defer t.RUnlock()

	limit := int(conf.GetBlockTXLimit())
	if t.index.Len() < limit {
		limit = t.index.Len()
	}

	proposable := make([]TransactionID, 0, limit)

	t.index.Ascend(func(i btree.Item) bool {
		txe, _ := t.buffer.Get(i.(mempoolItem).id)
		tx := txe.(*Transaction)

		if tx.Block <= atomic.LoadUint64(&t.height)+1 {
			proposable = append(proposable, i.(mempoolItem).id)
		}

		return len(proposable) < limit
	})

	return proposable
}

// MissingIDs returns a slice of IDs of transactions which the node would
// like to pull from its peers.
func (t *Transactions) MissingIDs() []TransactionID {
	// t.RLock()
	// defer t.RUnlock()

	missing := make([]TransactionID, 0, len(t.missing))

	t.missing.Iterate(func(key TransactionID, value interface{}) bool {
		missing = append(missing, key)
		return true
	})

	return missing
}
