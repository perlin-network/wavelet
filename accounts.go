package wavelet

import (
	"context"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type Accounts struct {
	sync.RWMutex

	kv   store.KV
	tree *avl.Tree

	profile *avl.GCProfile
}

func NewAccounts(kv store.KV) *Accounts {
	return &Accounts{kv: kv, tree: avl.New(kv)}
}

// GC periodically garbage collects every 5 seconds. Only one
// instance of GC worker can run at any time.
func (a *Accounts) GC(ctx context.Context) {
	timer := time.NewTicker(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			p := atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&a.profile)), nil)
			if p != nil {
				profile := (*avl.GCProfile)(p)
				_, _ = profile.PerformFullGC()
			}
		}
	}
}

func (a *Accounts) Snapshot() *avl.Tree {
	a.RLock()
	snapshot := a.tree.Snapshot()
	a.RUnlock()

	return snapshot
}

func (a *Accounts) Commit(new *avl.Tree) error {
	a.Lock()
	defer a.Unlock()

	if new != nil {
		a.tree = new
	}

	err := a.tree.Commit()
	if err != nil {
		return errors.Wrap(err, "accounts: failed to write")
	}

	profile := a.tree.GetGCProfile(0)
	if profile != nil {
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&a.profile)), unsafe.Pointer(profile))
	}
	return nil
}
