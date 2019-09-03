package wavelet

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"testing"
)

var testGenesisDir = "testdata/testgenesis"

func TestPerformInception(t *testing.T) {
	strp := func(v string) *string {
		return &v
	}
	tree := avl.New(store.NewInmem())
	round := performInception(tree, strp(testGenesisDir))

	assert.Equal(t, uint64(0), round.Index)
	assert.Equal(t, uint64(0), round.Applied)
	assert.Equal(t, Transaction{}, round.Start)

	tx := Transaction{}
	tx.rehash()
	assert.Equal(t, tx, round.End)

	checkAccounts(t, tree)

	checkContract(t, tree)
}

func TestLoadGenesis(t *testing.T) {
	tree := avl.New(store.NewInmem())
	if err := loadGenesisFromDir(tree, testGenesisDir); err != nil {
		t.Fatalf("failed to load genesis: %v", err)
	}

	checkAccounts(t, tree)

	checkContract(t, tree)

	// Test genesis directory does not exist

	var randomFilename = make([]byte, 32)

	_, err := rand.Read(randomFilename)
	assert.NoError(t, err)
	tree = avl.New(store.NewInmem())
	assert.Error(t, loadGenesisFromDir(tree, hex.EncodeToString(randomFilename)))
}

func checkAccounts(t *testing.T, tree *avl.Tree) {
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
}

func checkContract(t *testing.T, tree *avl.Tree) {
	id := func(id string) TransactionID {
		var contractID TransactionID

		if n, err := hex.Decode(contractID[:], []byte(id)); n != cap(contractID) || err != nil {
			assert.Fail(t, "invalid account ID")
		}

		return contractID
	}

	contractID := id("ca0e12024ed83dfd66fb48648d3853c68a31259b2df720dc709fb046e5de2b6e")

	code, exist := ReadAccountContractCode(tree, contractID)
	assert.True(t, exist)
	assert.NotEmpty(t, code)

	numPages, exist := ReadAccountContractNumPages(tree, contractID)
	assert.True(t, exist)
	assert.Equal(t, uint64(18), numPages)

	page0, exist := ReadAccountContractPage(tree, contractID, 0)
	assert.True(t, exist)
	assert.Empty(t, page0)

	page1, exist := ReadAccountContractPage(tree, contractID, 1)
	assert.True(t, exist)
	assert.Empty(t, page1)

	page16, exist := ReadAccountContractPage(tree, contractID, 16)
	assert.True(t, exist)
	assert.NotEmpty(t, page16)
	assert.Len(t, page16, PageSize)
}

func checkAccount(t *testing.T, tree *avl.Tree, accountID AccountID, expectedBalance, expectedReward, expectedStake *uint64) {
	bal, exist := ReadAccountBalance(tree, accountID)
	assert.Equal(t, expectedBalance != nil, exist)
	reward, exist := ReadAccountReward(tree, accountID)
	assert.Equal(t, expectedReward != nil, exist)
	stake, exist := ReadAccountStake(tree, accountID)
	assert.Equal(t, expectedStake != nil, exist)

	if expectedBalance != nil {
		assert.Equal(t, bal, *expectedBalance)
	}

	if expectedReward != nil {
		assert.Equal(t, reward, *expectedReward)
	}

	if expectedStake != nil {
		assert.Equal(t, stake, *expectedStake)
	}
}
