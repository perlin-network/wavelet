package wavelet

import (
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"math/rand"
	"testing"
)

type benchmarkCollapse struct {
	graph      *Graph
	accounts   map[AccountID]*skademlia.Keypair
	accountIDs []AccountID

	accountState *Accounts

	end    *Transaction
	viewID uint64
	round  Round
}

func newBenchmarkCollapse(b *testing.B, noOfAcc int) *benchmarkCollapse {
	if noOfAcc < 2 {
		assert.FailNow(b, "noOfAcc must be at least 2")
	}

	const InitialBalance = 100 * 1000 * 1000 * 1000 * 1000

	stateStore := store.NewInmem()
	state := avl.New(stateStore)
	var initialRoot Transaction

	viewID := uint64(0)
	state.SetViewID(viewID)

	var graph *Graph

	accounts := make(map[AccountID]*skademlia.Keypair)
	accountIDs := make([]AccountID, 0)
	for i := 0; i < noOfAcc; i++ {
		keys, err := skademlia.NewKeys(1, 1)
		assert.NoError(b, err)

		WriteAccountBalance(state, keys.PublicKey(), InitialBalance)

		accounts[keys.PublicKey()] = keys
		accountIDs = append(accountIDs, keys.PublicKey())
	}

	firstAccount := accounts[accountIDs[0]]
	initialRoot = AttachSenderToTransaction(firstAccount, NewTransaction(firstAccount, sys.TagNop, nil))
	graph = NewGraph(WithRoot(initialRoot))

	if graph == nil {
		assert.FailNow(b, "graph is nil")
	}

	accountState := NewAccounts(stateStore)
	assert.NoError(b, accountState.Commit(state))

	round := NewRound(viewID, state.Checksum(), 0, Transaction{}, initialRoot)

	testGraph := &benchmarkCollapse{
		graph:        graph,
		accounts:     accounts,
		accountIDs:   accountIDs,
		accountState: accountState,
		end:          &initialRoot,
		viewID:       viewID,
		round:        round,
	}

	return testGraph
}

func (g *benchmarkCollapse) applyContract(b *testing.B, code []byte) (Transaction, error) {
	rng := rand.New(rand.NewSource(42))

	// Choose a random
	var sender = g.accounts[g.accountIDs[rng.Intn(len(g.accountIDs))]]

	tx := AttachSenderToTransaction(sender,
		NewTransaction(sender, sys.TagContract, buildContractSpawnPayload(100000, 0, code).Marshal()), g.graph.FindEligibleParents()...
	)

	if err := g.graph.AddTransaction(tx); err != nil {
		return Transaction{}, err
	}

	if _, err := g.collapseTransactions(b); err != nil {
		return Transaction{}, err
	}
	return tx, nil
}

func (g *benchmarkCollapse) collapseTransactions(b *testing.B) (*collapseResults, error) {
	if g.end == nil {
		return nil, errors.New("end transaction is nil")
	}
	g.viewID = + 1

	results, err := collapseTransactions(g.graph, g.accountState, g.viewID, &g.round, g.round.End, *g.end, false)
	if err != nil {
		return nil, err
	}

	b.StopTimer()

	g.round = NewRound(g.viewID, results.snapshot.Checksum(), uint32(results.appliedCount+results.rejectedCount), g.round.End, *g.end)
	g.end = nil

	b.StartTimer()

	return results, err
}

// Call collapseTransactions with a copy of the account state.
// Use this benchmark collapseTransactions.
func (g *benchmarkCollapse) collapseTransactionsNewState(b *testing.B) (*collapseResults, error) {
	if g.end == nil {
		return nil, errors.New("end transaction is nil")
	}

	b.StopTimer()

	accountState := NewAccounts(store.NewInmem())

	if err := accountState.Commit(g.accountState.Snapshot()); err != nil {
		return nil, errors.Wrap(err, "failed to commit")
	}

	b.StartTimer()

	results, err := collapseTransactions(g.graph, accountState, g.viewID+1, &g.round, g.round.End, *g.end, false)
	if err != nil {
		return nil, err
	}

	return results, err
}

func (g *benchmarkCollapse) addTxs(b *testing.B, noOfTx int, getTx func(sender *skademlia.Keypair) Transaction) {
	rng := rand.New(rand.NewSource(42))

	var tx Transaction
	for i := 0; i < noOfTx; i++ {
		// Choose random sender
		var sender = g.accounts[g.accountIDs[rng.Intn(len(g.accountIDs))]]

		tx = getTx(sender)

		assert.NoError(b, g.graph.AddTransaction(tx))
	}

	g.end = &tx
}

func (g *benchmarkCollapse) addStakeTxs(b *testing.B, noOfTx int) {
	g.addTxs(b, noOfTx, func(sender *skademlia.Keypair) Transaction {
		return AttachSenderToTransaction(sender,
			NewTransaction(sender, sys.TagStake, buildPlaceStakePayload(1).Marshal()),
			g.graph.FindEligibleParents()...
		)
	})
}

func (g *benchmarkCollapse) addTransferTxs(b *testing.B, noOfTx int) {
	rng := rand.New(rand.NewSource(42))

	var recipient *skademlia.Keypair
	g.addTxs(b, noOfTx, func(sender *skademlia.Keypair) Transaction {
		// Choose a random account as the recipient.
		for {
			recipient = g.accounts[g.accountIDs[rng.Intn(len(g.accountIDs))]]
			// Make sure recipient is equal to sender
			if recipient != sender {
				break
			}
		}

		return AttachSenderToTransaction(sender,
			NewTransaction(sender, sys.TagTransfer, buildTransferPayload(recipient.PublicKey(), 1).Marshal()),
			g.graph.FindEligibleParents()...
		)
	})
}

func (g *benchmarkCollapse) addContractTransferTxs(b *testing.B, noOfTx int, sender, contractID AccountID, funcName []byte, amount, gasLimit, gasDeposit uint64) {
	g.addTxs(b, noOfTx, func(sender *skademlia.Keypair) Transaction {
		tx := NewTransaction(
			sender,
			sys.TagTransfer,
			buildTransferWithInvocationPayload(contractID, 200, 500000, []byte("on_money_received"), nil, 0).Marshal(),
		)

		tx = AttachSenderToTransaction(sender,
			tx,
			g.graph.FindEligibleParents()...
		)

		return tx
	})
}

func (g *benchmarkCollapse) addContractCreationTxs(b *testing.B, noOfTx int, code []byte) {
	g.addTxs(b, noOfTx, func(sender *skademlia.Keypair) Transaction {
		return AttachSenderToTransaction(sender,
			NewTransaction(sender, sys.TagContract, buildContractSpawnPayload(100000, 0, code).Marshal()),
			g.graph.FindEligibleParents()...
		)
	})
}

func BenchmarkCollapseTransactionsStake100(b *testing.B) {
	graph := newBenchmarkCollapse(b, 3)
	graph.addStakeTxs(b, 100)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 100, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsStake1000(b *testing.B) {
	graph := newBenchmarkCollapse(b, 3)
	graph.addStakeTxs(b, 1000)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 1000, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsStake10000(b *testing.B) {
	graph := newBenchmarkCollapse(b, 3)
	graph.addStakeTxs(b, 10000)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 10000, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsStake100000(b *testing.B) {
	graph := newBenchmarkCollapse(b, 3)
	graph.addStakeTxs(b, 100000)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 100000, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsTransfer100(b *testing.B) {
	graph := newBenchmarkCollapse(b, 3)
	graph.addTransferTxs(b, 100)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 100, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsTransfer1000(b *testing.B) {
	graph := newBenchmarkCollapse(b, 3)
	graph.addTransferTxs(b, 1000)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 1000, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsTransfer10000(b *testing.B) {
	graph := newBenchmarkCollapse(b, 3)
	graph.addTransferTxs(b, 10000)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 10000, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsTransfer100000(b *testing.B) {
	graph := newBenchmarkCollapse(b, 3)
	graph.addTransferTxs(b, 100000)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 100000, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsContractCreation100(b *testing.B) {
	code, err := ioutil.ReadFile("testdata/transfer_back.wasm")
	assert.NoError(b, err)

	graph := newBenchmarkCollapse(b, 3)
	graph.addContractCreationTxs(b, 100, code)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 100, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsContractCreation1000(b *testing.B) {
	code, err := ioutil.ReadFile("testdata/transfer_back.wasm")
	assert.NoError(b, err)

	graph := newBenchmarkCollapse(b, 3)
	graph.addContractCreationTxs(b, 1000, code)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactions(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 1000, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsContractCreation10000(b *testing.B) {
	code, err := ioutil.ReadFile("testdata/transfer_back.wasm")
	assert.NoError(b, err)

	graph := newBenchmarkCollapse(b, 3)
	graph.addContractCreationTxs(b, 10000, code)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 10000, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsContractTransfer100(b *testing.B) {
	code, err := ioutil.ReadFile("testdata/transfer_back.wasm")
	assert.NoError(b, err)

	graph := newBenchmarkCollapse(b, 3)

	contract, err := graph.applyContract(b, code)
	if err != nil {
		b.Fatal(err)
	}

	graph.addContractTransferTxs(b, 100, contract.Sender, contract.ID, []byte("on_money_received"), 200, 500000, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 101, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsContractTransfer1000(b *testing.B) {
	code, err := ioutil.ReadFile("testdata/transfer_back.wasm")
	assert.NoError(b, err)

	graph := newBenchmarkCollapse(b, 3)

	contract, err := graph.applyContract(b, code)
	if err != nil {
		b.Fatal(err)
	}

	graph.addContractTransferTxs(b, 1000, contract.Sender, contract.ID, []byte("on_money_received"), 200, 500000, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 1001, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsContractTransfer10000(b *testing.B) {
	code, err := ioutil.ReadFile("testdata/transfer_back.wasm")
	assert.NoError(b, err)

	graph := newBenchmarkCollapse(b, 3)

	contract, err := graph.applyContract(b, code)
	if err != nil {
		b.Fatal(err)
	}

	graph.addContractTransferTxs(b, 10000, contract.Sender, contract.ID, []byte("on_money_received"), 200, 500000, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 10001, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsContractTransfer100000(b *testing.B) {
	code, err := ioutil.ReadFile("testdata/transfer_back.wasm")
	assert.NoError(b, err)

	graph := newBenchmarkCollapse(b, 3)

	contract, err := graph.applyContract(b, code)
	if err != nil {
		b.Fatal(err)
	}

	graph.addContractTransferTxs(b, 100000, contract.Sender, contract.ID, []byte("on_money_received"), 200, 500000, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 100001, results.appliedCount)
	}
}

func BenchmarkCollapseTransactionsMixed(b *testing.B) {
	code, err := ioutil.ReadFile("testdata/transfer_back.wasm")
	assert.NoError(b, err)

	graph := newBenchmarkCollapse(b, 3)

	contract, err := graph.applyContract(b, code)
	if err != nil {
		b.Fatal(err)
	}

	graph.addContractTransferTxs(b, 30000, contract.Sender, contract.ID, []byte("on_money_received"), 200, 500000, 0)
	graph.addTransferTxs(b, 30000)
	graph.addStakeTxs(b, 30000)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		results, err := graph.collapseTransactionsNewState(b)
		if err != nil {
			b.Fatal(err)
		}
		assert.Equal(b, 1+30000+30000+30000, results.appliedCount)
	}
}
