package wavelet

import (
	"encoding/binary"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
)

type accounts struct {
	kv   store.KV
	tree *avl.Tree

	snapshot bool
}

func newAccounts(kv store.KV) accounts {
	return accounts{kv: kv, tree: avl.New(kv), snapshot: false}
}

func (a accounts) snapshotAccounts() accounts {
	return accounts{kv: a.kv, tree: avl.LoadFromSnapshot(a.kv, a.tree.Snapshot()), snapshot: true}
}

func (a accounts) ReadAccountBalance(id [PublicKeySize]byte) (uint64, bool) {
	buf, exists := a.readUnderAccounts(id, keyAccountBalance[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func (a accounts) WriteAccountBalance(id [PublicKeySize]byte, balance uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], balance)

	a.writeUnderAccounts(id, keyAccountBalance[:], buf[:])
}

func (a accounts) ReadAccountStake(id [PublicKeySize]byte) (uint64, bool) {
	buf, exists := a.readUnderAccounts(id, keyAccountStake[:])
	if !exists || len(buf) == 0 {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

func (a accounts) WriteAccountStake(id [PublicKeySize]byte, stake uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], stake)

	a.writeUnderAccounts(id, keyAccountStake[:], buf[:])
}

func (a accounts) readUnderAccounts(id [PublicKeySize]byte, key []byte) ([]byte, bool) {
	buf, exists := a.tree.Lookup(append(keyAccounts[:], append(key, id[:]...)...))

	if !exists {
		return nil, false
	}

	return buf, true
}

func (a accounts) writeUnderAccounts(id [PublicKeySize]byte, key, value []byte) {
	a.tree.Insert(append(keyAccounts[:], append(key, id[:]...)...), value[:])
}

func (a accounts) CommitAccounts() error {
	// If we are operating on a snapshot, we shouldn't be allowed to commit
	// any changes to our snapshot to the disk.
	if a.snapshot == true {
		return nil
	}

	err := a.tree.Commit()
	if err != nil {
		return errors.Wrap(err, "accounts: failed to write")
	}

	return nil
}
