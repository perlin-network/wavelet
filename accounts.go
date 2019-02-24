package wavelet

import (
	"encoding/binary"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
)

// TODO(kenta): use a merklized AVL tree w/ versioning instead of raw key/value pairs
type accounts struct {
	kv store.KV
}

func newAccounts(kv store.KV) accounts {
	return accounts{kv: kv}
}

func (a accounts) ReadAccountBalance(id []byte) (uint64, error) {
	buf, err := a.kv.Get(append(keyAccounts[:], append(keyAccountBalance[:], id...)...))
	if err != nil {
		return 0, errors.Wrap(err, "failed to load account balance")
	}

	return binary.LittleEndian.Uint64(buf), nil
}

func (a accounts) WriteAccountBalance(id []byte, balance uint64) error {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], balance)

	err := a.kv.Put(append(keyAccounts[:], append(keyAccountBalance[:], id...)...), buf[:])
	if err != nil {
		return errors.Wrap(err, "failed to store account balance")
	}

	return nil
}
