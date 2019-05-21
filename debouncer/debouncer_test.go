package debouncer

import (
	"github.com/perlin-network/wavelet"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestDebouncerOverfill(t *testing.T) {
	a := func(txs []*wavelet.Transaction) {}

	d := NewDebouncer(10, a, 1*time.Second)
	go d.Start()
	defer d.Stop()

	for i := 0; i < 10; i++ {
		d.Put(&wavelet.Transaction{})
	}
	assert.False(t, d.Put(&wavelet.Transaction{}))
}

func TestDebouncer(t *testing.T) {
	a := func(txs []*wavelet.Transaction) {}

	d := NewDebouncer(100, a, 1*time.Millisecond)
	go d.Start()
	defer d.Stop()

	for i := 0; i < 1000; i++ {
		assert.True(t, d.Put(&wavelet.Transaction{}), i)
	}
}

func BenchmarkDebouncer(b *testing.B) {
	d := NewDebouncer(40, func([]*wavelet.Transaction) {}, 50*time.Millisecond)
	go d.Start()
	defer d.Stop()

	for i := 0; i < b.N; i++ {
		d.Put(&wavelet.Transaction{})
	}
}
