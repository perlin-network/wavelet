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
	"github.com/fasthttp/websocket"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestPollLog(t *testing.T) {
	gateway := New()
	gateway.setup()

	log.SetWriter(log.LoggerWebsocket, gateway)

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ledger := wavelet.NewLedger(store.NewInmem(), skademlia.NewClient(":0", keys), nil)

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

		time.Sleep(2500 * time.Millisecond)

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

		// log bunch of messages but only about 2 accounts
		logger := log.Accounts("test")
		id := ""
		for i := 0; i < 10; i++ {
			if i%5 == 0 {
				id = strconv.Itoa(i)
			}
			logger.Log().Str("account_id", id).Msg("")
		}

		time.Sleep(1000 * time.Millisecond)
		close(stop)

		if !assert.Equal(t, 1, len(response)) {
			return
		}

		v, err := fastjson.Parse(string(<-response))
		if !assert.NoError(t, err) {
			return
		}

		vals, err := v.Array()
		if !assert.NoError(t, err) {
			return
		}

		assert.Equal(t, 2, len(vals))
	})
}
