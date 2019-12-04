// +build integration

package wavelet

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
)

const (
	testDumpDir = "testDumpIncludingContract"
)

var testRestoreDir = "testdata/testgenesis"

func getGenesisTestNetwork(t testing.TB, withContract bool) (testnet *TestNetwork, alice *TestLedger, cleanup func()) {
	testnet = NewTestNetwork(t)

	cleanup = func() {
		testnet.Cleanup()
	}

	alice = testnet.AddNode(t)
	bob := testnet.AddNode(t)

	testnet.WaitUntilSync(t)

	var err error

	_, err = testnet.faucet.Pay(alice, 10000000000)
	assert.NoError(t, err)
	alice.WaitUntilBalance(t, 10000000000)

	_, err = testnet.faucet.Pay(bob, 10000000000)
	assert.NoError(t, err)
	bob.WaitUntilBalance(t, 10000000000)

	_, err = alice.PlaceStake(100)
	assert.NoError(t, err)
	alice.WaitUntilStake(t, 100)

	_, err = bob.PlaceStake(100)
	assert.NoError(t, err)
	bob.WaitUntilStake(t, 100)

	block := alice.BlockIndex()

	if withContract {
		for i := 0; i < 4; i++ {
			tx, err := alice.SpawnContract("testdata/transfer_back.wasm", 10000, nil)
			if !assert.NoError(t, err) {
				return nil, nil, cleanup
			}
			block++
			alice.WaitUntilBlock(t, block)

			if i%2 == 0 {
				tx, err = alice.DepositGas(tx.ID, (uint64(i)+1)*100)
				if !assert.NoError(t, err) {
					return nil, nil, cleanup
				}
				block++
				alice.WaitUntilBlock(t, block)
			}
		}
	}

	return testnet, alice, cleanup
}

func TestDumpIncludingContract(t *testing.T) {
	testnet, target, cleanup := getGenesisTestNetwork(t, true)
	defer cleanup()
	if testnet == nil || target == nil {
		assert.FailNow(t, "failed to get test network.")
		return
	}
	defer func() {
		_ = os.RemoveAll(testDumpDir)
	}()

	// Delete the dir in case it already exists
	assert.NoError(t, os.RemoveAll(testDumpDir))

	expected := target.ledger.Snapshot()
	if !assert.NoError(t, Dump(expected, testDumpDir, true, false)) {
		return
	}

	fmt.Println("checkdump")
	testDump(t, testDumpDir, expected, true)
}

func TestDumpWithoutContract(t *testing.T) {
	testDumpDir := "testDumpIncludingContract"

	testnet, target, cleanup := getGenesisTestNetwork(t, false)
	defer cleanup()
	if testnet == nil || target == nil {
		assert.FailNow(t, "failed setup test network.")
		return
	}
	defer func() {
		_ = os.RemoveAll(testDumpDir)
	}()

	// Delete the dir in case it already exists
	assert.NoError(t, os.RemoveAll(testDumpDir))

	expected := target.ledger.Snapshot()
	if !assert.NoError(t, Dump(expected, testDumpDir, false, false)) {
		return
	}

	testDump(t, testDumpDir, expected, false)
}

// Restore the dump into a tree and compare the tree against the provided expected tree.
func testDump(t *testing.T, dumpDir string, expected *avl.Tree, checkContract bool) {
	actual := avl.New(store.NewInmem())
	_ = performInception(actual, &dumpDir)

	// Compare the expected tree against actual tree
	compareTree(t, expected, actual, checkContract)
	// Reverse compare, to check actual tree for keys/values that don't exist in the expected tree.
	compareTree(t, actual, expected, checkContract)

	checkRestoredDefaults(t, actual)

	// Repeatedly restore the dump and check it's checksum to make sure there's no randomness in the order of the restoration.
	var checksum = actual.Checksum()
	for i := 0; i < 30; i++ {
		tree := avl.New(store.NewInmem())
		_ = performInception(tree, &dumpDir)

		assert.Equal(t, checksum, tree.Checksum())
	}
}

func compareTree(t *testing.T, expected *avl.Tree, actual *avl.Tree, checkContract bool) {
	expected.Iterate(func(key, value []byte) {
		var globalPrefix [1]byte
		copy(globalPrefix[:], key)

		if globalPrefix != keyAccounts {
			return
		}

		var accountPrefix [1]byte
		copy(accountPrefix[:], key[1:])

		// Since contract can also have balance, we ignore contract account balance if we don't dump contracts.
		// This is because, in case of we don't contracts, the restored tree does not contain the contract account balance, but the original tree does.
		if !checkContract && accountPrefix == keyAccountBalance {
			var id AccountID
			copy(id[:], key[2:])

			if _, isContract := ReadAccountContractCode(expected, id); isContract {
				return
			}
		}

		var cond1, cond2 bool

		cond1 = accountPrefix == keyAccountBalance ||
			accountPrefix == keyAccountStake ||
			accountPrefix == keyAccountReward

		if checkContract {
			cond2 = accountPrefix == keyAccountContractCode ||
				accountPrefix == keyAccountContractNumPages ||
				accountPrefix == keyAccountContractPages ||
				accountPrefix == keyAccountContractGasBalance ||
				accountPrefix == keyAccountContractGlobals
		}

		if !(cond1 || cond2) {
			return
		}

		val, exist := actual.Lookup(key)
		if !exist {
			assert.Failf(t, "missing value", "key %x", key)
		}

		if !bytes.Equal(value, val) {
			assert.Failf(t, "value mismatch", "key %x, expected: %x, actual: %x", key, value, val)
		}
	})
}

func TestPerformInception(t *testing.T) {
	t.Parallel()

	tree := avl.New(store.NewInmem())
	block := performInception(tree, &testRestoreDir)

	assert.Equal(t, uint64(0), block.Index)
	assert.Nil(t, block.Transactions)
	assert.Equal(t, "3a4598625c7fcf107257c648c9f289da", fmt.Sprintf("%x", block.Merkle))

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
		uint64p(9999999979999996632), uint64p(5000000), nil)

	checkAccount(t, tree, id("696937c2c8df35dba0169de72990b80761e51dd9e2411fa1fce147f68ade830a"),
		uint64p(10000000000000000000), nil, nil)

	checkAccount(t, tree, id("f03bb6f98c4dfd31f3d448c7ec79fa3eaa92250112ada43471812f4b1ace6467"),
		uint64p(10000000000000000000), nil, nil)

	checkAccount(t, tree, id("d1798fa00253482cd66e9c45009db573650664604f38f2cc232fe4ddc08e2a19"),
		uint64p(9999964893), nil, uint64p(100))

	checkAccount(t, tree, id("b71038be4f2e09a5199bfb6fb99c1fca663997db851cb88fb5f5a2340790da2c"),
		uint64p(9999999572), nil, uint64p(100))

	var contractID TransactionID

	contractID = id("294b4ee8614d4a2c914154b2f112e2c8d899ffcf2a890202d2cbc224db87c64e")
	checkContract(t, tree, contractID,
		filepath.Join(testRestoreDir, fmt.Sprintf("%x.wasm", contractID)), 100,
		[]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}, []int{15, 16, 17},
	)

	contractID = id("4cd1808ad6c62dc96fc22c21dc9ba97a149642651b76a65c5a1ad789b5fb7d0a")
	checkContract(t, tree, contractID,
		filepath.Join(testRestoreDir, fmt.Sprintf("%x.wasm", contractID)), 300,
		[]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}, []int{15, 16, 17},
	)

	contractID = id("b11581647113dc928c02cb148ff8a4030b7c3468fc1d27f4b4ea89c9caec300d")
	checkContract(t, tree, contractID,
		filepath.Join(testRestoreDir, fmt.Sprintf("%x.wasm", contractID)), 500,
		[]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}, []int{15, 16, 17},
	)

	contractID = id("c1ae186b7a079dfb6a7db8ffdae487a39dbcd11e8d5da8dfeb7208a2e463a111")
	checkContract(t, tree, contractID,
		filepath.Join(testRestoreDir, fmt.Sprintf("%x.wasm", contractID)), 400,
		[]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}, []int{15, 16, 17},
	)

	contractID = id("c8f1bbfb4ed2952adea735e95327e144e5660b9de066110996ae1db34206a36f")
	checkContract(t, tree, contractID,
		filepath.Join(testRestoreDir, fmt.Sprintf("%x.wasm", contractID)), 200,
		[]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}, []int{15, 16, 17},
	)

	assert.Equal(t, uint64(5), ReadAccountsLen(tree))

	checkRestoredDefaults(t, tree)
}

const expectedPageNum = 18

// Check the expected values of a contract against the contract's values in the tree.
// Also, compare the contract's code in the tree against the actual contract code file.
func checkContract(t *testing.T, tree *avl.Tree, id TransactionID, codeFilePath string, expectedGasBalance uint64, expectedEmptyMemPages []int, expectedNotEmptyMemPages []int) {
	code, exist := ReadAccountContractCode(tree, id)
	assert.True(t, exist, "contract ID: %x", id)
	assert.NotEmpty(t, code, "contract ID: %x", id)

	gasBalance, exist := ReadAccountContractGasBalance(tree, id)
	assert.True(t, exist, "contract ID: %x", id)
	assert.Equal(t, expectedGasBalance, gasBalance, "contract ID: %x", id)

	expectedCode, err := ioutil.ReadFile(codeFilePath)
	assert.NoError(t, err)
	assert.EqualValues(t, expectedCode, code, "contract ID: %x, filepath: %s", id, codeFilePath)

	numPages, exist := ReadAccountContractNumPages(tree, id)
	assert.True(t, exist, "contract ID: %x", id)
	assert.EqualValues(t, expectedPageNum, numPages, "contract ID: %x", id)

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
func checkRestoredDefaults(t *testing.T, tree *avl.Tree) {
	val, exist := tree.Lookup(keyRewardWithdrawals[:])

	assert.False(t, exist)
	assert.Nil(t, val)
}

func BenchmarkDump(b *testing.B) {
	testDumpDir := "testDumpIncludingContract"

	testnet, target, cleanup := getGenesisTestNetwork(b, true)
	defer cleanup()
	if testnet == nil || target == nil {
		assert.FailNow(b, "failed setup test network.")
		return
	}
	defer func() {
		_ = os.RemoveAll(testDumpDir)
	}()

	b.ResetTimer()
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		b.StopTimer()

		assert.NoError(b, os.RemoveAll(testDumpDir))

		b.StartTimer()
		assert.NoError(b, Dump(target.ledger.Snapshot(), testDumpDir, true, false))
	}
}

func BenchmarkPerformInception(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		b.StopTimer()
		tree := avl.New(store.NewInmem())

		b.StartTimer()
		performInception(tree, &testRestoreDir)
	}
}
