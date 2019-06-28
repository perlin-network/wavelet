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
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestRateLimit(t *testing.T) {
	rl := newRateLimiter(2)
	rl.expirationTTL = 30 * time.Millisecond

	// check existing limited should be returned

	l := rl.getLimiter("key1")
	assert.Equal(t, rl.getLimiter("key1"), l)

	// check lastSeen should be updated

	now := time.Now().UnixNano()

	time.Sleep(10 * time.Millisecond)

	l = rl.getLimiter("key1")
	assert.True(t, l.lastSeen > now)

	// check cleanup

	done := make(chan struct{})
	go func() {
		stop := rl.cleanup(30 * time.Millisecond)
		time.Sleep(100 * time.Millisecond)
		stop()

		close(done)
	}()
	<-done
	assert.Nil(t, rl.limiters["key1"])
}
