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
	const InitialBalance = 1000000000000

	store := store.NewInmem()

	state := avl.New(store)
	viewID := uint64(0)
	state.SetViewID(viewID)

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(b, err)

	mempool := NewMempool()

	WriteAccountBalance(state, keys.PublicKey(), InitialBalance)

	round := NewBlock(viewID, state.Checksum())

	accounts := NewAccounts(store)
	assert.NoError(b, accounts.Commit(state))

	code, err := ioutil.ReadFile("testdata/transfer_back.wasm")
	assert.NoError(b, err)

	contract := NewTransaction(keys, sys.TagContract, buildContractSpawnPayload(100000, 0, code).Marshal())
	assert.NoError(b, ApplyTransaction(state, &round, &contract))

	var criticalCount int

	for criticalCount < b.N {
		mempool.Add(EmptyBlockID, NewTransaction(keys,
			sys.TagTransfer,
			buildTransferWithInvocationPayload(contract.ID, 200, 500000, []byte("on_money_received"), nil, 0).Marshal(),
		))

		//if tx.IsCritical(4) {
		//	results, err := collapseTransactions(graph, accounts, viewID+1, &round, round.End, tx, false)
		//	assert.NoError(b, err)
		//	err = accounts.Commit(results.snapshot)
		//	assert.NoError(b, err)
		//	state = results.snapshot
		//	round = NewRound(viewID+1, state.Checksum(), uint32(results.appliedCount+results.rejectedCount), round.End, tx)
		//	viewID += 1
		//	criticalCount += 1
		//}
	}
}
