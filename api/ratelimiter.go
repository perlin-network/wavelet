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

package api

import (
	"github.com/valyala/fasthttp"
	"golang.org/x/time/rate"
	"math"
	"net/http"
	"sync"
	"time"
)

type limiter struct {
	key      string
	limiter  *rate.Limiter
	lastSeen int64 // nanoseconds
}

type rateLimiter struct {
	// Rate per second
	max float64

	// Determine how long (since lastSeen) should the limiter be kept in the map.
	expirationTTL time.Duration

	limiters map[string]*limiter
	sync.RWMutex
}

func newRateLimiter(maxPerSec float64) *rateLimiter {
	return &rateLimiter{
		max:           maxPerSec,
		expirationTTL: 1 * time.Second,
		limiters:      make(map[string]*limiter),
	}
}

// Return the rate limiter for the key if it
// already exists. Otherwise, create a new one.
func (r *rateLimiter) getLimiter(key string) *limiter {
	r.Lock()
	defer r.Unlock()

	v := r.limiters[key]
	if v != nil {
		// Update the last seen time for the limiter.
		v.lastSeen = time.Now().UnixNano()

		return v
	}

	l := rate.NewLimiter(rate.Limit(r.max), int(math.Max(1, r.max)))
	v = &limiter{key, l, time.Now().UnixNano()}

	r.limiters[key] = v

	return v
}

// At every interval, check the map for limiters that haven't been seen for
// more than the expiry duration and delete the entries.
func (r *rateLimiter) cleanup(interval time.Duration) (stop func()) {
	done := make(chan struct{})
	stop = func() {
		close(done)
	}

	ticker := time.NewTicker(interval)

	ttl := r.expirationTTL.Nanoseconds()

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				now := time.Now().UnixNano()

				r.Lock()
				for ip, v := range r.limiters {
					if now-v.lastSeen > ttl {
						delete(r.limiters, ip)
					}
				}
				r.Unlock()
			}
		}
	}()

	return
}

// Apply rate limiting by key and IP
func (r *rateLimiter) limit(key string) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		fn := func(ctx *fasthttp.RequestCtx) {
			addr := ctx.RemoteAddr().String()

			l := r.getLimiter(key + addr)

			if !l.limiter.Allow() {
				ctx.Error(http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}

			next(ctx)
		}
		return fasthttp.RequestHandler(fn)
	}
}
