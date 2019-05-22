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
	timer     *time.Timer
	period    time.Duration
}

func NewDebouncer(threshold int, action actionHandler, period time.Duration) *Debouncer {
	return &Debouncer{
		buff:      make([]*wavelet.Transaction, 0, threshold),
		threshold: threshold,
		action:    action,
		period:    period,
		timer:     time.NewTimer(period),
	}
}

func (d *Debouncer) Start(ctx context.Context) {
	d.timer.Reset(d.period)
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.timer.C:
			d.Lock()
			if len(d.buff) > 0 {
				d.action(d.buff)
				d.buff = d.buff[:0]
			}

			d.timer.Reset(d.period)
			d.Unlock()
		}
	}
}

func (d *Debouncer) Put(tx *wavelet.Transaction) {
	d.Lock()
	if len(d.buff) == d.threshold {
		d.action(d.buff)
		d.buff = d.buff[:0]
		d.timer.Reset(d.period)
	}

	d.buff = append(d.buff, tx)
	d.Unlock()
}
