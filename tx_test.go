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

// +build unit

package wavelet

import (
	"bytes"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"testing"
)

func BenchmarkNewTX(b *testing.B) {
	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		NewTransaction(keys, 0, 0, sys.TagTransfer, nil)
	}
}

func BenchmarkMarshalUnmarshalTX(b *testing.B) {
	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(b, err)

	tx := NewTransaction(keys, 0, 0, sys.TagTransfer, nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := UnmarshalTransaction(bytes.NewReader(tx.Marshal()))
		assert.NoError(b, err)
	}
}

//func TestMarshalTransaction(t *testing.T) {
//	keys, err := skademlia.NewKeys(1, 1)
//	assert.NoError(t, err)
//
//	tx := NewTransaction(keys, 2, 13, sys.TagTransfer, []byte{1, 2, 3})
//	buf := tx.Marshal()
//
//	var b []byte
//
//	sh := (*reflect.SliceHeader)(unsafe.Pointer(&b))
//	sh.Data = uintptr(unsafe.Pointer(&tx))
//	sh.Len = int(unsafe.Sizeof(tx))
//	sh.Cap = int(unsafe.Sizeof(tx))
//
//	runtime.KeepAlive(tx)
//
//	fmt.Println(buf)
//	fmt.Println(b[:len(b)-32])
//
//	fmt.Println(len(buf), len(b), unsafe.Sizeof(tx))
//}
