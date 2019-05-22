package debouncer

import (
	"context"
	"github.com/perlin-network/wavelet"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestDebouncerOverfill(t *testing.T) {
	a := func(txs []*wavelet.Transaction) {}

	d := NewDebouncer(10, a, 1*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Start(ctx)

	for i := 0; i < 10; i++ {
		d.Put(&wavelet.Transaction{})
	}

	done := make(chan struct{})
	go func() {
		d.Put(&wavelet.Transaction{})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Millisecond):
		assert.Fail(t, "timeouted put")
	}
}

func TestDebouncerBufferFull(t *testing.T) {
	called := 0
	a := func(txs []*wavelet.Transaction) {
		called++
	}

	d := NewDebouncer(100, a, 1*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Start(ctx)

	for i := 0; i < 1000; i++ {
		d.Put(&wavelet.Transaction{})
	}

	time.Sleep(1500 * time.Millisecond)
	// since timer period is much bigger than needed, we expect debouncer to call handler
	// based on buffer threshold (10 = 1000/100)
	assert.Equal(t, 10, called)
}

func TestDebouncerTimer(t *testing.T) {
	called := 0
	a := func(txs []*wavelet.Transaction) {
		called++
	}

	d := NewDebouncer(10, a, 1*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Start(ctx)

	for i := 0; i < 100; i++ {
		d.Put(&wavelet.Transaction{})
		time.Sleep(2 * time.Millisecond)
	}

	// since timer period is much smaller comparing to speed on which data incoming
	// we expect number of handler calls to be based on timer (100 calls per 1tx)
	assert.Equal(t, 100, called)
}

func BenchmarkDebouncer(b *testing.B) {
	d := NewDebouncer(40, func([]*wavelet.Transaction) {}, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Start(ctx)

	for i := 0; i < b.N; i++ {
		d.Put(&wavelet.Transaction{})
	}
}
