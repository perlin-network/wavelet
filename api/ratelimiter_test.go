package api

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestRateLimit(t *testing.T) {
	rl := newRatelimiter(2)
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
