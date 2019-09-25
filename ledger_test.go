package wavelet

import (
	"sync"
	"testing"
	"time"

	"github.com/perlin-network/wavelet/conf"

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

	alice := testnet.AddNode(t)
	bob := testnet.AddNode(t)

	assert.True(t, <-alice.WaitForSync())
	assert.True(t, <-bob.WaitForSync())

	_, err := testnet.faucet.Pay(alice, 1000000)
	assert.NoError(t, err)

	alice.WaitUntilConsensus(t)

	// Add lots of transactions
	var txsLock sync.Mutex
	txs := make([]Transaction, 0, 10000)

	go func() {
		for i := 0; i < cap(txs); i++ {
			tx, err := alice.Pay(bob, 1)
			assert.NoError(t, err)

			txsLock.Lock()
			txs = append(txs, tx)
			txsLock.Unlock()

			// Somehow this prevents AddTransaction from
			// returning ErrMissingParents
			time.Sleep(time.Nanosecond * 1)
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

			currRound := alice.ledger.Rounds().Latest().Index

			if currRound-prevRound > 1 {
				t.Fatal("more than 1 round finalized")
			}

			prevRound = currRound

			if appliedCount < cap(txs) {
				assert.True(t, alice.ledger.BroadcastingNop(),
					"node should not stop broadcasting nop while there are unapplied tx")
			}

			// The test is successful if all tx are applied,
			// and nop broadcasting is stopped once all tx are applied
			if appliedCount == cap(txs) && !alice.ledger.BroadcastingNop() {
				return
			}
		}
	}
}

func TestLedger_AddTransaction(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t) // alice
	testnet.AddNode(t)          // bob

	start := alice.ledger.Rounds().Latest().Index

	assert.True(t, <-alice.WaitForSync())

	// Add just 1 transaction
	_, err := testnet.faucet.PlaceStake(100)
	assert.NoError(t, err)

	// Try to wait for 2 rounds of consensus.
	// The second call should result in timeout, because
	// only 1 round should be finalized.
	assert.True(t, <-alice.WaitForConsensus())
	assert.False(t, <-alice.WaitForConsensus())

	current := alice.ledger.Rounds().Latest().Index
	assert.Equal(t, current-start, uint64(1), "expected only 1 round to be finalized")
}

func TestLedger_Pay(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t)
	bob := testnet.AddNode(t)

	for i := 0; i < 5; i++ {
		testnet.AddNode(t)
	}

	testnet.WaitForSync(t)

	assert.NoError(t, txError(testnet.Faucet().Pay(alice, 1000000)))
	alice.WaitUntilConsensus(t)

	assert.NoError(t, txError(alice.Pay(bob, 1337)))
	alice.WaitUntilConsensus(t)

	// Bob should receive the tx amount
	<-bob.WaitForRound(alice.RoundIndex())
	assert.EqualValues(t, 1337, bob.Balance())

	// Alice balance should be balance-txAmount-gas
	aliceBalance := alice.Balance()
	assert.True(t, aliceBalance < 1000000-1337)

	testnet.WaitForRound(t, alice.RoundIndex())

	// Everyone else should see the updated balance of Alice and Bob
	for _, node := range testnet.Nodes() {
		assert.EqualValues(t, aliceBalance, node.BalanceOfAccount(alice))
		assert.EqualValues(t, 1337, node.BalanceOfAccount(bob))
	}
}

func TestLedger_PayInsufficientBalance(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t)
	bob := testnet.AddNode(t)

	for i := 0; i < 5; i++ {
		testnet.AddNode(t)
	}

	assert.NoError(t, txError(testnet.Faucet().Pay(alice, 1000000)))
	alice.WaitUntilConsensus(t)

	// Alice attempt to pay Bob more than what
	// she has in her wallet
	assert.NoError(t, txError(alice.Pay(bob, 1000001)))
	alice.WaitUntilConsensus(t)

	// Bob should not receive the tx amount
	assert.EqualValues(t, 0, bob.Balance())

	// Alice should have paid for gas even though the tx failed
	aliceBalance := alice.Balance()
	assert.True(t, aliceBalance > 0)
	assert.True(t, aliceBalance < 1000000)

	testnet.WaitForRound(t, alice.RoundIndex())

	// Everyone else should see the updated balance of Alice and Bob
	for _, node := range testnet.Nodes() {
		assert.EqualValues(t, aliceBalance, node.BalanceOfAccount(alice))
		assert.EqualValues(t, 0, node.BalanceOfAccount(bob))
	}
}

func TestLedger_Stake(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t)

	for i := 0; i < 5; i++ {
		testnet.AddNode(t)
	}

	testnet.WaitForSync(t)

	assert.NoError(t, txError(testnet.Faucet().Pay(alice, 1000000)))
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

func TestLedger_CallContract(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t)

	for i := 0; i < 5; i++ {
		testnet.AddNode(t)
	}

	testnet.WaitForSync(t)

	assert.NoError(t, txError(testnet.Faucet().Pay(alice, 1000000)))
	testnet.WaitForConsensus(t)

	contract, err := alice.SpawnContract("testdata/transfer_back.wasm",
		10000, nil)
	assert.NoError(t, err)

	testnet.WaitForConsensus(t)

	// Calling the contract should cause the contract to send back 250000 PERL back to alice
	_, err = alice.CallContract(contract.ID, 500000, 100000, "on_money_received", contract.ID[:])
	assert.NoError(t, err)

	alice.WaitUntilConsensus(t)

	assert.True(t, alice.Balance() > 700000)
}

func TestLedger_DepositGas(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t)

	for i := 0; i < 5; i++ {
		testnet.AddNode(t)
	}

	testnet.WaitForSync(t)

	assert.NoError(t, txError(testnet.Faucet().Pay(alice, 1000000)))
	testnet.WaitForConsensus(t)

	contract, err := alice.SpawnContract("testdata/transfer_back.wasm",
		10000, nil)
	assert.NoError(t, err)

	testnet.WaitForConsensus(t)

	_, err = alice.DepositGas(contract.ID, 654321)
	assert.NoError(t, err)

	alice.WaitUntilConsensus(t)

	assert.EqualValues(t, 654321, alice.GasBalanceOfAddress(contract.ID))
}

func TestLedger_Sync(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	// Setup network with 3 nodes
	alice := testnet.Faucet()
	for i := 0; i < 2; i++ {
		testnet.AddNode(t)
	}

	testnet.WaitForSync(t)

	// Advance the network by a few rounds larger than sys.SyncIfRoundsDifferBy
	for i := 0; i < int(conf.GetSyncIfRoundsDifferBy())+5; i++ {
		_, err := alice.PlaceStake(10)
		if err != nil {
			t.Fatal(err)
		}

		alice.WaitUntilConsensus(t)
	}

	testnet.WaitForRound(t, alice.RoundIndex())

	// When a new node joins the network, it should eventually
	// sync (state and txs) with the other nodes
	charlie := testnet.AddNode(t)

	timeout := time.NewTimer(time.Second * 30)
	for {
		select {
		case <-timeout.C:
			t.Fatal("timed out waiting for sync")

		default:
			ri := <-charlie.WaitForRound(alice.RoundIndex())
			if ri >= alice.RoundIndex() && charlie.BalanceOfAccount(alice) == alice.Balance() {
				return
			}
		}
	}
}

func TestLedger_SpamContracts(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t)
	testnet.AddNode(t)

	assert.True(t, <-alice.WaitForSync())

	_, err := testnet.faucet.Pay(alice, 100000)
	assert.NoError(t, err)

	testnet.WaitForConsensus(t)

	// spamming spawn transactions should cause no problem for consensus
	// this is possible if they applied in different order on different nodes
	for i := 0; i < 5; i++ {
		_, err := alice.SpawnContract("testdata/transfer_back.wasm", 10000, nil)
		if !assert.NoError(t, err) {
			return
		}
	}

	alice.WaitUntilConsensus(t)
}

func txError(tx Transaction, err error) error {
	return err
}
