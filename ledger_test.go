package wavelet

import (
	"fmt"
	"sync"
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
	for range alice.WaitForConsensus() {
		if alice.Balance() > 0 {
			break
		}
	}

	// Add lots of transactions
	var txsLock sync.Mutex
	txsCount := 10000
	txs := make([]Transaction, 0, txsCount)

	go func() {
		for i := 0; i < txsCount; i++ {
			tx, err := alice.Pay(bob, 1)
			assert.NoError(t, err)

			txsLock.Lock()
			txs = append(txs, tx)
			txsLock.Unlock()
		}
	}()

	prevRound := alice.ledger.Rounds().Latest().Index
	timeout := time.NewTimer(time.Minute * 5)
	for {
		select {
		case <-timeout.C:
			t.Fatal("timed out before all transactions are applied")

		case <-alice.WaitForConsensus():
			var appliedCount int
			var txsCount int

			txsLock.Lock()
			for _, tx := range txs {
				if alice.Applied(tx) {
					appliedCount++
				}
				txsCount++
			}
			txsLock.Unlock()

			currRound := alice.ledger.Rounds().Latest()

			fmt.Printf("%d/%d tx applied, round=%d, root depth=%d\n",
				appliedCount, txsCount,
				currRound.Index,
				alice.ledger.Graph().RootDepth())

			if currRound.Rejected > 0 {
				t.Fatal("no tx should be rejected")
			}
			if currRound.Ignored > 0 {
				t.Fatal("no tx should be ignored")
			}

			if currRound.Index-prevRound > 1 {
				t.Fatal("more than 1 round finalized")
			}

			prevRound = currRound.Index

			if appliedCount < txsCount {
				assert.True(t, alice.ledger.BroadcastingNop(),
					"node should not stop broadcasting nop while there are unapplied tx")
			}

			// The test is successful if all tx are applied,
			// and nop broadcasting is stopped once all tx are applied
			if appliedCount == txsCount && !alice.ledger.BroadcastingNop() {
				return
			}
		}
	}
}

func TestLedger_Ignore(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.faucet
	bob := testnet.AddNodeWithWallet(t, "85e7450f7cf0d9cd1d1d7bf4169c2f364eea4ba833a7280e0f931a1d92fd92c2696937c2c8df35dba0169de72990b80761e51dd9e2411fa1fce147f68ade830a")
	charlie := testnet.AddNodeWithWallet(t, "5b9fcd2d6f8e34f4aa472e0c3099fefd25f0ceab9e908196b1dda63e55349d22f03bb6f98c4dfd31f3d448c7ec79fa3eaa92250112ada43471812f4b1ace6467")

	go func() {
		timeout := time.NewTimer(time.Second * 10)
		for {
			select {
			case <-timeout.C:
				return

			default:
				_, err := alice.PlaceStake(1)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}()

	go func() {
		for {
			ri := <-charlie.WaitForRound(5)
			if ri >= 5 {
				break
			}
		}

		fmt.Println("charlie: ps 100")
		_, err := charlie.PlaceStake(100)
		if err != nil {
			t.Fatal(err)
		}
	}()

	for {
		ri := <-bob.WaitForConsensus()
		if ri == 0 {
			return
		}

		<-alice.WaitForRound(ri)
		<-charlie.WaitForRound(ri)

		round := bob.ledger.Rounds().Latest()
		fmt.Printf("round: %d, applied: %d, rejected: %d, ignored: %d, alice stake: %d, charlie stake: %d\n",
			round.Index, round.Applied, round.Rejected, round.Ignored,
			alice.Stake(), charlie.Stake())

		if round.Ignored > 0 {
			t.Fatal("no tx should be ignored")
		}
	}
}

func TestLedger_AddTransaction(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t, 0) // alice
	testnet.AddNode(t, 0)          // bob

	start := alice.ledger.Rounds().Latest().Index

	// Add just 1 transaction
	_, err := testnet.faucet.PlaceStake(100)
	assert.NoError(t, err)

	// Try to wait for 2 rounds of consensus.
	// The second call should result in timeout, because
	// only 1 round should be finalized.
	<-alice.WaitForConsensus()
	<-alice.WaitForConsensus()

	current := alice.ledger.Rounds().Latest()
	if current.Index-start > 1 {
		t.Fatal("more than 1 round finalized")
	}
	if current.Rejected > 0 {
		t.Fatal("no tx should be rejected")
	}
	if current.Ignored > 0 {
		t.Fatal("no tx should be ignored")
	}
}

func TestLedger_Pay(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	for i := 0; i < 5; i++ {
		testnet.AddNode(t, 0)
	}

	alice := testnet.AddNode(t, 1000000)
	bob := testnet.AddNode(t, 100)

	testnet.WaitForConsensus(t)

	assert.NoError(t, txError(alice.Pay(bob, 1237)))

	testnet.WaitForConsensus(t)

	// Bob should receive the tx amount
	assert.EqualValues(t, 1337, bob.Balance())

	// Alice balance should be balance-txAmount-gas
	aliceBalance := alice.Balance()
	assert.True(t, aliceBalance < 1000000-1237)

	// Everyone else should see the updated balance of Alice and Bob
	for _, node := range testnet.Nodes() {
		assert.EqualValues(t, aliceBalance, node.BalanceOfAccount(alice))
		assert.EqualValues(t, 1337, node.BalanceOfAccount(bob))
	}
}

func TestLedger_PayInsufficientBalance(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	for i := 0; i < 5; i++ {
		testnet.AddNode(t, 0)
	}

	alice := testnet.AddNode(t, 1000000)
	bob := testnet.AddNode(t, 100)

	testnet.WaitForConsensus(t)

	// Alice attempt to pay Bob more than what
	// she has in her wallet
	assert.NoError(t, txError(alice.Pay(bob, 1000001)))

	testnet.WaitForConsensus(t)

	// Bob should not receive the tx amount
	assert.EqualValues(t, 100, bob.Balance())

	// Alice should have paid for gas even though the tx failed
	aliceBalance := alice.Balance()
	assert.True(t, aliceBalance > 0)
	assert.True(t, aliceBalance < 1000000)

	// Everyone else should see the updated balance of Alice and Bob
	for _, node := range testnet.Nodes() {
		assert.EqualValues(t, aliceBalance, node.BalanceOfAccount(alice))
		assert.EqualValues(t, 100, node.BalanceOfAccount(bob))

		// All nodes should have rejected the tx
		round := node.ledger.Rounds().Latest()
		assert.EqualValues(t, 1, round.Rejected)
	}
}

func TestLedger_Gossip(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	for i := 0; i < 5; i++ {
		testnet.AddNode(t, 0)
	}

	alice := testnet.AddNode(t, 1000000)
	bob := testnet.AddNode(t, 100)

	testnet.WaitForConsensus(t)

	assert.NoError(t, txError(alice.Pay(bob, 1237)))

	testnet.WaitForConsensus(t)

	// When a new node joins the network, it will eventually receive
	// all transactions in the network.
	charlie := testnet.AddNode(t, 0)

	waitFor(t, "test timed out", func() bool {
		return charlie.BalanceOfAccount(alice) == alice.Balance() &&
			charlie.BalanceOfAccount(bob) == bob.Balance()
	})
}

func TestLedger_Stake(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	for i := 0; i < 5; i++ {
		testnet.AddNode(t, 0)
	}

	alice := testnet.AddNode(t, 1000000)

	testnet.WaitForConsensus(t)

	assert.NoError(t, txError(alice.PlaceStake(9001)))
	testnet.WaitForConsensus(t)

	assert.EqualValues(t, 9001, alice.Stake())

	// Alice balance should be balance-stakeAmount-gas
	aliceBalance := alice.Balance()
	assert.True(t, aliceBalance < 1000000-9001)

	// Everyone else should see the updated balance of Alice
	for _, node := range testnet.Nodes() {
		assert.EqualValues(t, aliceBalance, node.BalanceOfAccount(alice))
		assert.EqualValues(t, alice.Stake(), node.StakeOfAccount(alice))
	}

	assert.NoError(t, txError(alice.WithdrawStake(5000)))
	testnet.WaitForConsensus(t)

	assert.EqualValues(t, 4001, alice.Stake())

	// Withdrawn stake should be added to balance
	oldBalance := aliceBalance
	aliceBalance = alice.Balance()
	assert.True(t, aliceBalance > oldBalance)

	// Everyone else should see the updated balance of Alice
	for _, node := range testnet.Nodes() {
		assert.EqualValues(t, aliceBalance, node.BalanceOfAccount(alice))
		assert.EqualValues(t, alice.Stake(), node.StakeOfAccount(alice))
	}
}

func txError(tx Transaction, err error) error {
	return err
}
