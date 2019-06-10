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
	"sync"
	"time"
)

type Debouncer interface {
	Add([]byte, int, string)
}

var (
	_ Debouncer = (*Deduper)(nil)
	_ Debouncer = (*Limiter)(nil)
)

type Deduper struct {
	sync.Mutex
	action func([][]byte)

	payload map[string][]byte
	timer   *time.Timer
	period  time.Duration
}

func NewDeduper(ctx context.Context, action func([][]byte), period time.Duration) *Deduper {
	d := &Deduper{
		action:  action,
		period:  period,
		timer:   time.NewTimer(period),
		payload: make(map[string][]byte),
	}
	d.timer.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-d.timer.C:
				d.Lock()
				payload := make([][]byte, 0, len(d.payload))
				for _, v := range d.payload {
					payload = append(payload, v)
				}
				d.action(payload)
				d.Unlock()
			}
		}
	}()

	return d
}

func (d *Deduper) Add(payload []byte, _ int, key string) {
	d.Lock()
	d.payload[key] = payload
	d.timer.Reset(d.period)
	d.Unlock()
}

type Limiter struct {
	sync.Mutex

	action func([][]byte)

	timer  *time.Timer
	period time.Duration

	buffer      [][]byte
	bufferSize  int
	bufferLimit int
}

func NewLimiter(ctx context.Context, action func([][]byte), period time.Duration, limit int) *Limiter {
	d := &Limiter{
		action:      action,
		period:      period,
		bufferLimit: limit,
		timer:       time.NewTimer(period),
	}

	d.timer.Stop()

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

func (d *Limiter) Add(payload []byte, size int, _ string) {
	d.Lock()
	if d.bufferSize >= d.bufferLimit {
		d.action(d.buffer)
		d.buffer = d.buffer[:0]
		d.bufferSize = 0
	}

	d.timer.Reset(d.period)

	d.buffer = append(d.buffer, payload)
	d.bufferSize += size

	d.Unlock()
}
