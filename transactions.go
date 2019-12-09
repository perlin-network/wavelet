package wavelet

import (
	"sync"

	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/internal/btree"
	"github.com/pkg/errors"
)

type Transactions struct {
	sync.RWMutex

	buffer    map[TransactionID]*Transaction
	missing   map[TransactionID]uint64
	finalized map[TransactionID]struct{}
	index     btree.BTree

	latest Block // The latest block height the node is aware of.
}

func NewTransactions(latest Block) *Transactions {
	return &Transactions{
		buffer:    make(map[TransactionID]*Transaction),
		missing:   make(map[TransactionID]uint64),
		finalized: make(map[TransactionID]struct{}),

		latest: latest,
	}
}

// Add adds a transaction into the node, and indexes it into the nodes mempool
// based on the value BLAKE2b(tx.ID || block.ID).
func (t *Transactions) Add(tx Transaction, verifySignature bool) {
	if verifySignature && !tx.VerifySignature() {
		return
	}

	t.Lock()
	defer t.Unlock()

	t.add(tx)
}

func (t *Transactions) BatchAdd(transactions []Transaction, verifySignature bool) {
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
		t.add(tx)
	}
}

// BatchUnsafeAdd adds transactions to buffer without adding them to index
// used on node's start
func (t *Transactions) BatchUnsafeAdd(txs []*Transaction) {
	t.Lock()
	defer t.Unlock()

	for _, tx := range txs {
		t.buffer[tx.ID] = tx
	}
}

func (t *Transactions) add(tx Transaction) {
	if t.latest.Index >= tx.Block+uint64(conf.GetPruningLimit()) {
		delete(t.missing, tx.ID)

		return
	}

	if _, exists := t.buffer[tx.ID]; exists {
		return
	}

	if _, finalized := t.finalized[tx.ID]; !finalized {
		t.index.Set(tx.ComputeIndex(t.latest.ID), tx.ID)
	}

	t.buffer[tx.ID] = &tx

	delete(t.missing, tx.ID) // In case the transaction was previously missing, mark it as no longer missing.
}

// MarkMissing marks that the node was expected to have archived a transaction with a specified id, but
// does not have it archived and so needs to have said transaction pulled from the nodes peers.
func (t *Transactions) MarkMissing(id TransactionID) bool {
	t.Lock()
	defer t.Unlock()

	return t.markMissing(id)
}

// BatchMarkFinalized saves batch of transaction ids to finalized index
func (t *Transactions) BatchMarkFinalized(ids ...TransactionID) {
	t.Lock()
	defer t.Unlock()

	for _, id := range ids {
		t.finalized[id] = struct{}{}
	}
}

// BatchMarkMissing is the same as MarkMissing, but it accepts a list of transaction IDs.
// It returns false if at least 1 transaction ID is found missing.
func (t *Transactions) BatchMarkMissing(ids ...TransactionID) bool {
	t.Lock()
	defer t.Unlock()

	var missing bool

	for _, id := range ids {
		if t.markMissing(id) {
			missing = true
		}
	}

	return missing
}

func (t *Transactions) markMissing(id TransactionID) bool {
	_, exists := t.buffer[id]

	if exists {
		return false
	}

	t.missing[id] = t.latest.Index

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

	for _, id := range next.Transactions {
		t.finalized[id] = struct{}{}
	}

	// Recompute indices of all items in the mempool.
	var updated btree.BTree

	t.index.Scan(func(key []byte, value interface{}) bool {
		id := value.(TransactionID)

		if _, finalized := t.finalized[id]; finalized {
			return true
		}

		tx := t.buffer[id]

		if next.Index < tx.Block+uint64(conf.GetPruningLimit()) {
			updated.Set(tx.ComputeIndex(next.ID), id)
		}

		return true
	})

	// Re-insert all mempool items with the recomputed indices.

	t.index = updated

	// Go through the entire transactions index and prune away
	// any transactions that are too old.
	var pruned []TransactionID

	for _, tx := range t.buffer {
		if next.Index >= tx.Block+uint64(conf.GetPruningLimit()) {
			delete(t.buffer, tx.ID)
			delete(t.finalized, tx.ID)

			pruned = append(pruned, tx.ID)
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

	t.latest = next

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

func (t *Transactions) HasPending(block BlockID, id TransactionID) bool {
	t.RLock()
	defer t.RUnlock()

	tx, exists := t.buffer[id]
	if !exists {
		return false
	}

	_, found := t.index.Get(tx.ComputeIndex(block))

	return found
}

// Find searches and returns a transaction by its id if the node has it
// archived.
func (t *Transactions) Find(id TransactionID) *Transaction {
	t.RLock()
	defer t.RUnlock()

	return t.buffer[id]
}

// BatchFind returns an error if one of the id does not exist.
func (t *Transactions) BatchFind(ids []TransactionID) ([]*Transaction, error) {
	t.RLock()
	defer t.RUnlock()

	txs := make([]*Transaction, 0, len(ids))

	for i := range ids {
		tx, exist := t.buffer[ids[i]]
		if !exist {
			return nil, errors.Wrapf(ErrMissingTx, "%x", ids[i])
		}

		txs = append(txs, tx)
	}

	return txs, nil
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

// MissingLen returns the number of transactions that the node is looking to pull from
// its peers.
func (t *Transactions) MissingLen() int {
	t.RLock()
	defer t.RUnlock()

	return len(t.missing)
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

	limit := int(conf.GetBlockTXLimit())
	if t.index.Len() < limit {
		limit = t.index.Len()
	}

	proposable := make([]TransactionID, 0, limit)

	t.index.Scan(func(key []byte, value interface{}) bool {
		id := value.(TransactionID)

		if t.buffer[id].Block <= t.latest.Index+1 {
			proposable = append(proposable, id)
		}

		return len(proposable) < limit
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
