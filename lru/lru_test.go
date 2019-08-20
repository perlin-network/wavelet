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

package lru

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLRU(t *testing.T) {
	lru := NewLRU(2)

	lru.Put([32]byte{'a'}, 1)
	lru.Put([32]byte{'b'}, 2)
	_, ok := lru.Load([32]byte{'b'})
	assert.True(t, ok)
	_, ok = lru.Load([32]byte{'a'})
	assert.True(t, ok)

	lru.Put([32]byte{'c'}, 3)
	_, ok = lru.Load([32]byte{'b'})
	assert.False(t, ok)

	val, ok := lru.Load([32]byte{'a'})
	assert.True(t, ok)
	assert.Equal(t, 1, val.(int))

	val, ok = lru.Load([32]byte{'c'})
	assert.True(t, ok)
	assert.Equal(t, 3, val.(int))
}

func TestLRU_PutWithEvictCallback(t *testing.T) {
	lru := NewLRU(2)

	lru.Put([32]byte{'a'}, 1)
	lru.Put([32]byte{'b'}, 2)

	var called bool
	lru.PutWithEvictCallback([32]byte{'c'}, 3, func(key interface{}, val interface{}) {
		assert.EqualValues(t, [32]byte{'a'}, key)
		assert.EqualValues(t, 1, val)
		called = true
	})
	assert.True(t, called)

	called = false
	lru.PutWithEvictCallback([32]byte{'d'}, 4, func(key interface{}, val interface{}) {
		assert.EqualValues(t, [32]byte{'b'}, key)
		assert.EqualValues(t, 2, val)
		called = true
	})
	assert.True(t, called)
}
