package wctl

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

type Counter struct {
	n    uint64
	lock sync.RWMutex
	x    [15]byte // instead of "x uint64"
}

func (c *Counter) xAddr() *uint64 {
	// The return must be 8-byte aligned.
	return (*uint64)(unsafe.Pointer(
		uintptr(unsafe.Pointer(&c.x)) + 8 -
			uintptr(unsafe.Pointer(&c.x))%8))
}

func (c *Counter) Store(val uint64) {
	// c.lock.Lock()
	// defer c.lock.Unlock()

	// c.n = val

	p := c.xAddr()
	atomic.StoreUint64(p, val)
}

func (c *Counter) Add(delta uint64) uint64 {
	// c.lock.Lock()
	// defer c.lock.Unlock()

	// c.n += delta
	// return c.n

	p := c.xAddr()
	return atomic.AddUint64(p, delta)
}

func (c *Counter) Value() uint64 {
	// c.lock.RLock()
	// defer c.lock.RUnlock()

	// return c.n

	return atomic.LoadUint64(c.xAddr())
}
