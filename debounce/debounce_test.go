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

package debounce

import (
	"context"
	"crypto/rand"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLimiterOverfill(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := NewLimiter(ctx, WithPeriod(1*time.Second), WithBufferLimit(10))

	for i := 0; i < 10; i++ {
		d.Add()
	}

	done := make(chan struct{})
	go func() {
		d.Add()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Millisecond):
		assert.Fail(t, "Put() timed out")
	}
}

func TestLimiterBufferFull(t *testing.T) {
	var called int32
	a := func([][]byte) {
		atomic.AddInt32(&called, 1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := NewLimiter(ctx, WithAction(a), WithPeriod(10*time.Millisecond), WithBufferLimit(100))

	for i := 0; i < 1000; i++ {
		var msg [1]byte

		_, err := rand.Read(msg[:])
		assert.NoError(t, err)

		d.Add(Bytes(msg[:]))
	}

	time.Sleep(20 * time.Millisecond)

	// Since timer period is much bigger than needed, we expect debouncer to call handler
	// based on buffer threshold (10 = 1000/100).
	assert.Equal(t, int32(10), atomic.LoadInt32(&called))
}

func TestLimiterTimer(t *testing.T) {
	var called int32
	a := func([][]byte) {
		atomic.AddInt32(&called, 1)
	}

	timeout := 1 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := NewLimiter(ctx, WithAction(a), WithPeriod(timeout), WithBufferLimit(1))

	for i := 0; i < 100; i++ {
		d.Add(Bytes([]byte{0x00, 0x01, 0x02}))
	}

	time.Sleep(timeout * 2)

	// Since timer period is much smaller comparing to speed on which data incoming
	// we expect number of handler calls to be based on timer (100 calls per 1 tx).
	assert.Equal(t, int32(100), atomic.LoadInt32(&called))
}

func BenchmarkLimiter(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := NewLimiter(ctx)
	for i := 0; i < b.N; i++ {
		d.Add()
	}
}

func TestDeduper(t *testing.T) {
	var called int32
	size := 0
	action := func(data [][]byte) {
		size = len(data)
		atomic.AddInt32(&called, 1)
	}

	fd := NewDeduper(context.TODO(), WithAction(action), WithPeriod(100*time.Millisecond), WithKeys("test"))
	var key string
	for i := 0; i < 10; i++ {
		if i%5 == 0 {
			key = strconv.Itoa(i)
		}

		fd.Add(Bytes([]byte(fmt.Sprintf(`{"test": "%s"}`, key))))
	}

	time.Sleep(200 * time.Millisecond)

	assert.Equal(t, int32(1), atomic.LoadInt32(&called))
	assert.Equal(t, 2, size)
}
