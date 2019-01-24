package wavelet

import (
	"encoding/hex"
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/pem-avl"
)

type Accounts struct {
	store database.Store
	tree  *pem_avl.Tree
}

func (accounts *Accounts) save(account *Account) {
	accounts.store.Put(merge(BucketAccountIDs, account.publicKey), []byte{0x01})

	// Flush changes to persistent AVL trees.
	account.state.Writeback()

	accounts.tree.Insert(account.publicKey, account.GetRoot())
	accounts.tree.Writeback()
}

func (accounts *Accounts) GetAccountRootHash(publicKey []byte) ([]byte, bool) {
	return accounts.tree.Lookup(publicKey)
}

func (accounts *Accounts) GetRoot() []byte {
	return accounts.tree.GetRoot()[:]
}

func (accounts *Accounts) SetRoot(x []byte) {
	root := &[pem_avl.MerkleHashSize]byte{}
	copy(root[:], x)

	accounts.tree.SetRoot(root)
}

func newAccounts(store database.Store) *Accounts {
	return &Accounts{store: store, tree: pem_avl.NewTree(newPrefixedStore(store, BucketAccountRoots))}
}

type Account struct {
	publicKey []byte

	store prefixedStore
	state *pem_avl.Tree
}

// PublicKeyHex returns a hex-encoded string of this accounts public key.
func (a *Account) PublicKeyHex() string {
	return hex.EncodeToString(a.publicKey)
}

func (a *Account) Range(callback func(k string, v []byte)) {
	a.state.Range(func(k []byte, v []byte) {
		callback(string(k), v)
	})
}

func (a *Account) Load(key string) ([]byte, bool) {
	return a.state.Lookup(writeBytes(key))
}

func (a *Account) Store(key string, value []byte) {
	a.state.Insert([]byte(key), value)
}

func (a *Account) Delete(key string) {
	a.state.Delete([]byte(key))
}

func (a *Account) Clone() *Account {
	return &Account{
		state:     a.state.Clone(),
		publicKey: a.publicKey,
	}
}

func (a *Account) GetUint64Field(key string) uint64 {
	v, ok := a.Load(key)
	if !ok {
		return 0
	} else {
		return readUint64(v)
	}
}

func (a *Account) SetUint64Field(key string, value uint64) {
	a.Store(key, writeUint64(value))
}

func (a *Account) GetNonce() uint64 {
	return a.GetUint64Field("nonce")
}

func (a *Account) SetNonce(nonce uint64) {
	a.SetUint64Field("nonce", nonce)
}

func (a *Account) GetBalance() uint64 {
	return a.GetUint64Field("balance")
}

func (a *Account) SetBalance(balance uint64) {
	a.SetUint64Field("balance", balance)
}

func (a *Account) GetStake() uint64 {
	return a.GetUint64Field("stake")
}

func (a *Account) SetStake(stake uint64) {
	a.SetUint64Field("stake", stake)
}

func (a *Account) PublicKey() []byte {
	return a.publicKey
}

func (a *Account) GetRoot() []byte {
	return a.state.GetRoot()[:]
}

func (a *Account) SetRoot(x []byte) {
	root := &[pem_avl.MerkleHashSize]byte{}
	copy(root[:], x)

	a.state.SetRoot(root)
}

// LoadAccount returns or creates a new account object.
func LoadAccount(accounts *Accounts, publicKey []byte) *Account {
	store := newPrefixedStore(accounts.store, merge(BucketAccounts, publicKey))

	account := &Account{
		publicKey: publicKey,
		store:     store,
		state:     pem_avl.NewTree(store),
	}

	if rootHash, ok := accounts.GetAccountRootHash(publicKey); ok {
		account.SetRoot(rootHash)
	}

	return account
}
