package wavelet

import (
	"encoding/binary"
	"github.com/golang/snappy"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
	"sync"
)

type accounts struct {
	kv   store.KV
	tree *avl.Tree

	mu sync.RWMutex
}

func newAccounts(kv store.KV) *accounts {
	return &accounts{kv: kv, tree: avl.New(kv)}
}

func (a *accounts) checksum() [avl.MerkleHashSize]byte {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.tree.Checksum()
}

func (a *accounts) snapshot() *avl.Tree {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.tree.Snapshot()
}

func (a *accounts) commit(new *avl.Tree) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if new != nil {
		a.tree = new
	}

	err := a.tree.Commit()
	if err != nil {
		return errors.Wrap(err, "accounts: failed to write")
	}

	//a.tree.GC(0)

	return nil
}

func ReadAccountBalance(tree *avl.Tree, id common.AccountID) (uint64, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountBalance[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func WriteAccountBalance(tree *avl.Tree, id common.AccountID, balance uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], balance)

	logger := log.Account(id, "balance_updated")
	logger.Log().Uint64("balance", balance).Msg("Updated balance.")

	writeUnderAccounts(tree, id, keyAccountBalance[:], buf[:])
}

func ReadAccountStake(tree *avl.Tree, id common.AccountID) (uint64, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountStake[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func WriteAccountStake(tree *avl.Tree, id common.AccountID, stake uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], stake)

	logger := log.Account(id, "stake_updated")
	logger.Log().Uint64("stake", stake).Msg("Updated stake.")

	writeUnderAccounts(tree, id, keyAccountStake[:], buf[:])
}

func ReadAccountContractCode(tree *avl.Tree, id common.TransactionID) ([]byte, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountContractCode[:])
	if !exists || len(buf) == 0 {
		return nil, false
	}

	return buf, true
}

func WriteAccountContractCode(tree *avl.Tree, id common.TransactionID, code []byte) {
	writeUnderAccounts(tree, id, keyAccountContractCode[:], code[:])
}

func ReadAccountContractNumPages(tree *avl.Tree, id common.TransactionID) (uint64, bool) {
	buf, exists := readUnderAccounts(tree, id, keyAccountContractNumPages[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func WriteAccountContractNumPages(tree *avl.Tree, id common.TransactionID, numPages uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], numPages)

	logger := log.Account(id, "num_pages_updated")
	logger.Log().Uint64("num_pages", numPages).Msg("Updated number of memory pages for a contract.")

	writeUnderAccounts(tree, id, keyAccountContractNumPages[:], buf[:])
}

func ReadAccountContractPage(tree *avl.Tree, id common.TransactionID, idx uint64) ([]byte, bool) {
	var idxBuf [8]byte
	binary.LittleEndian.PutUint64(idxBuf[:], idx)

	buf, exists := readUnderAccounts(tree, id, append(keyAccountContractPages[:], idxBuf[:]...))
	if !exists || len(buf) == 0 {
		return nil, false
	}

	decoded, err := snappy.Decode(nil, buf)
	if err != nil {
		return nil, false
	}

	return decoded, true
}

func WriteAccountContractPage(tree *avl.Tree, id common.TransactionID, idx uint64, page []byte) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], idx)

	encoded := snappy.Encode(nil, page)

	writeUnderAccounts(tree, id, append(keyAccountContractPages[:], buf[:]...), encoded)
}

func readUnderAccounts(tree *avl.Tree, id common.AccountID, key []byte) ([]byte, bool) {
	buf, exists := tree.Lookup(append(keyAccounts[:], append(key, id[:]...)...))

	if !exists {
		return nil, false
	}

	return buf, true
}

func writeUnderAccounts(tree *avl.Tree, id common.AccountID, key, value []byte) {
	tree.Insert(append(keyAccounts[:], append(key, id[:]...)...), value[:])
}
