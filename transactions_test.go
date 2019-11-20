// +build !integration,unit

package wavelet

import (
	"crypto/rand"
	"math"
	"sort"
	"testing"
	"testing/quick"

	"github.com/google/btree"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
)

func TestTransactions(t *testing.T) {
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	fn := func(block BlockID, numTransactions uint8) bool {
		manager := NewTransactions(0)

		// Create `numTransactions` unique transactions.

		transactions := make([]Transaction, 0, numTransactions+1)

		for i := 0; i < int(numTransactions)+1; i++ {
			transactions = append(transactions, NewTransaction(keys, uint64(i), 0, sys.TagTransfer, nil))
		}

		// Check that all transactions were successfully stored in the manager.

		manager.BatchAdd(block, transactions...)

		// Attempt to re-add all transactions that were already stored in the manager.

		for _, tx := range transactions {
			manager.Add(block, tx)
		}

		if !assert.Len(t, manager.buffer, len(transactions)) || !assert.Equal(t, manager.Len(), len(transactions)) {
			return false
		}

		if !assert.Equal(t, manager.index.Len(), len(transactions)) || !assert.Equal(t, manager.PendingLen(), len(transactions)) {
			return false
		}

		// Check that transactions are properly stored in the manager.

		for _, tx := range transactions {
			if !assert.True(t, manager.Has(tx.ID)) {
				return false
			}

			if !assert.Equal(t, *manager.Find(tx.ID), tx) {
				return false
			}
		}

		// Test iterating functions.

		manager.Iterate(func(tx *Transaction) bool {
			if !assert.Contains(t, transactions, *tx) {
				t.FailNow()
			}

			return true
		})

		i := 0

		manager.Iterate(func(tx *Transaction) bool {
			i++

			return i != 2
		})

		// Check that the mempool index is sorted properly.

		sort.Slice(transactions, func(i, j int) bool {
			return transactions[i].ComputeIndex(block).Cmp(transactions[j].ComputeIndex(block)) < 0
		})

		for i, id := range manager.ProposableIDs() {
			if transactions[i].ID != id {
				return false
			}
		}

		return true
	}

	assert.NoError(t, quick.Check(fn, nil))
}

func TestTransactionsMarkMissing(t *testing.T) {
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	fn := func(numTransactions uint8) bool {
		manager := NewTransactions(0)

		// Mark a single transaction missing.

		tx := NewTransaction(keys, 0, 0, sys.TagTransfer, nil)
		manager.MarkMissing(tx.ID)

		// Mark a block of transactions missing.

		transactions := make([]Transaction, 0, numTransactions)
		ids := make([]TransactionID, 0, cap(transactions))

		for i := 0; i < cap(transactions); i++ {
			tx := NewTransaction(keys, uint64(i+1), 0, sys.TagTransfer, nil)

			ids = append(ids, tx.ID)
			transactions = append(transactions, tx)
		}

		for _, id := range ids {
			manager.MarkMissing(id)
		}

		// Assert the correct number of transactions are marked as missing,
		if !assert.Len(t, manager.missing, cap(transactions)+1) || !assert.Len(t, manager.MissingIDs(), cap(transactions)+1) {
			return false
		}

		// Adding all the transactions into the manager should make len(missing) = 0.

		manager.Add(ZeroBlockID, tx)
		manager.BatchAdd(ZeroBlockID, transactions...)

		if !assert.Len(t, manager.missing, 0) || !assert.Len(t, manager.MissingIDs(), 0) {
			return false
		}

		return true
	}

	assert.NoError(t, quick.Check(fn, nil))
}

func TestTransactionsReshuffleIndices(t *testing.T) {
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	fn := func(numTransactions uint8, prev BlockID) bool {
		if numTransactions == 0 {
			numTransactions++
		}

		manager := NewTransactions(0)

		// Generate and add a bunch of transactions to the manager.

		transactions := make([]Transaction, 0, numTransactions)

		for i := 0; i < cap(transactions); i++ {
			tx := NewTransaction(keys, uint64(i+1), 0, sys.TagTransfer, nil)

			transactions = append(transactions, tx)
		}

		manager.BatchAdd(prev, transactions...)

		// Generate a unique next-block ID to shuffle with.

		next, err := NewBlock(1, ZeroMerkleNodeID)
		if !assert.NoError(t, err) {
			return false
		}

		for {
			if next.ID != prev {
				break
			}

			if _, err := rand.Read(next.ID[:]); !assert.NoError(t, err) { // nolint:gosec
				return false
			}
		}

		// Check that the indices assigned are correct in the mempool index.

		manager.index.Ascend(func(i btree.Item) bool {
			item := i.(mempoolItem)

			if !assert.Equal(t, item.index.Cmp(manager.Find(item.id).ComputeIndex(prev)), 0) {
				t.FailNow()
			}

			return true
		})

		// Shuffle the manager, and assert no transactions have been pruned.

		if !assert.Len(t, manager.ReshufflePending(next), 0) {
			return false
		}

		// Check that the shuffle worked correctly, and the indices were updated.

		manager.index.Ascend(func(i btree.Item) bool {
			item := i.(mempoolItem)

			if !assert.Zero(t, item.index.Cmp(manager.Find(item.id).ComputeIndex(next.ID))) {
				t.FailNow()
			}

			return true
		})

		return assert.Equal(t, manager.height, next.Index)
	}

	assert.NoError(t, quick.Check(fn, nil))
}

func TestTransactionsPruneOnReshuffle(t *testing.T) { // nolint:gocognit
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	fn := func(numProposed, numFinalized, numMissing uint8, prev BlockID) bool {
		if numProposed == 0 {
			numProposed++
		}

		if numFinalized == 0 {
			numFinalized++
		}

		if numMissing == 0 {
			numMissing++
		}

		manager := NewTransactions(0)

		// Generate and add a bunch of proposable transactions to the manager.

		toNotBePrunedTransactions := make([]Transaction, 0)
		toNotBePrunedIDs := make([]TransactionID, 0)

		toBePrunedTransactions := make([]Transaction, 0)

		for i := 0; i < int(numProposed); i++ {
			if i%2 == 0 {
				tx := NewTransaction(keys, uint64(i+1), uint64(conf.GetPruningLimit())/2, sys.TagTransfer, nil)

				toNotBePrunedIDs = append(toNotBePrunedIDs, tx.ID)
				toNotBePrunedTransactions = append(toNotBePrunedTransactions, tx)
			} else {
				tx := NewTransaction(keys, uint64(i+1), 0, sys.TagTransfer, nil)

				toBePrunedTransactions = append(toBePrunedTransactions, tx)
			}
		}

		manager.BatchAdd(prev, toNotBePrunedTransactions...)
		manager.BatchAdd(prev, toBePrunedTransactions...)

		// Generate and add a bunch of finalized transactions to the manager.

		finalizedTransactions := make([]Transaction, 0)

		for i := 0; i < int(numFinalized); i++ {
			tx := NewTransaction(keys, uint64(i+1), 0, sys.TagStake, nil)

			finalizedTransactions = append(finalizedTransactions, tx)

			manager.buffer[tx.ID] = &tx // Do not index the transaction into the mempool index.
		}

		// Generate and mark a bunch of new transactions as missing to the manager.

		missingIDs := make([]TransactionID, 0)

		for i := 0; i < int(numMissing); i++ {
			tx := NewTransaction(keys, uint64(i+1), 0, sys.TagContract, nil)
			missingIDs = append(missingIDs, tx.ID)
		}

		for _, id := range missingIDs {
			manager.MarkMissing(id)
		}

		// Generate a unique next-block ID to shuffle with that is just 1 block index before the pruning limit.

		next, err := NewBlock(uint64(conf.GetPruningLimit())-1, ZeroMerkleNodeID)
		if !assert.NoError(t, err) {
			return false
		}

		for {
			if next.ID != prev {
				break
			}

			if _, err := rand.Read(next.ID[:]); !assert.NoError(t, err) { // nolint:gosec
				return false
			}
		}

		// Shuffle the manager, and assert that no transactions have been pruned.

		if !assert.Len(t, manager.ReshufflePending(next), 0) {
			return false
		}

		// Generate a unique next-block ID to shuffle with that is exactly at the pruning limit.

		next, err = NewBlock(uint64(conf.GetPruningLimit()), ZeroMerkleNodeID)
		if !assert.NoError(t, err) {
			return false
		}

		for {
			if next.ID != prev {
				break
			}

			if _, err := rand.Read(next.ID[:]); !assert.NoError(t, err) { // nolint:gosec
				return false
			}
		}

		// Check that the correct number of transactions are marked as missing.

		if !assert.Len(t, manager.missing, len(missingIDs)) {
			return false
		}

		// Shuffle the manager, and assert that the correct number of transactions have been pruned.

		if !assert.Equal(t, len(manager.ReshufflePending(next)), len(toBePrunedTransactions)+len(finalizedTransactions)) {
			return false
		}

		// Check that after shuffling, there are no longer any transactions that are marked as missing.

		if !assert.Len(t, manager.missing, 0) {
			return false
		}

		// Assert that the managers height has been properly updated.

		if !assert.Equal(t, manager.height, next.Index) {
			return false
		}

		// Assert that transactions that are not pruned are still proposable.

		sort.Slice(toNotBePrunedIDs, func(i, j int) bool {
			return manager.Find(toNotBePrunedIDs[i]).ComputeIndex(next.ID).Cmp(manager.Find(toNotBePrunedIDs[j]).ComputeIndex(next.ID)) < 0
		})

		if !assert.Equal(t, toNotBePrunedIDs, manager.ProposableIDs()) {
			return false
		}

		// Check that stale transactions cannot be added to the manager.

		before := len(manager.buffer)
		manager.Add(ZeroBlockID, NewTransaction(keys, math.MaxUint64, 0, sys.TagStake, nil))
		return assert.Len(t, manager.buffer, before)
	}

	assert.NoError(t, quick.Check(fn, nil))
}
