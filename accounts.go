package wavelet

import (
	"encoding/binary"
	"github.com/golang/snappy"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
)

type accounts struct {
	kv   store.KV
	tree *avl.Tree
}

func newAccounts(kv store.KV) accounts {
	return accounts{kv: kv, tree: avl.New(kv)}
}

func (a accounts) snapshotAccounts() accounts {
	return accounts{kv: a.kv, tree: avl.LoadFromSnapshot(a.kv, a.tree.Snapshot())}
}

func (a accounts) ReadAccountBalance(id [sys.PublicKeySize]byte) (uint64, bool) {
	buf, exists := a.readUnderAccounts(id, keyAccountBalance[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func (a accounts) WriteAccountBalance(id [sys.PublicKeySize]byte, balance uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], balance)

	log.Account(id, "balance_updated").Log().
		Uint64("balance", balance).
		Msg("Updated balance.")

	a.writeUnderAccounts(id, keyAccountBalance[:], buf[:])
}

func (a accounts) ReadAccountStake(id [sys.PublicKeySize]byte) (uint64, bool) {
	buf, exists := a.readUnderAccounts(id, keyAccountStake[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func (a accounts) WriteAccountStake(id [sys.PublicKeySize]byte, stake uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], stake)

	log.Account(id, "stake_updated").Log().
		Uint64("stake", stake).
		Msg("Updated stake.")

	a.writeUnderAccounts(id, keyAccountStake[:], buf[:])
}

func (a accounts) ReadAccountContractCode(id [sys.TransactionIDSize]byte) ([]byte, bool) {
	buf, exists := a.readUnderAccounts(id, keyAccountContractCode[:])
	if !exists || len(buf) == 0 {
		return nil, false
	}

	return buf, true
}

func (a accounts) WriteAccountContractCode(id [sys.TransactionIDSize]byte, code []byte) {
	a.writeUnderAccounts(id, keyAccountContractCode[:], code[:])
}

func (a accounts) ReadAccountContractNumPages(id [sys.TransactionIDSize]byte) (uint64, bool) {
	buf, exists := a.readUnderAccounts(id, keyAccountContractNumPages[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func (a accounts) WriteAccountContractNumPages(id [sys.TransactionIDSize]byte, numPages uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], numPages)

	log.Account(id, "num_pages_updated").Log().
		Uint64("num_pages", numPages).
		Msg("Updated number of memory pages for a contract.")

	a.writeUnderAccounts(id, keyAccountContractNumPages[:], buf[:])
}

func (a accounts) ReadAccountContractPage(id [sys.TransactionIDSize]byte, idx uint64) ([]byte, bool) {
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

func (a accounts) WriteAccountContractPage(id [sys.TransactionIDSize]byte, idx uint64, page []byte) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], idx)

	encoded := snappy.Encode(nil, page)

	a.writeUnderAccounts(id, append(keyAccountContractPages[:], buf[:]...), encoded)
}

func (a accounts) readUnderAccounts(id [sys.PublicKeySize]byte, key []byte) ([]byte, bool) {
	buf, exists := a.tree.Lookup(append(keyAccounts[:], append(key, id[:]...)...))

	if !exists {
		return nil, false
	}

	return buf, true
}

func (a accounts) writeUnderAccounts(id [sys.PublicKeySize]byte, key, value []byte) {
	a.tree.Insert(append(keyAccounts[:], append(key, id[:]...)...), value[:])
}

func (a accounts) CommitAccounts() error {
	err := a.tree.Commit()
	if err != nil {
		return errors.Wrap(err, "accounts: failed to write")
	}

	return nil
}
