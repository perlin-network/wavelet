package wavelet

import (
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"sync"
)

type accounts struct {
	kv   store.KV
	tree *avl.Tree

	mu sync.RWMutex

	gcWorkerChannel chan avl.GCProfile
}

func newAccounts(kv store.KV) *accounts {
	return &accounts{kv: kv, tree: avl.New(kv), gcWorkerChannel: make(chan avl.GCProfile, 200)}
}

// Only one instance of GC worker can run at any time.
func (a *accounts) runGCWorker() {
	for {
		profile := <-a.gcWorkerChannel
		n, err := profile.PerformFullGC()
		if err != nil {
			log.Error().Err(err).Msg("Full gc failed")
		} else {
			log.Info().Msgf("Removed %d nodes in full GC", n)
		}
	}
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

	profile, gotProfile := a.tree.GetGCProfile(0)
	if gotProfile {
		a.gcWorkerChannel <- profile
	}

	return nil
}
