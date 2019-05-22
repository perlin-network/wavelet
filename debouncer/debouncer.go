package debouncer

import (
	"context"
	"github.com/perlin-network/wavelet"
	"sync"
	"time"
)

type actionHandler func([]*wavelet.Transaction)

type Debouncer struct {
	buff []*wavelet.Transaction
	sync.Mutex
	threshold int
	action    actionHandler
	period    time.Duration
}

func NewDebouncer(threshold int, action actionHandler, period time.Duration) *Debouncer {
	return &Debouncer{
		buff:      make([]*wavelet.Transaction, 0, threshold),
		threshold: threshold,
		action:    action,
		period:    period,
	}
}

func (d *Debouncer) Start(ctx context.Context) {
	timer := time.NewTimer(d.period)
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			d.Lock()
			if len(d.buff) > 0 {
				d.action(d.buff)
				d.buff = d.buff[:0]
			}

			timer.Reset(d.period)
			d.Unlock()
		default:
			d.Lock()
			if len(d.buff) == d.threshold {
				d.action(d.buff)
				d.buff = d.buff[:0]
				timer.Reset(d.period)
			}
			d.Unlock()
		}
	}
}

func (d *Debouncer) Put(tx *wavelet.Transaction) {
	for {
		d.Lock()

		if len(d.buff) == d.threshold {
			d.Unlock()
			time.Sleep(1 * time.Nanosecond)
		} else {
			d.buff = append(d.buff, tx)
			d.Unlock()
			return
		}
	}
}
