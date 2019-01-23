package wavelet

import (
	"encoding/hex"
	"github.com/perlin-network/pem-avl"
)

/*
type KVStore interface {
	Get(key []byte) []byte
	Has(key []byte) bool
	Set(key []byte, value []byte)
	Delete(key []byte)
}
*/

type AccountStore struct {
	Ledger *Ledger

	// PublicKey is the hex-encoded public key
	PublicKey []byte
}

func (s *AccountStore) Get(key []byte) []byte {
	// TODO: Attack?
	ret, _ := s.Ledger.Store.Get(merge(BucketAccounts, s.PublicKey, key))
	return ret
}

func (s *AccountStore) Has(key []byte) bool {
	ok, err := s.Ledger.Store.Has(merge(BucketAccounts, s.PublicKey, key))
	if err != nil {
		panic(err)
	}

	return ok
}

func (s *AccountStore) Set(key []byte, value []byte) {
	err := s.Ledger.Store.Put(merge(BucketAccounts, s.PublicKey, key), value)
	if err != nil {
		panic(err)
	}
}

func (s *AccountStore) Delete(key []byte) {
	err := s.Ledger.Store.Delete(merge(BucketAccounts, s.PublicKey, key))
	if err != nil {
		panic(err)
	}
}

type Account struct {
	tree  *pem_avl.Tree
	store *AccountStore
}

// PublicKeyHex returns a hex-encoded string of this accounts public key.
func (a *Account) PublicKeyHex() string {
	return hex.EncodeToString(a.store.PublicKey)
}

func (a *Account) Range(callback func(k string, v []byte)) {
	a.tree.Range(func(k []byte, v []byte) {
		callback(string(k), v)
	})
}

func (a *Account) Load(key string) ([]byte, bool) {
	return a.tree.Lookup(writeBytes(key))
}

func (a *Account) Store(key string, value []byte) {
	a.tree.Insert([]byte(key), value)
}

func (a *Account) Delete(key string) {
	a.tree.Delete([]byte(key))
}

func (a *Account) Clone() *Account {
	return &Account{
		tree:  a.tree.Clone(),
		store: a.store,
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
	return a.store.PublicKey
}

func (a *Account) Writeback() {
	a.store.Ledger.Store.Put(merge(BucketAccountIDs, a.store.PublicKey), []byte("1"))
	a.tree.Writeback()
	a.store.Ledger.accountList.Insert(a.store.PublicKey, a.GetRoot())
	a.store.Ledger.accountList.Writeback()
}

func (a *Account) GetRoot() []byte {
	return a.tree.GetRoot()[:]
}

func (a *Account) SetRoot(x []byte) {
	root := &[pem_avl.MerkleHashSize]byte{}
	copy(root[:], x)
	a.tree.SetRoot(root)
}

// NewAccount returns or creates a new account object.
func NewAccount(ledger *Ledger, publicKey []byte) *Account {
	store := &AccountStore{
		Ledger:    ledger,
		PublicKey: publicKey,
	}

	acct := &Account{
		store: store,
		tree:  pem_avl.NewTree(store),
	}

	if rh, ok := ledger.accountList.GetAccountRootHash(publicKey); ok {
		acct.SetRoot(rh)
	}

	return acct
}

type AccountListStore struct {
	Ledger *Ledger
}

func (s *AccountListStore) Get(key []byte) []byte {
	ret, _ := s.Ledger.Store.Get(merge(BucketAccountList, key))
	return ret
}

func (s *AccountListStore) Has(key []byte) bool {
	ok, err := s.Ledger.Store.Has(merge(BucketAccountList, key))
	if err != nil {
		panic(err)
	}

	return ok
}

func (s *AccountListStore) Set(key []byte, value []byte) {
	err := s.Ledger.Store.Put(merge(BucketAccountList, key), value)
	if err != nil {
		panic(err)
	}
}

func (s *AccountListStore) Delete(key []byte) {
	err := s.Ledger.Store.Delete(merge(BucketAccountList, key))
	if err != nil {
		panic(err)
	}
}

type AccountList struct {
	tree  *pem_avl.Tree
	store *AccountListStore
}

func (l *AccountList) Writeback() {
	l.tree.Writeback()
}

func (l *AccountList) Insert(publicKey, rootHash []byte) {
	l.tree.Insert(publicKey, rootHash)
}

func (l *AccountList) GetAccountRootHash(publicKey []byte) ([]byte, bool) {
	return l.tree.Lookup(publicKey)
}

func (l *AccountList) GetRoot() []byte {
	return l.tree.GetRoot()[:]
}

func (l *AccountList) SetRoot(x []byte) {
	root := &[pem_avl.MerkleHashSize]byte{}
	copy(root[:], x)
	l.tree.SetRoot(root)
}

func NewAccountList(ledger *Ledger) *AccountList {
	store := &AccountListStore{
		Ledger: ledger,
	}

	acctList := &AccountList{
		tree:  pem_avl.NewTree(store),
		store: store,
	}

	return acctList
}
