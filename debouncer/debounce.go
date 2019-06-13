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
	Add(...DebounceOptionSetter)
}

const (
	DUMMY int = iota
	LIMITER
	DEDUPER
	SINGLE
)

var (
	_ Debouncer = (*Dummy)(nil)
	_ Debouncer = (*Deduper)(nil)
	_ Debouncer = (*Limiter)(nil)
	_ Debouncer = (*Single)(nil)
)

type config struct {
	batchAction     func([][]byte)
	singleAction    func([]byte)
	period          time.Duration
	bufferLimit     int
	useSingleAction bool
}

type ConfigSetter func(cfg *config)

func gatherConfig(css []ConfigSetter) *config {
	cfg := &config{
		batchAction: func([][]byte) {},
		period:      50 * time.Millisecond,
		bufferLimit: 16384,
	}

	for _, cs := range css {
		cs(cfg)
	}

	return cfg
}

func WithBatchAction(action func([][]byte)) ConfigSetter {
	return func(cfg *config) {
		cfg.batchAction = action
	}
}

func WithSingleAction(action func([]byte)) ConfigSetter {
	return func(cfg *config) {
		cfg.singleAction = action
	}
}

func WithPeriod(period time.Duration) ConfigSetter {
	return func(cfg *config) {
		cfg.period = period
	}
}

func WithBufferLimit(limit int) ConfigSetter {
	return func(cfg *config) {
		cfg.bufferLimit = limit
	}
}

type options struct {
	payload []byte
	key     string
}

type DebounceOptionSetter func(o *options)

func gatherOptions(oss []DebounceOptionSetter) *options {
	o := &options{
		payload: make([]byte, 1),
	}

	for _, os := range oss {
		os(o)
	}

	return o
}

func WithPayload(p []byte) DebounceOptionSetter {
	return func(o *options) {
		o.payload = p
	}
}

func WithGroupKeys(keys ...string) DebounceOptionSetter {
	return func(o *options) {
		k := ""
		for _, key := range keys {
			k += key
		}

		o.key = k
	}
}

type DebounceFactory func(css ...ConfigSetter) Debouncer

func MakeFactory(ctx context.Context, debounceType int, factorySetters ...ConfigSetter) DebounceFactory {
	return func(css ...ConfigSetter) Debouncer {
		css = append(css, factorySetters...)

		switch debounceType {
		case DEDUPER:
			return NewDeduper(ctx, css...)
		case LIMITER:
			return NewLimiter(ctx, css...)
		case SINGLE:
			return NewSingle(ctx, css...)
		case DUMMY:
			fallthrough
		default:
			return NewDummy(css...)
		}
	}
}

type Dummy struct {
	config
}

func NewDummy(css ...ConfigSetter) Debouncer {
	cfg := gatherConfig(css)
	return &Dummy{*cfg}
}

func (d *Dummy) Add(oss ...DebounceOptionSetter) {
	o := gatherOptions(oss)

	if d.batchAction != nil {
		d.batchAction([][]byte{o.payload})
	}
}

type Single struct {
	sync.Mutex
	config

	payload []byte
	timer   *time.Timer
}

func NewSingle(ctx context.Context, css ...ConfigSetter) *Single {
	cfg := gatherConfig(css)
	d := &Single{config: *cfg}
	d.timer = time.NewTimer(d.period)
	d.timer.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-d.timer.C:
				d.Lock()
				d.singleAction(d.payload)
				d.Unlock()
			}
		}
	}()

	return d
}

func (d *Single) Add(oss ...DebounceOptionSetter) {
	o := gatherOptions(oss)

	d.Lock()
	d.payload = o.payload
	d.timer.Reset(d.period)
	d.Unlock()
}

type Deduper struct {
	sync.Mutex
	config

	payload map[string][]byte
	timer   *time.Timer
}

func NewDeduper(ctx context.Context, css ...ConfigSetter) *Deduper {
	cfg := gatherConfig(css)
	d := &Deduper{config: *cfg}
	d.payload = make(map[string][]byte)
	d.timer = time.NewTimer(d.period)
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

				if d.useSingleAction && len(payload) == 1 {
					d.singleAction(payload[0])
				} else {
					d.batchAction(payload)
				}

				d.Unlock()
			}
		}
	}()

	return d
}

func (d *Deduper) Add(oss ...DebounceOptionSetter) {
	o := gatherOptions(oss)

	d.Lock()
	d.payload[o.key] = o.payload
	d.timer.Reset(d.period)
	d.Unlock()
}

type Limiter struct {
	sync.Mutex
	config

	timer        *time.Timer
	buffer       [][]byte
	bufferOffset int
}

func NewLimiter(ctx context.Context, css ...ConfigSetter) *Limiter {
	cfg := gatherConfig(css)
	d := &Limiter{config: *cfg}
	d.timer = time.NewTimer(d.period)
	d.timer.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-d.timer.C:
				d.Lock()
				if d.bufferOffset > 0 {
					if d.useSingleAction && len(d.buffer) == 1 {
						d.singleAction(d.buffer[0])
					} else {
						d.batchAction(d.buffer)
					}

					d.buffer = d.buffer[:0]
					d.bufferOffset = 0
				}
				d.Unlock()
			}
		}
	}()

	return d
}

func (d *Limiter) Add(oss ...DebounceOptionSetter) {
	o := gatherOptions(oss)

	d.Lock()
	if d.bufferOffset >= d.bufferLimit {
		d.batchAction(d.buffer)
		d.buffer = d.buffer[:0]
		d.bufferOffset = 0
	}

	d.timer.Reset(d.period)

	d.buffer = append(d.buffer, o.payload)
	d.bufferOffset += len(o.payload)

	d.Unlock()
}
