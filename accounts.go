package wavelet

import (
	"encoding/binary"
	"github.com/golang/snappy"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
)

type accounts struct {
	kv   store.KV
	tree *avl.Tree
}

func newAccounts(kv store.KV) accounts {
	return accounts{kv: kv, tree: avl.New(kv)}
}

func (a accounts) Checksum() [avl.MerkleHashSize]byte {
	return a.tree.Checksum()
}

func (a accounts) SnapshotAccounts() accounts {
	return accounts{kv: a.kv, tree: avl.LoadFromSnapshot(a.kv, a.tree.Snapshot())}
}

func (a accounts) ApplyDiff(diff []byte) (accounts, error) {
	err := a.tree.ApplyDiff(diff)
	if err != nil {
		return a, err
	}

	return accounts{kv: a.kv, tree: a.tree}, nil
}

func (a accounts) DumpDiff(viewID uint64) []byte {
	return a.tree.DumpDiff(viewID)
}

func (a accounts) ReadAccountBalance(id common.AccountID) (uint64, bool) {
	buf, exists := a.readUnderAccounts(id, keyAccountBalance[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func (a accounts) WriteAccountBalance(id common.AccountID, balance uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], balance)

	logger := log.Account(id, "balance_updated")
	logger.Log().Uint64("balance", balance).Msg("Updated balance.")

	a.writeUnderAccounts(id, keyAccountBalance[:], buf[:])
}

func (a accounts) ReadAccountStake(id common.AccountID) (uint64, bool) {
	buf, exists := a.readUnderAccounts(id, keyAccountStake[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func (a accounts) WriteAccountStake(id common.AccountID, stake uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], stake)

	logger := log.Account(id, "stake_updated")
	logger.Log().Uint64("stake", stake).Msg("Updated stake.")

	a.writeUnderAccounts(id, keyAccountStake[:], buf[:])
}

func (a accounts) ReadAccountContractCode(id common.TransactionID) ([]byte, bool) {
	buf, exists := a.readUnderAccounts(id, keyAccountContractCode[:])
	if !exists || len(buf) == 0 {
		return nil, false
	}

	return buf, true
}

func (a accounts) WriteAccountContractCode(id common.TransactionID, code []byte) {
	a.writeUnderAccounts(id, keyAccountContractCode[:], code[:])
}

func (a accounts) ReadAccountContractNumPages(id common.TransactionID) (uint64, bool) {
	buf, exists := a.readUnderAccounts(id, keyAccountContractNumPages[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func (a accounts) WriteAccountContractNumPages(id common.TransactionID, numPages uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], numPages)

	logger := log.Account(id, "num_pages_updated")
	logger.Log().Uint64("num_pages", numPages).Msg("Updated number of memory pages for a contract.")

	a.writeUnderAccounts(id, keyAccountContractNumPages[:], buf[:])
}

func (a accounts) ReadAccountContractPage(id common.TransactionID, idx uint64) ([]byte, bool) {
	var idxBuf [8]byte
	binary.LittleEndian.PutUint64(idxBuf[:], idx)

	buf, exists := a.readUnderAccounts(id, append(keyAccountContractPages[:], idxBuf[:]...))
	if !exists || len(buf) == 0 {
		return nil, false
	}

	decoded, err := snappy.Decode(nil, buf)
	if err != nil {
		return nil, false
	}

	return decoded, true
}

func (a accounts) WriteAccountContractPage(id common.TransactionID, idx uint64, page []byte) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], idx)

	encoded := snappy.Encode(nil, page)

	a.writeUnderAccounts(id, append(keyAccountContractPages[:], buf[:]...), encoded)
}

func (a accounts) readUnderAccounts(id common.AccountID, key []byte) ([]byte, bool) {
	buf, exists := a.tree.Lookup(append(keyAccounts[:], append(key, id[:]...)...))

	if !exists {
		return nil, false
	}

	return buf, true
}

func (a accounts) writeUnderAccounts(id common.AccountID, key, value []byte) {
	a.tree.Insert(append(keyAccounts[:], append(key, id[:]...)...), value[:])
}

func (a accounts) CommitAccounts() error {
	err := a.tree.Commit()
	if err != nil {
		return errors.Wrap(err, "accounts: failed to write")
	}

	return nil
}
