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

// +build integration,!unit

package api

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
)

func TestPollLog(t *testing.T) {
	gateway := New()
	gateway.setup()

	log.SetWriter(log.LoggerWebsocket, gateway)

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	kv := store.NewInmem()
	ledger := wavelet.NewLedger(kv, skademlia.NewClient(":0", keys))

	go gateway.StartHTTP(8080, nil, ledger, keys, kv)
	defer gateway.Shutdown()

	t.Run("tx-tag-filter", func(t *testing.T) {
		u := url.URL{Scheme: "ws", Host: ":8080", Path: `/poll/tx`, RawQuery: "tag=1"}
		c, cleanup := tryConnectWebsocket(t, u)
		defer cleanup()

		// log 2 messages with different tags
		logger := log.TX("test")
		logger.Log().Uint8("tag", byte(sys.TagTransfer)).Msg("")
		logger.Log().Uint8("tag", byte(sys.TagStake)).Msg("")

		messages := readAllMessages(t, c, 1)
		assert.Equal(t, 1, len(messages))
	})

	t.Run("accounts-grouping", func(t *testing.T) {
		u := url.URL{Scheme: "ws", Host: ":8080", Path: `/poll/accounts`}
		c, cleanup := tryConnectWebsocket(t, u)
		defer cleanup()

		// log bunch of messages but only about 2 accounts
		logger := log.Accounts("test")
		id := ""
		for i := 0; i < 10; i++ {
			if i%5 == 0 {
				id = strconv.Itoa(i)
			}
			logger.Log().Str("account_id", id).Msg("")
		}

		messages := readAllMessages(t, c, 1)
		if !assert.Equal(t, 1, len(messages)) {
			return
		}

		v, err := fastjson.Parse(string(messages[0]))
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

func TestPollNonce(t *testing.T) {
	testnet := wavelet.NewTestNetwork(t)
	defer testnet.Cleanup()

	alice := testnet.AddNode(t)
	testnet.AddNode(t)

	testnet.WaitUntilSync(t)

	gateway := New()
	gateway.setup()

	go gateway.StartHTTP(8080, nil, alice.Ledger(), alice.Keys(), alice.KV())
	defer gateway.Shutdown()

	_, err := testnet.Faucet().Pay(alice, 1000000)
	if !assert.NoError(t, err) {
		return
	}

	u := url.URL{
		Scheme: "ws",
		Host:   ":8080",
		Path:   "/poll/accounts",
	}

	c, cleanup := tryConnectWebsocket(t, u)
	defer cleanup()

	_, msg, err := c.ReadMessage()
	if !assert.NoError(t, err) {
		return
	}

	v, err := fastjson.Parse(string(msg))
	if !assert.NoError(t, err) {
		return
	}

	vals, err := v.Array()
	if !assert.NoError(t, err) {
		return
	}

	for _, e := range vals {
		if string(e.GetStringBytes("event")) != "nonce_updated" {
			continue
		}

		assert.EqualValues(t, 2, e.GetUint64("nonce"))
		return
	}

	assert.Fail(t, "nonce_updated event not found")
}

func tryConnectWebsocket(t *testing.T, url url.URL) (*websocket.Conn, func()) {
	var conn *websocket.Conn
	var resp *http.Response
	var err error

	tries := 0
	tick := time.Tick(time.Second * 1)
	for range tick {
		conn, resp, err = websocket.DefaultDialer.Dial(url.String(), nil)
		if err == nil || (err != nil && tries >= 5) {
			break
		}

		tries++
	}

	assert.NoError(t, err)

	return conn, func() {
		_ = resp.Body.Close()
		_ = conn.Close()
	}
}

func readAllMessages(t *testing.T, client *websocket.Conn, n int) (messages [][]byte) {
	messages = make([][]byte, 0)

	ch := make(chan []byte, 10)
	go func() {
		for {
			_, msg, err := client.ReadMessage()
			if err != nil {
				return
			}

			ch <- msg
		}
	}()

	for {
		select {
		case msg := <-ch:
			messages = append(messages, msg)
			if len(messages) == n {
				return
			}

		case <-time.After(10 * time.Second):
			return
		}
	}
}
