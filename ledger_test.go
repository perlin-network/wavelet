package wavelet

import (
	"bytes"
	"math/rand"
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
	waitFor(t, func() bool { return alice.Balance() < 1000000-1337 })

	// Everyone else should see the updated balance of Alice and Bob
	for _, node := range testnet.Nodes() {
		waitFor(t, func() bool {
			return node.BalanceOfAccount(alice) == alice.Balance() &&
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
	waitFor(t, func() bool { return alice.Balance() < 1000000-9001 })

	// Everyone else should see the updated balance of Alice
	for _, node := range testnet.Nodes() {
		waitFor(t, func() bool {
			return node.BalanceOfAccount(alice) == alice.Balance() &&
				node.StakeOfAccount(alice) == alice.Stake()
		})
	}

	oldBalance := alice.Balance()

	assert.NoError(t, txError(alice.WithdrawStake(5000)))
	alice.WaitUntilStake(t, 4001)

	// Withdrawn stake should be added to balance
	waitFor(t, func() bool { return alice.Balance() > oldBalance })

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

type account struct {
	PublicKey [32]byte
	Balance   uint64
	Stake     uint64
	Reward    uint64
}

func TestLedger_Sync(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	rand.Seed(time.Now().UnixNano())

	var code [1024 * 1024]byte
	if _, err := rand.Read(code[:]); err != nil {
		t.Fatal(err)
	}

	// Generate accounts
	accounts := make([]account, 500)
	for i := 0; i < len(accounts); i++ {
		// Use random keys to speed up generation
		var key [32]byte
		if _, err := rand.Read(key[:]); err != nil {
			t.Fatal(err)
		}

		accounts[i] = account{
			PublicKey: key,
			Balance:   rand.Uint64(),
			Stake:     rand.Uint64(),
			Reward:    rand.Uint64(),
		}
	}

	// Setup network with 3 nodes
	alice := testnet.Faucet()
	for i := 0; i < 2; i++ {
		testnet.AddNode(t)
	}

	testnet.WaitForSync(t)

	// Advance the network by a few rounds larger than sys.SyncIfRoundsDifferBy
	for i := 0; i < int(conf.GetSyncIfRoundsDifferBy())+1; i++ {
		assert.NoError(t, txError(alice.PlaceStake(1)))
		alice.WaitUntilConsensus(t)
	}

	testnet.WaitForRound(t, alice.RoundIndex())

	snapshot := testnet.Nodes()[0].ledger.accounts.Snapshot()

	snapshot.SetViewID(alice.RoundIndex() + 1)
	for _, acc := range accounts {
		WriteAccountBalance(snapshot, acc.PublicKey, acc.Balance)
		WriteAccountStake(snapshot, acc.PublicKey, acc.Stake)
		WriteAccountReward(snapshot, acc.PublicKey, acc.Reward)
		WriteAccountContractCode(snapshot, acc.PublicKey, code[:])
	}

	// Override ledger state of all nodes
	for _, node := range testnet.Nodes() {
		err := node.ledger.accounts.Commit(snapshot)
		if err != nil {
			t.Fatal(err)
		}

		// Override latest round merkle with the new state snapshot
		// TODO: this is causing data race
		round := node.ledger.rounds.Latest()
		round.Merkle = snapshot.Checksum()
	}

	// When a new node joins the network, it will eventually
	// sync with the other nodes
	// log.SetWriter(log.ModuleNode, os.Stdout)
	charlie := testnet.AddNode(t)

	timeout := time.NewTimer(time.Second * 300)
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
	for _, acc := range accounts {
		assert.EqualValues(t, acc.Balance, charlie.BalanceWithPublicKey(acc.PublicKey))
		assert.EqualValues(t, acc.Stake, charlie.StakeWithPublicKey(acc.PublicKey))
		assert.EqualValues(t, acc.Reward, charlie.RewardWithPublicKey(acc.PublicKey))

		checkCode, _ := ReadAccountContractCode(charlie.ledger.accounts.Snapshot(), acc.PublicKey)
		assert.True(t, bytes.Equal(code[:], checkCode))
	}
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

func txError(tx Transaction, err error) error {
	return err
}
