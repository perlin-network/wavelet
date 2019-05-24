package wavelet

import (
	"context"
	"sync"
	"time"
)

type TransactionDebouncerOption func(*TransactionDebouncer)

func WithBufferLen(bufferLen int) TransactionDebouncerOption {
	return func(d *TransactionDebouncer) {
		d.bufferLen = bufferLen
	}
}

func WithPeriod(period time.Duration) TransactionDebouncerOption {
	return func(d *TransactionDebouncer) {
		d.period = period
	}
}

func WithAction(action func([][]byte)) TransactionDebouncerOption {
	return func(d *TransactionDebouncer) {
		d.action = action
	}
}

type TransactionDebouncer struct {
	sync.Mutex

	action func([][]byte)

	timer  *time.Timer
	period time.Duration

	buffer    [][]byte
	bufferPtr int
	bufferLen int
}

func NewTransactionDebouncer(ctx context.Context, opts ...TransactionDebouncerOption) *TransactionDebouncer {
	d := &TransactionDebouncer{
		action: func([][]byte) {},

		timer:  time.NewTimer(50 * time.Millisecond),
		period: 50 * time.Millisecond,

		bufferLen: 16384,
	}
	d.timer.Stop()

	for _, opt := range opts {
		opt(d)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-d.timer.C:
				d.Lock()
				if d.bufferPtr >= d.bufferLen {
					d.action(d.buffer)
					d.buffer = d.buffer[:0]
					d.bufferPtr = 0
				}
				d.Unlock()
			}
		}
	}()

	return d
}

func (d *TransactionDebouncer) Push(tx Transaction) {
	d.Lock()
	if d.bufferPtr >= d.bufferLen {
		d.action(d.buffer)
		d.buffer = d.buffer[:0]
		d.bufferPtr = 0
	}

	d.timer.Reset(d.period)

	buf := tx.Marshal()
	d.buffer = append(d.buffer, buf)
	d.bufferPtr += len(buf)

	d.Unlock()
}
