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
)

var (
	_ Debouncer = (*Dummy)(nil)
	_ Debouncer = (*Deduper)(nil)
	_ Debouncer = (*Limiter)(nil)
)

type config struct {
	action      func([][]byte)
	period      time.Duration
	bufferLimit int
}

type ConfigSetter func(cfg *config)

func gatherConfig(css []ConfigSetter) *config {
	cfg := &config{
		action:      func([][]byte) {},
		period:      50 * time.Millisecond,
		bufferLimit: 16384,
	}

	for _, cs := range css {
		cs(cfg)
	}

	return cfg
}

func WithAction(action func([][]byte)) ConfigSetter {
	return func(cfg *config) {
		cfg.action = action
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
	payload  []byte
	groupKey string
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

func WithGroupKey(k string) DebounceOptionSetter {
	return func(o *options) {
		o.groupKey = k
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

	if d.action != nil {
		d.action([][]byte{o.payload})
	}
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
				d.action(payload)
				d.Unlock()
			}
		}
	}()

	return d
}

func (d *Deduper) Add(oss ...DebounceOptionSetter) {
	o := gatherOptions(oss)

	d.Lock()
	d.payload[o.groupKey] = o.payload
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
					d.action(d.buffer)
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
		d.action(d.buffer)
		d.buffer = d.buffer[:0]
		d.bufferOffset = 0
	}

	d.timer.Reset(d.period)

	d.buffer = append(d.buffer, o.payload)
	d.bufferOffset += len(o.payload)

	d.Unlock()
}
