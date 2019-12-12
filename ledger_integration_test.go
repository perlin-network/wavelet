// +build integration

package wavelet

import (
	"bytes"
	"crypto/rand"
	mrand "math/rand"
	"testing"
	"time"

	"github.com/perlin-network/wavelet/conf"
	"github.com/stretchr/testify/assert"
)

func TestLedger_Pay(t *testing.T) {
	testnet, err := NewTestNetwork()
	FailTest(t, err)

	defer testnet.Cleanup()

	alice, err := testnet.AddNode()
	FailTest(t, err)

	bob, err := testnet.AddNode()
	FailTest(t, err)

	FailTest(t, testnet.WaitUntilSync())

	_, err = testnet.Faucet().Pay(alice, 1000000)
	FailTest(t, err)

	FailTest(t, alice.WaitUntilBalance(1000000))

	_, err = alice.Pay(bob, 1337)
	FailTest(t, err)

	FailTest(t, bob.WaitUntilBalance(1337))

	// Alice balance should be balance-txAmount-gas
	err = waitFor(func() bool {
		return alice.Balance() < 1000000-1337
	})
	FailTest(t, err)

	// Everyone else should see the updated balance of Alice and Bob
	for _, node := range testnet.Nodes() {
		node := node
		err = waitFor(func() bool {
			return node.BalanceOfAccount(alice) == alice.Balance() &&
				node.BalanceOfAccount(bob) == 1337
		})

		assert.NoError(t, err)
	}
}

func TestLedger_Stake(t *testing.T) {
	testnet, err := NewTestNetwork()
	FailTest(t, err)

	defer testnet.Cleanup()

	alice, err := testnet.AddNode()
	FailTest(t, err)

	_, err = testnet.AddNode() // bob
	FailTest(t, err)

	FailTest(t, testnet.WaitUntilSync())

	_, err = testnet.Faucet().Pay(alice, 1000000)
	FailTest(t, err)

	FailTest(t, alice.WaitUntilBalance(1000000))

	_, err = alice.PlaceStake(9001)
	FailTest(t, err)

	FailTest(t, alice.WaitUntilStake(9001))

	// Alice balance should be balance-stakeAmount-gas
	err = waitFor(func() bool { return alice.Balance() < 1000000-9001 })
	FailTest(t, err)

	// Everyone else should see the updated balance of Alice
	for _, node := range testnet.Nodes() {
		node := node
		err = waitFor(func() bool {
			return node.BalanceOfAccount(alice) == alice.Balance() &&
				node.StakeOfAccount(alice) == alice.Stake()
		})

		assert.NoError(t, err)
	}

	oldBalance := alice.Balance()

	_, err = alice.WithdrawStake(5000)
	if !assert.NoError(t, err) {
		return
	}

	FailTest(t, alice.WaitUntilStake(4001))

	// Withdrawn stake should be added to balance
	err = waitFor(func() bool { return alice.Balance() > oldBalance })
	FailTest(t, err)

	// Everyone else should see the updated balance of Alice
	for _, node := range testnet.Nodes() {
		node := node
		err = waitFor(func() bool {
			return node.BalanceOfAccount(alice) == alice.Balance() &&
				node.StakeOfAccount(alice) == alice.Stake()
		})

		assert.NoError(t, err)
	}
}

func TestLedger_CallContract(t *testing.T) {
	testnet, err := NewTestNetwork()
	FailTest(t, err)

	defer testnet.Cleanup()

	alice, err := testnet.AddNode()
	FailTest(t, err)

	_, err = testnet.AddNode() // bob
	FailTest(t, err)

	FailTest(t, testnet.WaitUntilSync())

	_, err = testnet.Faucet().Pay(alice, 1000000)
	FailTest(t, err)

	FailTest(t, alice.WaitUntilBalance(1000000))

	contract, err := alice.SpawnContract(
		"testdata/transfer_back.wasm", 10000, nil,
	)
	FailTest(t, err)

	FailTest(t, alice.WaitUntilBlock(2))

	// Calling the contract should cause the contract to send back 250000 PERL back to alice
	_, err = alice.CallContract(
		contract.ID, 500000, 100000, "on_money_received", contract.ID[:],
	)
	FailTest(t, err)

	assert.NoError(t, waitFor(func() bool { return alice.Balance() > 700000 }))
}

func TestLedger_DepositGas(t *testing.T) {
	testnet, err := NewTestNetwork()
	FailTest(t, err)

	defer testnet.Cleanup()

	alice, err := testnet.AddNode()
	FailTest(t, err)

	_, err = testnet.AddNode() // bob
	FailTest(t, err)

	FailTest(t, testnet.WaitUntilSync())

	_, err = testnet.Faucet().Pay(alice, 1000000)
	FailTest(t, err)

	FailTest(t, alice.WaitUntilBalance(1000000))

	contract, err := alice.SpawnContract("testdata/transfer_back.wasm",
		10000, nil)
	FailTest(t, err)

	FailTest(t, alice.WaitUntilBlock(2))

	_, err = alice.DepositGas(contract.ID, 654321)
	FailTest(t, err)

	err = waitFor(func() bool { return alice.GasBalanceOfAddress(contract.ID) == 654321 })
	assert.NoError(t, err)
}

type account struct {
	PublicKey [32]byte
	Balance   uint64
	Stake     uint64
	Reward    uint64
}

func TestLedger_Sync(t *testing.T) {
	testnet, err := NewTestNetwork()
	FailTest(t, err)

	defer testnet.Cleanup()

	mrand.Seed(time.Now().UnixNano())

	var code [1024 * 1024]byte
	if _, err := rand.Read(code[:]); err != nil {
		t.Fatal(err)
	}

	// Generate accounts
	accounts := make([]account, 100)
	for i := 0; i < len(accounts); i++ {
		// Use random keys to speed up generation
		var key [32]byte
		if _, err := rand.Read(key[:]); err != nil {
			t.Fatal(err)
		}

		accounts[i] = account{
			PublicKey: key,
			Balance:   mrand.Uint64(),
			Stake:     mrand.Uint64(),
			Reward:    mrand.Uint64(),
		}
	}

	// Setup network with 3 nodes
	alice := testnet.Faucet()
	for i := 0; i < 2; i++ {
		_, err = testnet.AddNode()
		FailTest(t, err)
	}

	FailTest(t, testnet.WaitUntilSync())

	// Advance the network by a few blocks larger than sys.SyncIfBlockIndicesDifferBy
	for i := 0; i < int(conf.GetSyncIfBlockIndicesDifferBy())+1; i++ {
		_, err := alice.PlaceStake(1)
		if !assert.NoError(t, err) {
			return
		}

		FailTest(t, alice.WaitUntilBlock(uint64(i+1)))
	}

	FailTest(t, testnet.WaitForBlock(alice.BlockIndex()))

	snapshot := testnet.Nodes()[0].ledger.accounts.Snapshot()

	snapshot.SetViewID(alice.BlockIndex() + 1)
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
		block := node.ledger.blocks.Latest()
		block.Merkle = snapshot.Checksum()
	}

	// When a new node joins the network, it will eventually
	// sync with the other nodes
	// log.SetWriter(log.ModuleNode, os.Stdout)
	charlie, err := testnet.AddNode()
	FailTest(t, err)

	timeout := time.NewTimer(time.Second * 1000)
	for {
		select {
		case <-timeout.C:
			t.Fatal("timed out waiting for sync")

		default:
			ri := <-charlie.WaitForBlock(alice.BlockIndex())
			if ri >= alice.BlockIndex() {
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

func TestLedger_LoadTxsOnStart(t *testing.T) {
	testnet, err := NewTestNetwork()
	FailTest(t, err)

	defer testnet.Cleanup()

	alice, err := testnet.AddNode()
	FailTest(t, err)

	bob, err := testnet.AddNode()
	FailTest(t, err)

	FailTest(t, testnet.WaitUntilSync())

	tx, err := testnet.Faucet().Pay(alice, 1000000)
	FailTest(t, err)

	FailTest(t, testnet.WaitForConsensus())

	assert.NotNil(t, bob.ledger.transactions.Find(tx.ID))

	bobDBPath := bob.DBPath()
	bob.Leave(false)

	bob, err = testnet.AddNode(WithRemoveExistingDB(false), WithDBPath(bobDBPath))
	FailTest(t, err)

	FailTest(t, bob.WaitUntilSync())

	assert.NotNil(t, bob.ledger.transactions.Find(tx.ID))
}
