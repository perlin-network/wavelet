package wavelet

import (
	"encoding/hex"
	"fmt"
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
	tx, err := testnet.Faucet().PlaceStake(100)
	assert.NoError(t, err)

	// Try to wait for 2 rounds of consensus.
	// The second call should result in timeout, because
	// only 1 round should be finalized.
	assert.True(t, <-alice.WaitForConsensus())
	assert.False(t, <-alice.WaitForConsensus())

	current := alice.ledger.Rounds().Latest().Index
	assert.Equal(t, current-start, uint64(1), "expected only 1 round to be finalized")

	// Adding existing transaction returns no error as the error is ignored
	assert.NoError(t, alice.ledger.AddTransaction(tx))

	for i := uint64(0); i < conf.GetMaxDepthDiff()+1; i++ {
		assert.NoError(t, txError(testnet.Faucet().PlaceStake(1)))
		alice.WaitUntilConsensus(t)
	}

	// Force ledger to prune graph
	alice.ledger.Graph().PruneBelowDepth(alice.ledger.Graph().RootDepth())

	// Adding a tx with depth that is too low returns no error as the error is ignored
	assert.NoError(t, alice.ledger.AddTransaction(tx))
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

func TestLedger_DownloadMissingTx(t *testing.T) {
	conf.Update(conf.WithSyncIfRoundsDifferBy(2))

	testnet := NewTestNetwork(t, WithoutFaucet())
	defer testnet.Cleanup()

	nodes := 4
	initialRounds := 70
	// postLeaveRounds := 10

	var alice, bob *TestLedger
	for i := 0; i < nodes; i++ {
		switch i {
		case 0:
			alice = testnet.AddNode(t,
				WithWallet(FaucetWallet))
			testnet.SetFaucet(alice)

		case 1:
			bob = testnet.AddNode(t, WithKeepExistingDB())

		default:
			testnet.AddNode(t)
		}
	}

	// Give each node enough PERL to run benchmark
	for _, node := range testnet.Nodes() {
		if node.PublicKey() != alice.PublicKey() {
			assert.NoError(t, txError(alice.Pay(node, 100000000)))
			node.WaitUntilConsensus(t)
		}
	}

	startBenchmark := func(n *TestLedger) chan struct{} {
		stop := make(chan struct{})
		go func() {
			for {
				select {
				case <-stop:
					return

				default:
					assert.NoError(t, txError(n.Benchmark(5)))
				}
			}
		}()
		return stop
	}

	// Run benchmark on all nodes
	stops := map[AccountID]chan struct{}{}
	for _, node := range testnet.Nodes() {
		stops[node.PublicKey()] = startBenchmark(node)
	}

	defer func() {
		for _, stop := range stops {
			close(stop)
		}
	}()

	for {
		alice.WaitUntilConsensus(t)
		fmt.Println("consensus round", alice.RoundIndex())
		if alice.RoundIndex() >= uint64(initialRounds) {
			break
		}
	}

	time.Sleep(time.Second * 1)

	fmt.Println("bob left:", bob.DBPath())

	// Make bob leave and stop the benchmark on his node
	close(stops[bob.PublicKey()])
	delete(stops, bob.PublicKey())
	bob.Leave()

	time.Sleep(time.Second * 1)

	bobKey := bob.PrivateKey()
	bob = testnet.AddNode(t,
		WithKeepExistingDB(),
		WithDBPath(bob.DBPath()),
		WithWallet(hex.EncodeToString(bobKey[:])))

	fmt.Printf("bob rejoined, db=%s, index=%d, missing=%d, incomplete=%d\n",
		bob.DBPath(), bob.RoundIndex(), bob.ledger.Graph().MissingLen(), bob.ledger.Graph().IncompleteLen())

	// Wait for no more missing txs
	waitForDuration(t, func() bool {
		fmt.Printf("bob status, index=%d, missing=%d, incomplete=%d\n",
			bob.RoundIndex(), bob.ledger.Graph().MissingLen(), bob.ledger.Graph().IncompleteLen())
		return bob.ledger.Graph().MissingLen() == 0
	}, time.Minute*5)
}

func TestLedger_MinimalSync(t *testing.T) {
	testnet := NewTestNetwork(t, WithoutFaucet())
	defer testnet.Cleanup()

	var alice, bob *TestLedger
	for i := 0; i < 4; i++ {
		switch i {
		case 0:
			alice = testnet.AddNode(t,
				WithWallet(FaucetWallet))
			testnet.SetFaucet(alice)

		case 1:
			bob = testnet.AddNode(t)

		default:
			testnet.AddNode(t)
		}
	}

	assert.NoError(t, txError(alice.Pay(bob, 100)))
	bob.WaitUntilBalance(t, 100)
	fmt.Println("alice round:", alice.RoundIndex())

	// End of round 1

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
