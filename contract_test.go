package wavelet

import (
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"testing"
)

func BenchmarkContractInGraph(b *testing.B) {
	const InitialBalance = 100000000

	stateStore := store.NewInmem()
	state := avl.New(stateStore)

	viewID := uint64(0)
	state.SetViewID(viewID)

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(b, err)

	initialRoot := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
	graph := NewGraph(WithRoot(initialRoot))

	WriteAccountBalance(state, keys.PublicKey(), 1000000000000)

	round := NewRound(viewID, state.Checksum(), 0, Transaction{}, initialRoot)

	accountState := NewAccounts(stateStore)
	assert.NoError(b, accountState.Commit(state))

	code, err := ioutil.ReadFile("testdata/transfer_back.wasm")
	assert.NoError(b, err)

	tx := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagContract, buildContractSpawnPayload(100000, 0, code).Marshal()))

	assert.NoError(b, ApplyTransaction(state, &round, &tx))

	assert.NoError(b, err)

	contractID := AccountID(tx.ID)

	var criticalCount int

	for criticalCount < b.N {
		tx := AttachSenderToTransaction(
			keys,
			NewTransaction(
				keys,
				sys.TagTransfer,
				buildTransferWithInvocationPayload(contractID, 200, 500000, []byte("on_money_received"), nil, 0).Marshal(),
			), graph.FindEligibleParents()...)
		assert.NoError(b, graph.AddTransaction(tx))

		if tx.IsCritical(4) {
			results, err := collapseTransactions(graph, accountState, viewID+1, &round, round.End, tx, false)
			assert.NoError(b, err)
			err = accountState.Commit(results.snapshot)
			assert.NoError(b, err)
			state = results.snapshot
			round = NewRound(viewID+1, state.Checksum(), uint32(results.appliedCount+results.rejectedCount), round.End, tx)
			viewID += 1
			criticalCount += 1
		}
	}
}
