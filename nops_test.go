package wavelet

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLedger_TransactionThroughput(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	for i := 0; i < 20; i++ {
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

	txs := make([]Transaction, 1000)
	var err error

	fmt.Println("adding transactions...")
	for i := 0; i < len(txs); i++ {
		txs[i], err = alice.Pay(bob, 1)
		assert.NoError(t, err)
	}

	timeout := time.NewTimer(time.Second * 60)
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
