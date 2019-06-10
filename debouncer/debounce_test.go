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

package debouncer

import (
	"context"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
	"time"
)

func TestDebouncerOverfill(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := NewBatchDebouncer(ctx, func([]interface{}) {}, 1*time.Second, 10)

	for i := 0; i < 10; i++ {
		d.Add(interface{}(nil), 1, "")
	}

	done := make(chan struct{})
	go func() {
		d.Add(interface{}(nil), 1, "")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Millisecond):
		assert.Fail(t, "Put() timed out")
	}
}

func TestDebouncerBufferFull(t *testing.T) {
	called := 0
	a := func([]interface{}) {
		called++
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := NewBatchDebouncer(ctx, a, 10*time.Millisecond, 100)

	for i := 0; i < 1000; i++ {
		d.Add(interface{}(nil), 1, "")
	}

	time.Sleep(20 * time.Millisecond)

	// Since timer period is much bigger than needed, we expect debouncer to call handler
	// based on buffer threshold (10 = 1000/100).
	assert.Equal(t, 10, called)
}

func TestDebouncerTimer(t *testing.T) {
	called := 0
	a := func([]interface{}) {
		called++
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := NewBatchDebouncer(ctx, a, 1*time.Millisecond, 1)

	for i := 0; i < 100; i++ {
		d.Add(interface{}(nil), 1, "")
	}

	time.Sleep(4 * time.Millisecond)

	// Since timer period is much smaller comparing to speed on which data incoming
	// we expect number of handler calls to be based on timer (100 calls per 1 tx).
	assert.Equal(t, 100, called)
}

func BenchmarkDebouncer(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := NewBatchDebouncer(ctx, func([]interface{}) {}, 50*time.Millisecond, 16384)
	for i := 0; i < b.N; i++ {
		d.Add(interface{}(nil), 1, "")
	}
}

func TestFuncDebouncer(t *testing.T) {
	called := 0
	size := 0
	action := func(data []interface{}) {
		size = len(data)
		called++
	}

	fd := NewGroupDebouncer(context.TODO(), action, 100*time.Millisecond)
	var key string
	for i := 0; i < 10; i++ {
		if i%5 == 0 {
			key = strconv.Itoa(i)
		}
		fd.Add(interface{}(nil), 1, key)
	}

	time.Sleep(300 * time.Millisecond)

	assert.Equal(t, 1, called)
	assert.Equal(t, 2, size)
}
