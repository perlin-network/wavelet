package wavelet

import (
	"encoding/binary"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
)

type accounts struct {
	tree *avl.Tree
}

func newAccounts(kv store.KV) accounts {
	return accounts{tree: avl.New(kv)}
}

func (a accounts) ReadAccountBalance(id []byte) (uint64, error) {
	buf, exists := a.tree.Lookup(append(keyAccounts[:], append(keyAccountBalance[:], id...)...))

	if !exists {
		return 0, errors.New("accounts: could not find account balance")
	}

	return binary.LittleEndian.Uint64(buf), nil
}

func (a accounts) WriteAccountBalance(id []byte, balance uint64) error {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], balance)

	a.tree.Insert(append(keyAccounts[:], append(keyAccountBalance[:], id...)...), buf[:])

	err := a.tree.Commit()
	if err != nil {
		return errors.Wrap(err, "failed to store account balance")
	}

	return nil
}
