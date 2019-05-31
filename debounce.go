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

package wavelet

import (
	"context"
	"sync"
	"time"
)

type DebouncerOption func(*Debouncer)

func WithBufferLen(bufferLen int) DebouncerOption {
	return func(d *Debouncer) {
		d.actionLimit = bufferLen
	}
}

func WithPeriod(period time.Duration) DebouncerOption {
	return func(d *Debouncer) {
		d.period = period
	}
}

func WithAction(action func([][]byte)) DebouncerOption {
	return func(d *Debouncer) {
		d.action = action
	}
}

func WithLimit(limit int) DebouncerOption {
	return func(d *Debouncer) {
		d.actionLimit = limit
	}
}

type Debouncer struct {
	sync.Mutex

	action func([][]byte)

	timer  *time.Timer
	period time.Duration

	buffer      [][]byte
	bufferSize  int
	actionLimit int
}

func NewDebouncer(ctx context.Context, opts ...DebouncerOption) *Debouncer {
	d := &Debouncer{
		action: func([][]byte) {},

		timer:  time.NewTimer(50 * time.Millisecond),
		period: 50 * time.Millisecond,

		actionLimit: 16384,
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
				if d.bufferSize > 0 {
					d.action(d.buffer)
					d.buffer = d.buffer[:0]
					d.bufferSize = 0
				}
				d.Unlock()
			}
		}
	}()

	return d
}

func (d *Debouncer) Push(payload []byte) {
	d.Lock()
	if d.bufferSize >= d.actionLimit {
		d.action(d.buffer)
		d.buffer = d.buffer[:0]
		d.bufferSize = 0
	}

	d.timer.Reset(d.period)

	d.buffer = append(d.buffer, payload)
	d.bufferSize += len(payload)

	d.Unlock()
}
