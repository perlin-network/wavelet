// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package cuckoo

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"unsafe"
)

const (
	BucketSize           = 4
	NumBuckets           = 524288
	MaxInsertionAttempts = 500
)

type TransactionID [32]byte

type Filter struct {
	Buckets [NumBuckets]Bucket
	Count   uint

	unsafe bool
}

// UnsafeUnmarshalBinary is roughly 1.7x faster than UnmarshalBinary, but in return
// does not provide an accurate cardinality estimate of the items in the filter.
func UnsafeUnmarshalBinary(buf []byte) (*Filter, error) {
	if len(buf) != NumBuckets*BucketSize {
		return nil, fmt.Errorf("must be %d bytes, but got %d bytes", NumBuckets*BucketSize, len(buf))
	}

	ptr := (*reflect.SliceHeader)(unsafe.Pointer(&buf)).Data
	buckets := *(*[NumBuckets]Bucket)(unsafe.Pointer(ptr))

	return &Filter{Buckets: buckets, unsafe: true}, nil
}

func UnmarshalBinary(buf []byte) (*Filter, error) {
	if len(buf) != NumBuckets*BucketSize {
		return nil, fmt.Errorf("must be %d bytes, but got %d bytes", NumBuckets*BucketSize, len(buf))
	}

	count := CountNonzeroBytes(buf)

	ptr := (*reflect.SliceHeader)(unsafe.Pointer(&buf)).Data
	buckets := *(*[NumBuckets]Bucket)(unsafe.Pointer(ptr))

	return &Filter{Buckets: buckets, Count: count}, nil
}

func (f *Filter) MarshalBinary() []byte {
	if f.unsafe {
		panic("attempted to re-marshal an unsafely unmarshaled cuckoo filter")
	}

	var buf []byte

	sh := (*reflect.SliceHeader)(unsafe.Pointer(&buf))
	sh.Data = uintptr(unsafe.Pointer(&f.Buckets))
	sh.Len = NumBuckets * BucketSize
	sh.Cap = NumBuckets * BucketSize

	runtime.KeepAlive(f.Buckets)

	return buf
}

func NewFilter() *Filter {
	return &Filter{}
}

func (f *Filter) Reset() {
	f.Buckets = [NumBuckets]Bucket{}
	f.Count = 0
}

func (f *Filter) Insert(id TransactionID) bool {
	val, a, b := process(id)

	// Assert that the ID has not been inserted into the filter before.
	if f.Buckets[a].IndexOf(val) > -1 || f.Buckets[b].IndexOf(val) > -1 {
		return false
	}

	// Attempt to insert into bucket A.
	if f.Buckets[a].Insert(val) {
		f.Count++
		return true
	}

	// Attempt to insert into bucket B .
	if f.Buckets[b].Insert(val) {
		f.Count++
		return true
	}

	i := a
	if rand.Intn(2) == 0 {
		i = b
	}

	for attempt := 0; attempt < MaxInsertionAttempts; attempt++ {
		j := rand.Intn(BucketSize)

		val, f.Buckets[i][j] = f.Buckets[i][j], val

		i = (i ^ jenkins(uint(val))) % NumBuckets

		if f.Buckets[i].Insert(val) {
			f.Count++
			return true
		}
	}

	return false
}

func (f *Filter) Delete(id TransactionID) bool {
	val, a, b := process(id)

	if f.Buckets[a].Delete(val) {
		f.Count--
		return true
	}

	if f.Buckets[b].Delete(val) {
		f.Count--
		return true
	}

	return false
}

func (f *Filter) Lookup(id TransactionID) bool {
	val, a, b := process(id)
	return f.Buckets[a].IndexOf(val) > -1 || f.Buckets[b].IndexOf(val) > -1
}

func process(id TransactionID) (byte, uint, uint) {
	val := byte(binary.BigEndian.Uint64(id[0:8])%255 + 1)
	a := uint(binary.BigEndian.Uint64(id[8:16])) % NumBuckets
	b := (a ^ jenkins(uint(val))) % NumBuckets

	return val, a, b
}

type Bucket [BucketSize]byte

func (b *Bucket) Insert(val byte) bool {
	for i, stored := range b {
		if stored == 0 {
			b[i] = val
			return true
		}
	}

	return false
}

func (b *Bucket) Delete(val byte) bool {
	for i, stored := range b {
		if stored == val {
			b[i] = 0
			return true
		}
	}

	return false
}

func (b *Bucket) IndexOf(val byte) int {
	for i, stored := range b {
		if stored == val {
			return i
		}
	}

	return -1
}

func jenkins(a uint) uint {
	a = (a + 0x7ed55d16) + (a << 12)
	a = (a ^ 0xc761c23c) ^ (a >> 19)
	a = (a + 0x165667b1) + (a << 5)
	a = (a + 0xd3a2646c) ^ (a << 9)
	a = (a + 0xfd7046c5) + (a << 3)
	a = (a ^ 0xb55a4f09) ^ (a >> 16)

	return a
}
