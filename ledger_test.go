package wavelet

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/log"

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

	assert.NoError(t, txError(testnet.Faucet().Pay(alice, 1000000)))
	alice.WaitUntilBalance(t, 1000000)

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
	assert.NoError(t, txError(testnet.Faucet().PlaceStake(100)))

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
	alice.WaitUntilBalance(t, 1000000)

	assert.NoError(t, txError(alice.Pay(bob, 1337)))
	bob.WaitUntilBalance(t, 1337)

	// Alice balance should be balance-txAmount-gas
	aliceBalance := alice.Balance()
	waitFor(t, func() bool { return aliceBalance < 1000000-1337 })

	// Everyone else should see the updated balance of Alice and Bob
	for _, node := range testnet.Nodes() {
		waitFor(t, func() bool {
			return node.BalanceOfAccount(alice) == aliceBalance &&
				node.BalanceOfAccount(bob) == 1337
		})
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
	alice.WaitUntilBalance(t, 1000000)

	// Alice attempt to pay Bob more than what
	// she has in her wallet
	assert.NoError(t, txError(alice.Pay(bob, 1000001)))
	alice.WaitUntilConsensus(t)

	// Alice should have paid for gas even though the tx failed
	waitFor(t, func() bool {
		return alice.Balance() > 0 && alice.Balance() < 1000000
	})

	// Bob should not receive the tx amount
	assert.EqualValues(t, 0, bob.Balance())

	// Everyone else should see the updated balance of Alice and Bob
	for _, node := range testnet.Nodes() {
		waitFor(t, func() bool {
			return node.BalanceOfAccount(alice) == alice.Balance() &&
				node.BalanceOfAccount(bob) == 0
		})
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
	alice.WaitUntilBalance(t, 1000000)

	assert.NoError(t, txError(alice.PlaceStake(9001)))
	alice.WaitUntilStake(t, 9001)

	// Alice balance should be balance-stakeAmount-gas
	aliceBalance := alice.Balance()
	waitFor(t, func() bool { return aliceBalance < 1000000-9001 })

	// Everyone else should see the updated balance of Alice
	for _, node := range testnet.Nodes() {
		waitFor(t, func() bool {
			return node.BalanceOfAccount(alice) == alice.Balance() &&
				node.StakeOfAccount(alice) == alice.Stake()
		})
	}

	assert.NoError(t, txError(alice.WithdrawStake(5000)))
	alice.WaitUntilStake(t, 4001)

	// Withdrawn stake should be added to balance
	oldBalance := aliceBalance
	aliceBalance = alice.Balance()
	waitFor(t, func() bool { return aliceBalance > oldBalance })

	// Everyone else should see the updated balance of Alice
	for _, node := range testnet.Nodes() {
		waitFor(t, func() bool {
			return node.BalanceOfAccount(alice) == alice.Balance() &&
				node.StakeOfAccount(alice) == alice.Stake()
		})
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
	alice.WaitUntilBalance(t, 1000000)

	contract, err := alice.SpawnContract("testdata/transfer_back.wasm",
		10000, nil)
	assert.NoError(t, err)

	alice.WaitUntilConsensus(t)

	// Calling the contract should cause the contract to send back 250000 PERL back to alice
	_, err = alice.CallContract(contract.ID, 500000, 100000, "on_money_received", contract.ID[:])
	assert.NoError(t, err)

	waitFor(t, func() bool { return alice.Balance() > 700000 })
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
	alice.WaitUntilBalance(t, 1000000)

	contract, err := alice.SpawnContract("testdata/transfer_back.wasm",
		10000, nil)
	assert.NoError(t, err)

	alice.WaitUntilConsensus(t)

	assert.NoError(t, txError(alice.DepositGas(contract.ID, 654321)))
	waitFor(t, func() bool { return alice.GasBalanceOfAddress(contract.ID) == 654321 })
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
			if ri >= alice.RoundIndex() {
				goto DONE
			}
		}
	}

DONE:
	waitFor(t, func() bool { return charlie.BalanceOfAccount(alice) == alice.Balance() })
}

func TestLedger_SpamContracts(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t)
	testnet.AddNode(t)

	assert.True(t, <-alice.WaitForSync())

	assert.NoError(t, txError(testnet.Faucet().Pay(alice, 100000)))
	alice.WaitUntilBalance(t, 100000)

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

// func TestLedger_DownloadMissingTx(t *testing.T) {
// 	testnet := NewTestNetwork(t)
// 	defer testnet.Cleanup()
//
// 	nodes := 3
// 	initialRounds := 70
// 	// postLeaveRounds := 10
//
// 	for i := 0; i < nodes-1; i++ {
// 		testnet.AddNode(t)
// 	}
//
// 	alice := testnet.Faucet()
// 	stop := make(chan struct{})
// 	go func() {
// 		for {
// 			select {
// 			case <-stop:
// 				return
//
// 			default:
// 				assert.NoError(t, txError(alice.Benchmark()))
// 			}
// 		}
// 	}()
//
// 	var info *FinalizeRoundResult
//
// 	for {
// 		alice.WaitUntilConsensus(t)
//
// 		waitFor(t, func() bool {
// 			info = alice.ledger.FinalizeRoundInfo(alice.RoundIndex())
// 			return info != nil
// 		})
//
// 		fmt.Printf("consensus round, index=%d, applied=%d, ignored=%d, rejected=%d, missing=%d, incomplete=%d\n",
// 			alice.RoundIndex(), info.AppliedTxCount, info.IgnoredTxCount, info.RejectedTxCount,
// 			alice.ledger.Graph().MissingLen(), alice.ledger.Graph().IncompleteLen())
//
// 		if alice.RoundIndex() >= uint64(initialRounds) {
// 			break
// 		}
// 	}
//
// 	time.Sleep(time.Second * 1)
//
// 	bob := testnet.Nodes()[nodes-1]
// 	bobKey := bob.PrivateKey()
//
// 	fmt.Println("bob left")
//
// 	bob.Cleanup()
// 	testnet.nodes = testnet.nodes[:nodes-1]
//
// 	time.Sleep(time.Second * 1)
//
// 	// for {
// 	// 	alice.WaitUntilConsensus(t)
//
// 	// 	waitFor(t, func() bool {
// 	// 		info = alice.ledger.FinalizeRoundInfo(alice.RoundIndex())
// 	// 		return info != nil
// 	// 	})
//
// 	// 	fmt.Printf("consensus round, index=%d, applied=%d, ignored=%d, rejected=%d, missing=%d, incomplete=%d\n",
// 	// 		alice.RoundIndex(), info.AppliedTxCount, info.IgnoredTxCount, info.RejectedTxCount,
// 	// 		alice.ledger.Graph().MissingLen(), alice.ledger.Graph().IncompleteLen())
//
// 	// 	if alice.RoundIndex() >= uint64(initialRounds+postLeaveRounds) {
// 	// 		break
// 	// 	}
// 	// }
//
// 	time.Sleep(time.Second * 5)
//
// 	fmt.Println("benchmark stopped")
// 	close(stop)
//
// 	bob = testnet.AddNode(t, TestWithWallet(hex.EncodeToString(bobKey[:])))
//
// 	fmt.Printf("bob rejoined, index=%d, missing=%d, incomplete=%d\n",
// 		bob.RoundIndex(), bob.ledger.Graph().MissingLen(), bob.ledger.Graph().IncompleteLen())
//
// 	time.Sleep(time.Second * 10)
//
// 	// log.SetWriter(log.ModuleNode, os.Stdout)
//
// 	// Wait for no more missing txs
// 	waitForDuration(t, func() bool {
// 		fmt.Printf("bob status, index=%d, missing=%d, incomplete=%d\n",
// 			bob.RoundIndex(), bob.ledger.Graph().MissingLen(), bob.ledger.Graph().IncompleteLen())
// 		return bob.ledger.Graph().MissingLen() == 0
// 	}, time.Minute*5)
// }

func TestLedger_DownloadMissingTx(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	// Start with 4 nodes
	for i := 0; i < 3; i++ {
		testnet.AddNode(t)
	}

	nodes := testnet.Nodes()
	alice := nodes[0]
	bob := nodes[1]

	assert.NoError(t, txError(alice.Pay(bob, 100)))
	bob.WaitUntilBalance(t, 100)
	fmt.Println("alice round:", alice.RoundIndex())

	// End of round 1

	log.SetWriter(log.ModuleNode, os.Stdout)
	charlie := testnet.AddNode(t)
	<-charlie.WaitForSync()
	fmt.Println(charlie.RoundIndex())
	fmt.Println(charlie.ledger.SyncStatus())

	assert.NoError(t, txError(alice.Pay(bob, 100)))

	bob.WaitUntilBalance(t, 200)
	fmt.Println("alice round:", alice.RoundIndex())

	charlie.WaitUntilRound(t, 2)
	fmt.Println("charlie round:", charlie.RoundIndex())
}

func txError(tx Transaction, err error) error {
	return err
}
