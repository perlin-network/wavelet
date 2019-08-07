package wavelet

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestLedger_BroadcastNop checks that:
//
// * The ledger will keep broadcasting nop tx as long
//   as there are unapplied tx (latestTxDepth <= rootDepth).
//
// * The ledger will stop broadcasting nop once there
//   are no more unapplied tx.
func TestLedger_BroadcastNop(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	for i := 0; i < 3; i++ {
		testnet.AddNode(t, 0)
	}

	alice := testnet.AddNode(t, 1000000)
	bob := testnet.AddNode(t, 0)

	// Wait for alice to receive her PERL from the faucet
	for <-alice.WaitForConsensus() {
		if alice.Balance() > 0 {
			break
		}
	}

	// Sleep for some time to give room for the nodes to
	// bootstrap the overlay S/Kademlia network
	time.Sleep(time.Second * 3)

	// Add lots of transactions
	txs := make([]Transaction, 1000)
	var err error

	fmt.Println("adding transactions...")
	for i := 0; i < len(txs); i++ {
		txs[i], err = alice.Pay(bob, 1)
		assert.NoError(t, err)
	}

	timeout := time.NewTimer(time.Minute * 5)
	for {
		select {
		case <-timeout.C:
			t.Fatal("timed out before all transactions are applied")

		case <-alice.WaitForConsensus():
			var appliedCount int
			for _, tx := range txs {
				if alice.Applied(tx) {
					appliedCount++
				}
			}

			fmt.Printf("%d/%d tx applied\n", appliedCount, len(txs))

			if appliedCount < len(txs) {
				assert.True(t, alice.ledger.BroadcastingNop(),
					"node should not stop broadcasting nop while there are unapplied tx")
			}

			// The test is successful if all tx are applied,
			// and nop broadcasting is stopped once all tx are applied
			if appliedCount == len(txs) && !alice.ledger.BroadcastingNop() {
				return
			}
		}
	}
}

func TestLedger_AddTransaction(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t, 1000000)
	testnet.AddNode(t, 0) // bob

	// Wait for alice to receive her PERL from the faucet
	for <-alice.WaitForConsensus() {
		if alice.Balance() > 0 {
			break
		}
	}

	start := alice.ledger.Rounds().Latest().Index

	// Add just 1 transaction
	_, err := alice.PlaceStake(100)
	assert.NoError(t, err)

	// Try to wait for 2 rounds of consensus.
	// The second call should result in timeout, because
	// only 1 round should be finalized.
	<-alice.WaitForConsensus()
	<-alice.WaitForConsensus()

	current := alice.ledger.Rounds().Latest().Index
	if current-start > 1 {
		t.Fatal("more than 1 round finalized")
	}
}
