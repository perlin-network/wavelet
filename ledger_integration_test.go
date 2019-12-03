// +build integration

package wavelet

import (
	"bytes"
	"crypto/rand"
	mrand "math/rand"
	"testing"
	"time"

	cuckoo "github.com/seiflotfy/cuckoofilter"

	"github.com/perlin-network/wavelet/conf"
	"github.com/stretchr/testify/assert"
)

func TestLedger_Pay(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t)
	bob := testnet.AddNode(t)

	testnet.WaitUntilSync(t)

	_, err := testnet.Faucet().Pay(alice, 1000000)
	if !assert.NoError(t, err) {
		return
	}

	alice.WaitUntilBalance(t, 1000000)

	_, err = alice.Pay(bob, 1337)
	if !assert.NoError(t, err) {
		return
	}

	bob.WaitUntilBalance(t, 1337)

	// Alice balance should be balance-txAmount-gas
	waitFor(t, func() bool { return alice.Balance() < 1000000-1337 })

	// Everyone else should see the updated balance of Alice and Bob
	for _, node := range testnet.Nodes() {
		node := node
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

	testnet.WaitUntilSync(t)

	_, err := testnet.Faucet().Pay(alice, 1000000)
	if !assert.NoError(t, err) {
		return
	}

	alice.WaitUntilBalance(t, 1000000)

	// Alice attempt to pay Bob more than what
	// she has in her wallet
	_, err = alice.Pay(bob, 1000001)
	if !assert.NoError(t, err) {
		return
	}

	alice.WaitUntilBlock(t, 2)

	// Alice should have paid for gas even though the tx failed
	waitFor(t, func() bool {
		return alice.Balance() > 0 && alice.Balance() < 1000000
	})

	// Bob should not receive the tx amount
	assert.EqualValues(t, 0, bob.Balance())

	// Everyone else should see the updated balance of Alice and Bob
	for _, node := range testnet.Nodes() {
		node := node
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
	testnet.AddNode(t) // bob

	testnet.WaitUntilSync(t)

	_, err := testnet.Faucet().Pay(alice, 1000000)
	if !assert.NoError(t, err) {
		return
	}

	alice.WaitUntilBalance(t, 1000000)

	_, err = alice.PlaceStake(9001)
	if !assert.NoError(t, err) {
		return
	}

	alice.WaitUntilStake(t, 9001)

	// Alice balance should be balance-stakeAmount-gas
	waitFor(t, func() bool { return alice.Balance() < 1000000-9001 })

	// Everyone else should see the updated balance of Alice
	for _, node := range testnet.Nodes() {
		node := node
		waitFor(t, func() bool {
			return node.BalanceOfAccount(alice) == alice.Balance() &&
				node.StakeOfAccount(alice) == alice.Stake()
		})
	}

	oldBalance := alice.Balance()

	_, err = alice.WithdrawStake(5000)
	if !assert.NoError(t, err) {
		return
	}

	alice.WaitUntilStake(t, 4001)

	// Withdrawn stake should be added to balance
	waitFor(t, func() bool { return alice.Balance() > oldBalance })

	// Everyone else should see the updated balance of Alice
	for _, node := range testnet.Nodes() {
		node := node
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
	testnet.AddNode(t) // bob

	testnet.WaitUntilSync(t)

	_, err := testnet.Faucet().Pay(alice, 1000000)
	if !assert.NoError(t, err) {
		return
	}

	alice.WaitUntilBalance(t, 1000000)

	contract, err := alice.SpawnContract("testdata/transfer_back.wasm",
		10000, nil)
	assert.NoError(t, err)

	alice.WaitUntilBlock(t, 2)

	// Calling the contract should cause the contract to send back 250000 PERL back to alice
	_, err = alice.CallContract(
		contract.ID, 500000, 100000, "on_money_received", contract.ID[:],
	)
	if !assert.NoError(t, err) {
		return
	}

	waitFor(t, func() bool { return alice.Balance() > 700000 })
}

func TestLedger_DepositGas(t *testing.T) {
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t)
	testnet.AddNode(t) // bob

	testnet.WaitUntilSync(t)

	_, err := testnet.Faucet().Pay(alice, 1000000)
	if !assert.NoError(t, err) {
		return
	}

	alice.WaitUntilBalance(t, 1000000)

	contract, err := alice.SpawnContract("testdata/transfer_back.wasm",
		10000, nil)
	assert.NoError(t, err)

	alice.WaitUntilBlock(t, 2)

	_, err = alice.DepositGas(contract.ID, 654321)
	if !assert.NoError(t, err) {
		return
	}

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
		testnet.AddNode(t)
	}

	testnet.WaitUntilSync(t)

	// Advance the network by a few blocks larger than sys.SyncIfBlockIndicesDifferBy
	for i := 0; i < int(conf.GetSyncIfBlockIndicesDifferBy())+1; i++ {
		_, err := alice.PlaceStake(1)
		if !assert.NoError(t, err) {
			return
		}

		alice.WaitUntilBlock(t, uint64(i+1))
	}

	testnet.WaitForBlock(t, alice.BlockIndex())

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
	charlie := testnet.AddNode(t)

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
	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t)
	bob := testnet.AddNode(t)

	testnet.WaitUntilSync(t)

	tx, err := testnet.Faucet().Pay(alice, 1000000)
	if !assert.NoError(t, err) {
		return
	}

	testnet.WaitForConsensus(t)

	assert.NotNil(t, bob.ledger.transactions.Find(tx.ID))

	bobDBPath := bob.DBPath()
	bob.Leave(false)

	bob = testnet.AddNode(t, WithRemoveExistingDB(false), WithDBPath(bobDBPath))

	bob.WaitUntilSync(t)

	assert.NotNil(t, bob.ledger.transactions.Find(tx.ID))
}

func benchBloom(n int, b *testing.B) {
	bf := cuckoo.NewFilter(conf.GetBloomFilterM())

	var txID TransactionID
	for i := 0; i < n; i++ {
		if _, err := rand.Read(txID[:]); err != nil {
			b.Fatal(err)
		}
		bf.InsertUnique(txID[:])
	}

	for i := 0; i < b.N; i++ {
		if _, err := rand.Read(txID[:]); err != nil {
			b.Fatal(err)
		}
		bf.Lookup(txID[:])
	}
}

func BenchmarkBloom10K(b *testing.B)  { benchBloom(10000, b) }
func BenchmarkBloom100K(b *testing.B) { benchBloom(100000, b) }
func BenchmarkBloom1M(b *testing.B)   { benchBloom(1000000, b) }

//BenchmarkBloom10K-12     	 4642104	       261 ns/op
//BenchmarkBloom100K-12    	 4711498	       238 ns/op
//BenchmarkBloom1M-12      	 4065445	       262 ns/op
