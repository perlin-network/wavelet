package wavelet

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestDumps(t *testing.T) {
	dumpDir := "testdata/testdump"

	testnet := NewTestNetwork(t)
	defer testnet.Cleanup()
	defer os.RemoveAll(dumpDir)

	alice := testnet.AddNode(t)
	bob := testnet.AddNode(t)

	assert.True(t, <-alice.WaitForSync())
	assert.True(t, <-bob.WaitForSync())

	var err error

	_, err = testnet.faucet.Pay(alice, 1000000000)
	assert.NoError(t, err)
	for <-alice.WaitForConsensus() {
		if alice.Balance() > 0 {
			break
		}
	}

	_, err = testnet.faucet.Pay(bob, 1000000000)
	assert.NoError(t, err)
	for <-bob.WaitForConsensus() {
		if bob.Balance() > 0 {
			break
		}
	}

	for i := 0; i < 2; i++ {
		_, err := alice.SpawnContract("testdata/transfer_back.wasm", 10000, nil)
		if !assert.NoError(t, err) {
			return
		}
	}

	_, err = bob.Nop()
	assert.NoError(t, err)

	assert.True(t, <-bob.WaitForConsensus())
	assert.True(t, <-alice.WaitForSync())

	// Delete the dir in case it already exists
	assert.NoError(t, os.RemoveAll(dumpDir))

	expected := alice.ledger.Snapshot()

	assert.NoError(t, Dump(expected, dumpDir, true, false))

	// Restore from the dump and then perform various checks.

	actual := avl.New(store.NewInmem())
	_ = performInception(actual, &dumpDir)

	// Check the keys/values in the actual tree against the expected tree.
	compareTree(t, expected, actual)

	// Make sure the actual tree does not contain keys/values that does not exist on the expected tree.
	compareTree(t, actual, expected)

	checkRestoredDefaults(t, actual)
}

// For each key in the expected tree, we compare it's existence and value against the actual tree.
// Note that not all keys are dumped and restored.
func compareTree(t *testing.T, expected *avl.Tree, actual *avl.Tree) {
	expected.Iterate(func(key, value []byte) {
		// Not all keys are dumped and restored.
		// So, we indicate if the key should be checked.
		var check = false

		var globalPrefix [1]byte
		copy(globalPrefix[:], key[:])

		if globalPrefix == keyAccounts {
			var accountPrefix [1]byte
			copy(accountPrefix[:], key[1:])

			var id AccountID
			copy(id[:], key[2:])

			// Explicitly list of the account prefixes that are dumped and restored.

			check = accountPrefix == keyAccountBalance ||
				accountPrefix == keyAccountStake ||
				accountPrefix == keyAccountReward ||
				accountPrefix == keyAccountContractCode ||
				accountPrefix == keyAccountContractNumPages ||
				accountPrefix == keyAccountContractPages ||
				accountPrefix == keyAccountContractGasBalance ||
				accountPrefix == keyAccountContractGlobals
		} else {
			// In the global prefixes, only keyAccountsLen is dumped and restored.
			check = globalPrefix == keyAccountsLen
		}

		if !check {
			return
		}

		actualValue, actualExist := actual.Lookup(key)
		if !actualExist {
			t.Errorf("key %x, missing value", key)
		}

		if !bytes.Equal(value, actualValue) {
			t.Errorf("key %x, expected: %x, actual: %x", key, value, actualValue)
		}
	})
}

func TestPerformInception(t *testing.T) {
	var testGenesisDir = "testdata/testgenesis"

	tree := avl.New(store.NewInmem())
	round := performInception(tree, &testGenesisDir)

	assert.Equal(t, uint64(0), round.Index)
	assert.Equal(t, uint64(0), round.Applied)
	assert.Equal(t, Transaction{}, round.Start)

	tx := Transaction{}
	tx.rehash()
	assert.Equal(t, tx, round.End)

	uint64p := func(v uint64) *uint64 {
		return &v
	}

	id := func(id string) AccountID {
		var accountID AccountID

		if n, err := hex.Decode(accountID[:], []byte(id)); n != cap(accountID) || err != nil {
			assert.Fail(t, "invalid account ID")
		}

		return accountID
	}

	checkAccount(t, tree, id("400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405"),
		uint64p(9999999999999997557), uint64p(5000000), nil)

	checkAccount(t, tree, id("696937c2c8df35dba0169de72990b80761e51dd9e2411fa1fce147f68ade830a"),
		uint64p(10000000000000000100), nil, nil)

	checkAccount(t, tree, id("f03bb6f98c4dfd31f3d448c7ec79fa3eaa92250112ada43471812f4b1ace6467"),
		uint64p(10000000000000000000), nil, nil)

	contractID := id("ca0e12024ed83dfd66fb48648d3853c68a31259b2df720dc709fb046e5de2b6e")

	checkContract(t, tree, contractID,
		filepath.Join(testGenesisDir, fmt.Sprintf("%x.wasm", contractID)),
		18, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}, []int{15, 16},
	)

	assert.Equal(t, uint64(3), ReadAccountsLen(tree))

	checkRestoredDefaults(t, tree)
}

// Check the expected values of a contract against the contract's values in the tree.
// Also, compare the contract's code in the tree against the actual contract code file.
func checkContract(t *testing.T, tree *avl.Tree, id TransactionID, codeFilePath string, expectedPageNum uint64, expectedEmptyMemPages []int, expectedNotEmptyMemPages []int) {
	code, exist := ReadAccountContractCode(tree, id)
	assert.True(t, exist, "contract ID: %x", id)
	assert.NotEmpty(t, code, "contract ID: %x", id)

	expectedCode, err := ioutil.ReadFile(codeFilePath)
	assert.NoError(t, err)
	assert.EqualValues(t, expectedCode, code, "contract ID: %x, filepath: %s", id, codeFilePath)

	numPages, exist := ReadAccountContractNumPages(tree, id)
	assert.True(t, exist, "contract ID: %x", id)
	assert.Equal(t, expectedPageNum, numPages, "contract ID: %x", id)

	for _, v := range expectedEmptyMemPages {
		page, exist := ReadAccountContractPage(tree, id, uint64(v))
		assert.False(t, exist)
		assert.Empty(t, page)
	}

	for _, v := range expectedNotEmptyMemPages {
		page, exist := ReadAccountContractPage(tree, id, uint64(v))
		assert.True(t, exist)
		assert.NotEmpty(t, page)
		assert.Len(t, page, PageSize)
	}
}

// Check the expected values of a account against the account's values in the tree.
// If an expected value is nil, we check that it must not exists in the tree.
func checkAccount(t *testing.T, tree *avl.Tree, id AccountID, expectedBalance, expectedReward, expectedStake *uint64) {
	var balance, reward, stake uint64
	var exist bool

	balance, exist = ReadAccountBalance(tree, id)
	assert.Equal(t, expectedBalance != nil, exist, "account ID: %x", id)
	reward, exist = ReadAccountReward(tree, id)
	assert.Equal(t, expectedReward != nil, exist, "account ID: %x", id)
	stake, exist = ReadAccountStake(tree, id)
	assert.Equal(t, expectedStake != nil, exist, "account ID: %x", id)

	if expectedBalance != nil {
		assert.Equal(t, balance, *expectedBalance, "account ID: %x", id)
	}

	if expectedReward != nil {
		assert.Equal(t, reward, *expectedReward, "account ID: %x", id)
	}

	if expectedStake != nil {
		assert.Equal(t, stake, *expectedStake, "account ID: %x", id)
	}
}

// Used to check the restored tree to make sure some of the global prefixes must not exist.
// Also, check all the accounts' nonce have value 1.
func checkRestoredDefaults(t *testing.T, tree *avl.Tree) {
	// Check for global prefixes that must not exists.

	var val []byte
	var exist bool

	val, exist = tree.Lookup(keyRounds[:])
	assert.False(t, exist)
	assert.Nil(t, val)

	val, exist = tree.Lookup(keyRoundLatestIx[:])
	assert.False(t, exist)
	assert.Nil(t, val)

	val, exist = tree.Lookup(keyRoundOldestIx[:])
	assert.False(t, exist)
	assert.Nil(t, val)

	val, exist = tree.Lookup(keyRoundStoredCount[:])
	assert.False(t, exist)
	assert.Nil(t, val)

	val, exist = tree.Lookup(keyRewardWithdrawals[:])
	assert.False(t, exist, )
	assert.Nil(t, val)

	// Check all the account nonce values must be 1.

	tree.IteratePrefix(append(keyAccounts[:], keyAccountNonce[:]...), func(key, value []byte) {
		var id AccountID
		copy(id[:], key[2:])

		if _, isContract := ReadAccountContractCode(tree, id); isContract {
			return
		}

		nonce, exist := ReadAccountNonce(tree, id)
		assert.True(t, exist, "account %x is missing nonce", id)
		assert.Equal(t, uint64(1), nonce, "account %x, expected nonce is 1", id)
	})
}
