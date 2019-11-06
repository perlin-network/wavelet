package wctl

import (
	"sync/atomic"
	"unsafe"
)

type Counter struct {
	x [15]byte // instead of "x uint64"
}

func (c *Counter) xAddr() *uint64 {
	// The return must be 8-byte aligned.
	return (*uint64)(unsafe.Pointer(
		uintptr(unsafe.Pointer(&c.x)) + 8 -
			uintptr(unsafe.Pointer(&c.x))%8))
}

func (c *Counter) Store(val uint64) {
	p := c.xAddr()
	atomic.StoreUint64(p, val)
}

func (c *Counter) Add(delta uint64) uint64 {
	p := c.xAddr()
	return atomic.AddUint64(p, delta)
}

func (c *Counter) Value() uint64 {
	return atomic.LoadUint64(c.xAddr())
}
