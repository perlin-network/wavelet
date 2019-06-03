package api

import (
	"crypto/rand"
	"github.com/fasthttp/websocket"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"net/url"
	"testing"
)

func TestPollTx(t *testing.T) {
	gateway := New()
	gateway.setup()

	gateway.ledger = createLedger(t)

	// Create a transaction
	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	var buf [200]byte
	_, err = rand.Read(buf[:])
	assert.NoError(t, err)
	_ = wavelet.NewTransaction(keys, sys.TagTransfer, buf[:])
	assert.NoError(t, err)

	t.Run("tag-filter", func(t *testing.T) {
		u := url.URL{Scheme: "ws", Host: "localhost:8080", Path:`/poll/tx?tag=1"`}
		c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if !assert.NoError(t, err) {
			return
		}

		response := make(chan []byte, 1)
		go func() {
			_, msg, err := c.ReadMessage()
			if !assert.NoError(t, err) {
				return
			}
			response <-  msg
		}()



		assert.Equal(t, 1, len(response))
	})

}