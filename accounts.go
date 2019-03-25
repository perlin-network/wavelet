package wavelet

import (
	"github.com/perlin-network/wavelet/avl"
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
