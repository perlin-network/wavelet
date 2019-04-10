package wavelet

import (
	"context"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type accounts struct {
	kv   store.KV
	tree *avl.Tree

	mu sync.RWMutex

	gcProfile *avl.GCProfile
}

func newAccounts(kv store.KV) *accounts {
	return &accounts{kv: kv, tree: avl.New(kv)}
}

// Only one instance of GC worker can run at any time.
func (a *accounts) runGCWorker(ctx context.Context) {
	timer := time.NewTicker(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			p := atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&a.gcProfile)), nil)
			if p != nil {
				profile := (*avl.GCProfile)(p)
				_, _ = profile.PerformFullGC()
			}
		}
	}
}

func (a *accounts) checksum() common.MerkleNodeID {
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

	profile := a.tree.GetGCProfile(0)
	if profile != nil {
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&a.gcProfile)), unsafe.Pointer(profile))
	}
	return nil
}
