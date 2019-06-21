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

package debounce

import (
	"context"
	"github.com/valyala/fastjson"
	"sync"
	"time"
)

type Type int

const (
	TypeDeduper Type = iota
	TypeLimiter
)

type Factory struct {
	t    Type
	opts []ConfigOption
}

func NewFactory(t Type, opts ...ConfigOption) *Factory {
	return &Factory{t: t, opts: opts}
}

func (f Factory) Init(ctx context.Context, opts ...ConfigOption) Debouncer {
	switch f.t {
	case TypeDeduper:
		return NewDeduper(ctx, append(opts, f.opts...)...)
	case TypeLimiter:
		return NewLimiter(ctx, append(opts, f.opts...)...)
	}

	panic("Invalid debouncer type was specified.")
}

type Debouncer interface {
	Add(...PayloadOption)
}

var (
	_ Debouncer = (*Deduper)(nil)
	_ Debouncer = (*Limiter)(nil)
)

type Deduper struct {
	mu sync.Mutex
	Config

	set   map[string][]byte
	timer *time.Timer
}

func NewDeduper(ctx context.Context, opts ...ConfigOption) *Deduper {
	cfg := parseOptions(opts)

	d := &Deduper{
		Config: *cfg,
		set:    make(map[string][]byte),
		timer:  time.NewTimer(cfg.period),
	}
	d.timer.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-d.timer.C:
				d.mu.Lock()
				payload := make([][]byte, 0, len(d.set))
				for _, v := range d.set {
					payload = append(payload, v)
				}

				action := d.action
				d.mu.Unlock()
				go action(payload)
			}
		}
	}()

	return d
}

func (d *Deduper) Add(oss ...PayloadOption) {
	o := parsePayload(oss)

	if o.buf == nil {
		return
	}

	key := ""

	for _, k := range d.keys {
		key += fastjson.GetString(o.buf, k)
	}

	d.mu.Lock()
	d.set[key] = o.buf
	d.timer.Reset(d.period)
	d.mu.Unlock()
}

type Limiter struct {
	mu sync.Mutex
	Config

	timer        *time.Timer
	buffer       [][]byte
	bufferOffset int
}

func NewLimiter(ctx context.Context, opts ...ConfigOption) *Limiter {
	cfg := parseOptions(opts)
	d := &Limiter{
		Config: *cfg,
		timer:  time.NewTimer(cfg.period),
	}
	d.timer.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-d.timer.C:
				var buffer [][]byte
				var action func([][]byte)

				d.mu.Lock()
				if d.bufferOffset > 0 {
					action = d.action
					buffer = d.buffer
					d.buffer = nil
					d.bufferOffset = 0
				}
				d.mu.Unlock()
				if action != nil {
					go action(buffer)
				}
			}
		}
	}()

	return d
}

func (d *Limiter) Add(oss ...PayloadOption) {
	o := parsePayload(oss)

	if o.buf == nil {
		return
	}

	var buffer [][]byte
	var action func([][]byte)

	d.mu.Lock()
	if d.bufferOffset >= d.bufferLimit {
		action = d.action
		buffer = d.buffer
		d.buffer = nil
		d.bufferOffset = 0
	}

	d.timer.Reset(d.period)

	d.buffer = append(d.buffer, o.buf)
	d.bufferOffset += len(o.buf)

	d.mu.Unlock()

	if action != nil {
		go action(buffer)
	}
}
