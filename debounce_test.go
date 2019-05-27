package wavelet

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestDebouncerOverfill(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	size := len(Transaction{}.Marshal())

	d := NewTransactionDebouncer(ctx, WithBufferLen(10*size), WithPeriod(1*time.Second))

	for i := 0; i < 10; i++ {
		d.Push(Transaction{})
	}

	done := make(chan struct{})
	go func() {
		d.Push(Transaction{})
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
	a := func([][]byte) {
		called++
	}

	size := len(Transaction{}.Marshal())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := NewTransactionDebouncer(ctx, WithBufferLen(100*size), WithAction(a), WithPeriod(10*time.Millisecond))

	for i := 0; i < 1000; i++ {
		d.Push(Transaction{})
	}

	time.Sleep(20 * time.Millisecond)

	// Since timer period is much bigger than needed, we expect debouncer to call handler
	// based on buffer threshold (10 = 1000/100).
	assert.Equal(t, 10, called)
}

func TestDebouncerTimer(t *testing.T) {
	called := 0
	a := func([][]byte) {
		called++
	}

	size := len(Transaction{}.Marshal())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := NewTransactionDebouncer(ctx, WithBufferLen(1*size), WithAction(a), WithPeriod(1*time.Millisecond))

	for i := 0; i < 100; i++ {
		d.Push(Transaction{})
	}

	time.Sleep(4 * time.Millisecond)

	// Since timer period is much smaller comparing to speed on which data incoming
	// we expect number of handler calls to be based on timer (100 calls per 1 tx).
	assert.Equal(t, 100, called)
}

func BenchmarkDebouncer(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := NewTransactionDebouncer(ctx)

	for i := 0; i < b.N; i++ {
		d.Push(Transaction{})
	}
}
