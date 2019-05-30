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

package avl

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLRU(t *testing.T) {
	lru := newLRU(2)

	lru.put([MerkleHashSize]byte{'a'}, 1)
	lru.put([MerkleHashSize]byte{'b'}, 2)
	_, ok := lru.load([MerkleHashSize]byte{'b'})
	assert.True(t, ok)
	_, ok = lru.load([MerkleHashSize]byte{'a'})
	assert.True(t, ok)

	lru.put([MerkleHashSize]byte{'c'}, 3)
	_, ok = lru.load([MerkleHashSize]byte{'b'})
	assert.False(t, ok)

	val, ok := lru.load([MerkleHashSize]byte{'a'})
	assert.True(t, ok)
	assert.Equal(t, 1, val.(int))

	val, ok = lru.load([MerkleHashSize]byte{'c'})
	assert.True(t, ok)
	assert.Equal(t, 3, val.(int))
}
