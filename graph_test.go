package wavelet

import (
	"bytes"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/blake2b"
	"math/rand"
	"os"
	"sort"
	"testing"
	"testing/quick"
)

func TestCorrectGraphState(t *testing.T) {
	g := NewGraph(nil)

	tx1 := randomTX(t, common.ZeroTransactionID)
	tx1.Depth = 1
	tx1.Confidence = 1

	tx2 := randomTX(t, tx1.ID)
	tx2.Depth = 2
	tx2.Confidence = 2

	tx3 := randomTX(t, tx1.ID, tx2.ID)
	tx3.Depth = 3
	tx3.Confidence = 4

	tx4 := randomTX(t, tx3.ID)
	tx4.Depth = 4
	tx4.Confidence = 5

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
	badTX1.Confidence = 6

	badTX2.ParentIDs = []common.TransactionID{badTX1.ID}
	badTX2.Depth = 6
	badTX2.Confidence = 7

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

func randomTX(t testing.TB, parents ...common.TransactionID) Transaction {
	t.Helper()

	var tx Transaction

	// Set transaction ID.
	_, err := rand.Read(tx.ID[:])
	assert.NoError(t, err)

	// Set transaction sender.
	_, err = rand.Read(tx.Sender[:])
	assert.NoError(t, err)

	// Set transaction creator.
	_, err = rand.Read(tx.Creator[:])
	assert.NoError(t, err)

	// Set transaction parents.
	tx.ParentIDs = parents

	sort.Slice(tx.ParentIDs, func(i, j int) bool {
		return bytes.Compare(tx.ParentIDs[i][:], tx.ParentIDs[j][:]) < 0
	})

	// Set transaction seed.
	var buf bytes.Buffer
	_, _ = buf.Write(tx.Sender[:])
	for _, parentID := range tx.ParentIDs {
		_, _ = buf.Write(parentID[:])
	}
	seed := blake2b.Sum256(buf.Bytes())
	tx.Seed = byte(prefixLen(seed[:]))

	return tx
}

func TestAddInRandomOrder(t *testing.T) {
	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	f := func(n int) bool {
		n = (n + 1) % 1024

		ledger := NewLedger(keys, store.NewInmem())

		for i := 0; i < n; i++ {
			tx := NewTransaction(keys, sys.TagNop, nil)
			tx, err = ledger.attachSenderToTransaction(tx)
			assert.NoError(t, err)

			assert.NoError(t, ledger.graph.addTransaction(tx))
		}

		var transactions []Transaction

		for _, tx := range ledger.graph.transactions {
			transactions = append(transactions, *tx)
		}

		if !assert.Len(t, ledger.graph.transactions, len(transactions)) {
			return false
		}

		if !assert.Len(t, ledger.graph.incomplete, 0) {
			return false
		}

		if !assert.Len(t, ledger.graph.missing, 0) {
			return false
		}

		for i, j := range rand.Perm(len(transactions)) {
			transactions[i], transactions[j] = transactions[j], transactions[i]
		}

		ledger = NewLedger(keys, store.NewInmem())

		for _, tx := range transactions {
			_ = ledger.graph.addTransaction(tx)
		}

		if !assert.Len(t, ledger.graph.transactions, len(transactions)) {
			return false
		}

		if !assert.Len(t, ledger.graph.incomplete, 0) {
			return false
		}

		if !assert.Len(t, ledger.graph.missing, 0) {
			return false
		}

		return true
	}

	assert.NoError(t, quick.Check(f, &quick.Config{MaxCount: 100}))
}

func GetKV(kv string, path string) (store.KV, func()) {
	if kv == "inmem" {
		inmemdb := store.NewInmem()
		return inmemdb, func() {
			_ = inmemdb.Close()
		}
	}
	if kv == "level" {
		// Remove existing db
		_ = os.RemoveAll(path)

		leveldb, err := store.NewLevelDB(path)
		if err != nil {
			panic("failed to create LevelDB: " + err.Error())
		}

		return leveldb, func() {
			_ = leveldb.Close()
			_ = os.RemoveAll(path)
		}
	}

	panic("unknown kv " + kv)
}
