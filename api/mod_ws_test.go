package api

import (
	"github.com/fasthttp/websocket"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestPollLog(t *testing.T) {
	gateway := New()
	gateway.setup()

	log.Set("ws", gateway)

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ledger := wavelet.NewLedger(store.NewInmem(), skademlia.NewClient(":0", keys))

	go gateway.StartHTTP(8080, nil, ledger, keys)
	defer gateway.Shutdown()

	time.Sleep(100 * time.Millisecond)

	t.Run("tx-tag-filter", func(t *testing.T) {
		u := url.URL{Scheme: "ws", Host: ":8080", Path: `/poll/tx`, RawQuery: "tag=1"}
		c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if !assert.NoError(t, err) {
			return
		}

		response := make(chan []byte, 10)
		stop := make(chan struct{})
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
				}

				_, msg, err := c.ReadMessage()
				if !assert.NoError(t, err) {
					return
				}
				response <- msg
			}
		}()

		// log 2 messages with different tags
		logger := log.TX("test")
		logger.Log().Uint8("tag", sys.TagTransfer).Msg("")

		logger.Log().Uint8("tag", sys.TagStake).Msg("")

		time.Sleep(100 * time.Millisecond)

		close(stop)

		assert.Equal(t, 1, len(response))
	})

	t.Run("accounts-grouping", func(t *testing.T) {
		u := url.URL{Scheme: "ws", Host: ":8080", Path: `/poll/accounts`}
		c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if !assert.NoError(t, err) {
			return
		}

		response := make(chan []byte, 10)
		stop := make(chan struct{})
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
				}

				_, msg, err := c.ReadMessage()
				if !assert.NoError(t, err) {
					return
				}
				response <- msg
			}
		}()

		// log 2 messages with different tags
		logger := log.Accounts("test")
		id := ""
		for i := 0; i < 10; i++ {
			if i%5 == 0 {
				id = strconv.Itoa(i)
			}
			logger.Log().Str("account_id", id).Msg("")
		}

		time.Sleep(200 * time.Millisecond)

		close(stop)

		assert.Equal(t, 2, len(response))
	})
}
