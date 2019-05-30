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

func newRatelimiter(maxPerSec float64) *rateLimiter {
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
