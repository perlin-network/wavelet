package wavelet

import (
	"github.com/perlin-network/wavelet/common"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
	"testing/quick"
)

func TestCorrectGraphState(t *testing.T) {
	g := NewGraph()

	tx1 := randomTX(t)
	tx1.ParentIDs = []common.TransactionID{common.ZeroTransactionID}
	tx1.Depth = 1

	tx2 := randomTX(t)
	tx2.ParentIDs = []common.TransactionID{tx1.ID}
	tx2.Depth = 2

	tx3 := randomTX(t)
	tx3.ParentIDs = []common.TransactionID{tx1.ID, tx2.ID}
	tx3.Depth = 3

	tx4 := randomTX(t)
	tx4.ParentIDs = []common.TransactionID{tx3.ID}
	tx4.Depth = 4

	assert.NoError(t, g.addTransaction(tx1))
	assert.NoError(t, g.addTransaction(tx2))
	assert.NoError(t, g.addTransaction(tx3))
	assert.NoError(t, g.addTransaction(tx4))

	assert.Len(t, g.transactions, 5)
	assert.Len(t, g.children, 4)
	assert.Len(t, g.incomplete, 0)
	assert.Len(t, g.missing, 0)

	badTX1 := randomTX(t)
	badTX2 := randomTX(t)

	badTX1.ParentIDs = []common.TransactionID{tx4.ID}
	badTX1.Depth = 5

	badTX2.ParentIDs = []common.TransactionID{badTX1.ID}
	badTX2.Depth = 6

	// Add incomplete transaction with one missing parent.
	assert.Error(t, g.addTransaction(badTX2))
	assert.Len(t, g.transactions, 6)
	assert.Len(t, g.children, 5)
	assert.Len(t, g.incomplete, 1)
	assert.Len(t, g.missing, 1)

	// Add transaction to make last transaction inserted complete, such that there
	// is now zero missing parents.
	assert.NoError(t, g.addTransaction(badTX1))
	assert.Len(t, g.transactions, 7)
	assert.Len(t, g.children, 6)
	assert.Len(t, g.incomplete, 0)
	assert.Len(t, g.missing, 0)
}

func randomTX(t testing.TB) Transaction {
	t.Helper()

	var id common.TransactionID

	_, err := rand.Read(id[:])
	assert.NoError(t, err)

	return Transaction{ID: id}
}

func TestAddInRandomOrder(t *testing.T) {
	f := func(n int) bool {
		n = (n + 1) % 1024

		g := NewGraph()

		transactions := randomGraph(t, *g.transactions[common.ZeroTransactionID], n)
		for _, tx := range transactions {
			assert.NoError(t, g.addTransaction(tx))
		}

		if !assert.Len(t, g.transactions, len(transactions)) {
			return false
		}

		if !assert.Len(t, g.incomplete, 0) {
			return false
		}

		if !assert.Len(t, g.missing, 0) {
			return false
		}

		g = NewGraph()

		for i, j := range rand.Perm(len(transactions)) {
			transactions[i], transactions[j] = transactions[j], transactions[i]
		}

		for _, tx := range transactions {
			_ = g.addTransaction(tx)
		}

		if !assert.Len(t, g.transactions, len(transactions)) {
			return false
		}

		if !assert.Len(t, g.incomplete, 0) {
			return false
		}

		if !assert.Len(t, g.missing, 0) {
			return false
		}

		return true
	}

	assert.NoError(t, quick.Check(f, &quick.Config{MaxCount: 1000}))
}

func randomGraph(t testing.TB, genesis Transaction, n int) []Transaction {
	t.Helper()

	transactions := []Transaction{genesis}

	for i := 0; i < n; i++ {
		tx := randomTX(t)

		numParents := 1 + rand.Intn(7)
		for x := 0; x < numParents; x++ {
			parent := transactions[rand.Intn(len(transactions))]

			if tx.Depth < parent.Depth {
				tx.Depth = parent.Depth
			}

			tx.ParentIDs = append(tx.ParentIDs, parent.ID)
		}

		tx.Depth++

		transactions = append(transactions, tx)
	}

	return transactions
}
