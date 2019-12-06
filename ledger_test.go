// +build unit

package wavelet

import (
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLedger_FilterInvalidVotes(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t)

	var transactions []*Transaction
	var ids []TransactionID

	current := NewBlock(uint64(100+conf.GetPruningLimit()), alice.ledger.accounts.tree.Checksum(), ids...)

	// Create transactions.
	for i := 0; i < conf.GetSnowballK()*2; i++ {
		tx := NewTransaction(alice.Keys(), uint64(i), current.Index, sys.TagTransfer, nil)

		transactions = append(transactions, &tx)
		ids = append(ids, tx.ID)
	}

	alice.ledger.transactions.BatchUnsafeAdd(transactions)

	// Crete a single transaction with an invalid height.
	invalid := NewTransaction(alice.Keys(), uint64(conf.GetSnowballK()*2), current.Index-uint64(conf.GetPruningLimit()), sys.TagTransfer, nil)
	alice.ledger.transactions.BatchUnsafeAdd([]*Transaction{&invalid})

	// Create valid block proposals.
	var proposals []Block

	for i := 1; i <= conf.GetSnowballK(); i++ {
		proposal := NewBlock(current.Index+1, alice.ledger.accounts.tree.Checksum(), ids[:i]...)
		proposals = append(proposals, proposal)
	}

	// Create a single block proposal containing the transaction with invalid height.
	proposals = append(proposals, NewBlock(current.Index+1, alice.ledger.accounts.tree.Checksum(), append(ids, invalid.ID)...))

	votes := make([]Vote, 0, len(proposals))

	for _, proposal := range proposals {
		votes = append(votes, &finalizationVote{voter: alice.client.ID(), block: &proposal, tally: float64(1) / float64(len(proposals))})
	}

	assert.NotNil(t, votes[len(votes)-1].(*finalizationVote).block)

	alice.ledger.filterInvalidVotes(&current, votes)

	assert.Nil(t, votes[len(votes)-1].(*finalizationVote).block)
}
