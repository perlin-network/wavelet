// +build !amd64

package cuckoo

import (
	"math/bits"
	"unsafe"
)

const (
	x7F = uint64(0x7F7F7F7F7F7F7F7F)
	x80 = uint64(0x8080808080808080)
)

func CountNonzeroBytes(buf []byte) (count uint) {
	l := len(buf) - len(buf)%8

	for _, v := range buf[l:] {
		if v != 0 {
			count++
		}
	}

	bin := (*[1 << 20]uint64)(unsafe.Pointer(&buf[0]))[: l/8 : l/8]
	for _, w := range bin {
		w = (w & x80) | (((w & x7F) + x7F) & x80)
		count += uint(bits.OnesCount64(w))
	}

	return count
}
