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
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/handshake"
	"github.com/perlin-network/wavelet/conf"

	"github.com/buaazp/fasthttprouter"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

func TestListTransaction(t *testing.T) {
	gateway := New()
	gateway.setup()

	gateway.ledger = createLedger(t)

	// Create a transaction
	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	var buf [200]byte
	_, err = rand.Read(buf[:])
	assert.NoError(t, err)

	tx := newTransaction(keys, sys.TagTransfer, 0, 0, buf[:])
	gateway.ledger.AddTransaction(tx)

	// Build an expected response
	var expectedResponse transactionList

	gateway.ledger.Transactions().Iterate(func(tx *wavelet.Transaction) bool {
		txRes := &transaction{tx: tx}
		txRes.status = "applied"

		//_, err := txRes.marshal()
		//assert.NoError(t, err)
		expectedResponse = append(expectedResponse, txRes)
		return true
	})

	tests := []struct {
		name         string
		url          string
		wantCode     int
		wantResponse marshalableJSON
	}{
		{
			name:     "sender not hex",
			url:      "/tx?sender=1",
			wantCode: http.StatusBadRequest,
			wantResponse: testErrResponse{
				StatusText: "Bad Request",
				ErrorText:  "sender ID must be presented as valid hex: encoding/hex: odd length hex string",
			},
		},
		{
			name:     "sender invalid length",
			url:      "/tx?sender=746c703579786279793638626e726a77666574656c6d34386d6739306b7166306565",
			wantCode: http.StatusBadRequest,
			wantResponse: testErrResponse{
				StatusText: "Bad Request",
				ErrorText:  "sender ID must be 32 bytes long",
			},
		},
		{
			name:     "offset negative invalid",
			url:      "/tx?offset=-1",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "limit negative invalid",
			url:      "/tx?limit=-1",
			wantCode: http.StatusBadRequest,
		},
		{
			name:         "success",
			url:          "/tx?limit=1&offset=0",
			wantCode:     http.StatusOK,
			wantResponse: expectedResponse,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest("GET", "http://localhost"+tc.url, nil)
			assert.NoError(t, err)

			w, err := serve(gateway.router, request)
			assert.NoError(t, err)
			assert.NotNil(t, w)

			defer w.Body.Close()

			assert.NoError(t, err)
			assert.NotNil(t, w)

			response, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.StatusCode, "status code")

			if tc.wantResponse != nil {
				r, err := tc.wantResponse.marshalJSON(new(fastjson.ArenaPool).Get())
				assert.NoError(t, err)
				assert.Equal(t, string(r), string(bytes.TrimSpace(response)))
			}
		})
	}
}

func TestGetTransaction(t *testing.T) {
	gateway := New()
	gateway.setup()

	gateway.ledger = createLedger(t)

	// Create a transaction
	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	var buf [200]byte
	_, err = rand.Read(buf[:])
	assert.NoError(t, err)

	gateway.ledger.AddTransaction(newTransaction(keys, sys.TagTransfer, 0, 0, buf[:]))

	var txID wavelet.TransactionID
	gateway.ledger.Transactions().Iterate(func(tx *wavelet.Transaction) bool {
		txID = tx.ID
		return false
	})

	tx := gateway.ledger.Transactions().Find(txID)
	if tx == nil {
		t.Fatal("not found")
	}

	txRes := &transaction{tx: tx}
	txRes.status = "applied"

	tests := []struct {
		name         string
		id           string
		wantCode     int
		wantResponse marshalableJSON
	}{
		{
			name:     "invalid id length",
			id:       "1c331c1d",
			wantCode: http.StatusBadRequest,
			wantResponse: &testErrResponse{
				StatusText: "Bad Request",
				ErrorText:  fmt.Sprintf("transaction ID must be %d bytes long", wavelet.SizeTransactionID),
			},
		},
		{
			name:         "success",
			id:           hex.EncodeToString(txID[:]),
			wantCode:     http.StatusOK,
			wantResponse: txRes,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest("GET", "http://localhost/tx/"+tc.id, nil)
			assert.NoError(t, err)

			w, err := serve(gateway.router, request)
			assert.NoError(t, err)
			assert.NotNil(t, w)

			defer w.Body.Close()

			response, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.StatusCode, "status code")

			if tc.wantResponse != nil {
				r, err := tc.wantResponse.marshalJSON(new(fastjson.ArenaPool).Get())
				assert.Nil(t, err)
				assert.Equal(t, string(r), string(bytes.TrimSpace(response)))
			}
		})
	}
}

func TestSendTransaction(t *testing.T) {
	gateway := New()
	gateway.setup()

	tests := []struct {
		name     string
		wantCode int
		req      sendTransactionRequest
	}{
		{
			name:     "ok",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tc.req)
			assert.NoError(t, err)

			request := httptest.NewRequest("POST", "http://localhost/tx/send", bytes.NewReader(reqBody))

			w, err := serve(gateway.router, request)
			if !assert.NoError(t, err) || !assert.NotNil(t, w) {
				return
			}

			defer func() {
				_ = w.Body.Close()
			}()

			_, err = ioutil.ReadAll(w.Body)
			assert.Nil(t, err)

			assert.Equal(t, tc.wantCode, w.StatusCode, "status code")
		})
	}
}

func TestSendTransactionRandom(t *testing.T) {
	gateway := New()
	gateway.setup()

	type request struct {
		Sender    string `json:"sender"`
		Tag       byte   `json:"tag"`
		Payload   string `json:"payload"`
		Signature string `json:"signature"`
	}

	f := func(req request) bool {
		reqBody, err := json.Marshal(req)
		assert.NoError(t, err)

		request := httptest.NewRequest("POST", "http://localhost/tx/send", bytes.NewReader(reqBody))

		res, err := serve(gateway.router, request)
		if !assert.NoError(t, err) || !assert.NotNil(t, res) {
			return false
		}

		defer func() {
			_ = res.Body.Close()
		}()

		assert.NotEqual(t, http.StatusNotFound, res.StatusCode)

		return true
	}

	if err := quick.Check(f, &quick.Config{
		MaxCountScale: 10,
	}); err != nil {
		t.Error(err)
	}
}

// Test POST APIs with completely random payload
func TestPostPayloadRandom(t *testing.T) {
	gateway := New()
	gateway.setup()

	tests := []struct {
		url string
	}{
		{
			url: "/tx/send",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.url, func(t *testing.T) {
			f := func(random1 [][]byte, random2 [][]byte) bool {
				var payload []byte

				for i := range random1 {
					payload = append(payload, random1[i]...)
				}

				for i := range random2 {
					payload = append(payload, random2[i]...)
				}

				request := httptest.NewRequest("POST", "http://localhost"+tc.url, bytes.NewReader(payload))

				res, err := serve(gateway.router, request)
				if !assert.NoError(t, err) || !assert.NotNil(t, res) {
					return false
				}

				defer func() {
					_ = res.Body.Close()
				}()

				assert.NotEmpty(t, res.StatusCode)

				return true
			}

			if err := quick.Check(f, &quick.Config{
				MaxCountScale: 10,
			}); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestGetAccount(t *testing.T) {
	gateway := New()
	gateway.setup()

	gateway.ledger = createLedger(t)

	idHex := "1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d"
	idBytes, err := hex.DecodeString(idHex)
	assert.NoError(t, err)

	var id32 wavelet.AccountID
	copy(id32[:], idBytes)

	wavelet.WriteAccountBalance(gateway.ledger.Snapshot(), id32, 10)
	wavelet.WriteAccountStake(gateway.ledger.Snapshot(), id32, 11)
	wavelet.WriteAccountContractNumPages(gateway.ledger.Snapshot(), id32, 12)

	var id wavelet.AccountID
	copy(id[:], idBytes)

	tests := []struct {
		name         string
		url          string
		wantCode     int
		wantResponse marshalableJSON
	}{
		{
			name:     "missing id",
			url:      "/accounts/",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "id not hex",
			url:      "/accounts/-----",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid id length",
			url:      "/accounts/1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d",
			wantCode: http.StatusBadRequest,
		},
		{
			name:         "valid id",
			url:          "/accounts/" + idHex,
			wantCode:     http.StatusOK,
			wantResponse: &account{ledger: gateway.ledger, id: id},
		},
	}

	for _, tc := range tests { // nolint:dupl
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://localhost"+tc.url, nil)

			w, err := serve(gateway.router, request)
			if !assert.NoError(t, err) || !assert.NotNil(t, w) {
				return
			}

			defer func() {
				_ = w.Body.Close()
			}()

			response, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.StatusCode, "status code")

			if tc.wantResponse != nil {
				r, err := tc.wantResponse.marshalJSON(new(fastjson.ArenaPool).Get())
				assert.Nil(t, err)
				assert.Equal(t, string(r), string(bytes.TrimSpace(response)))
			}
		})
	}
}

func TestGetContractCode(t *testing.T) {
	gateway := New()
	gateway.setup()

	gateway.ledger = createLedger(t)

	idHex := "1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d"
	idBytes, err := hex.DecodeString(idHex)
	assert.NoError(t, err)

	var id32 wavelet.AccountID
	copy(id32[:], idBytes)

	s := gateway.ledger.Snapshot()
	wavelet.WriteAccountContractCode(s, id32, []byte("contract code"))

	tests := []struct {
		name      string
		url       string
		wantCode  int
		wantError marshalableJSON
	}{
		{
			name:     "missing id",
			url:      "/contract/",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "id not hex",
			url:      "/contract/-----",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid id length",
			url:      "/contract/1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "id not exist",
			url:      "/contract/" + "3132333435363738393031323334353637383930313233343536373839303132",
			wantCode: http.StatusNotFound,
			wantError: testErrResponse{
				StatusText: "Not Found",
				ErrorText:  fmt.Sprintf("could not find contract with ID %s", "3132333435363738393031323334353637383930313233343536373839303132"),
			},
		},
	}

	for _, tc := range tests { // nolint:dupl
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://localhost"+tc.url, nil)

			w, err := serve(gateway.router, request)
			if !assert.NoError(t, err) || !assert.NotNil(t, w) {
				return
			}

			defer func() {
				_ = w.Body.Close()
			}()

			response, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.StatusCode, "status code")

			if tc.wantError != nil {
				r, err := tc.wantError.marshalJSON(new(fastjson.ArenaPool).Get())
				assert.Nil(t, err)
				assert.Equal(t, string(r), string(bytes.TrimSpace(response)))
			}
		})
	}
}

func TestGetContractPages(t *testing.T) {
	gateway := New()
	gateway.setup()

	gateway.ledger = createLedger(t)

	// string: u1mf2g3b2477y5btco22txqxuc41cav6
	var id = "75316d66326733623234373779356274636f3232747871787563343163617636"

	tests := []struct {
		name      string
		url       string
		wantCode  int
		wantError marshalableJSON
	}{
		{
			name:     "page not uint",
			url:      "/contract/" + id + "/page/-1",
			wantCode: http.StatusBadRequest,
			wantError: testErrResponse{
				StatusText: "Bad Request",
				ErrorText:  "could not parse page index",
			},
		},
		{
			name:     "id not exist",
			url:      "/contract/3132333435363738393031323334353637383930313233343536373839303132/page/1",
			wantCode: http.StatusNotFound,
			wantError: testErrResponse{
				StatusText: "Not Found",
				ErrorText:  fmt.Sprintf("could not find any pages for contract with ID %s", "3132333435363738393031323334353637383930313233343536373839303132"),
			},
		},
	}

	for _, tc := range tests { // nolint:dupl
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://localhost"+tc.url, nil)

			w, err := serve(gateway.router, request)
			if !assert.NoError(t, err) || !assert.NotNil(t, w) {
				return
			}

			defer func() {
				_ = w.Body.Close()
			}()

			response, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.StatusCode, "status code")

			if tc.wantError != nil {
				r, err := tc.wantError.marshalJSON(new(fastjson.ArenaPool).Get())
				assert.Nil(t, err)
				assert.Equal(t, string(r), string(bytes.TrimSpace(response)))
			}
		})
	}
}

func TestGetLedger(t *testing.T) {
	gateway := New()
	gateway.setup()

	gateway.ledger = createLedger(t)

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)
	gateway.keys = keys

	listener, err := net.Listen("tcp", ":0") // nolint:gosec
	assert.NoError(t, err)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))

	gateway.client = skademlia.NewClient(addr, keys,
		skademlia.WithC1(sys.SKademliaC1),
		skademlia.WithC2(sys.SKademliaC2),
	)

	request := httptest.NewRequest("GET", "http://localhost/ledger", nil)

	w, err := serve(gateway.router, request)
	if !assert.NoError(t, err) || !assert.NotNil(t, w) {
		return
	}

	defer func() {
		_ = w.Body.Close()
	}()

	response, err := ioutil.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, w.StatusCode)

	publicKey := keys.PublicKey()

	expectedJSON := fmt.Sprintf(
		`{"public_key":"%s","address":"127.0.0.1:%d","num_accounts":3,"preferred_votes":0,"sync_status":"Node is taking part in consensus process","block":{"merkle_root":"cd3b0df841268ab6c987a594de29ad19","height":0,"id":"4b35bb7ead7ac9a0e7064c6d99342962360854308f84dde9fe0703cd7772a94d","transactions":0},"preferred":null,"num_missing_tx":0,"num_tx":0,"num_tx_in_store":0,"num_accounts_in_store":3,"peers":null}`,
		hex.EncodeToString(publicKey[:]),
		listener.Addr().(*net.TCPAddr).Port,
	)

	assert.NoError(t, compareJSON([]byte(expectedJSON), response))
}

// Test the rate limit on all endpoints
func TestEndpointsRateLimit(t *testing.T) {
	gateway := New()
	gateway.rateLimiter = newRateLimiter(10)
	gateway.setup()

	gateway.ledger = createLedger(t)

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)
	gateway.keys = keys

	listener, err := net.Listen("tcp", ":0") // nolint:gosec
	assert.NoError(t, err)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))

	gateway.client = skademlia.NewClient(addr, keys,
		skademlia.WithC1(sys.SKademliaC1),
		skademlia.WithC2(sys.SKademliaC2),
	)

	tests := []struct {
		url           string
		method        string
		isRateLimited bool
	}{
		{
			url:           "/poll/network",
			method:        "GET",
			isRateLimited: true,
		},
		{
			url:           "/poll/consensus",
			method:        "GET",
			isRateLimited: true,
		},
		{
			url:           "/poll/stake",
			method:        "GET",
			isRateLimited: true,
		},
		{
			url:           "/poll/accounts",
			method:        "GET",
			isRateLimited: true,
		},
		{
			url:           "/poll/contract",
			method:        "GET",
			isRateLimited: true,
		},
		{
			url:           "/poll/tx",
			method:        "GET",
			isRateLimited: true,
		},
		{
			url:           "/poll/metrics",
			method:        "GET",
			isRateLimited: true,
		},
		{
			url:           "/ledger",
			method:        "GET",
			isRateLimited: true,
		},
		{
			url:           "/accounts/1",
			method:        "GET",
			isRateLimited: false,
		},
		{
			url:           "/contract/1/page/1",
			method:        "GET",
			isRateLimited: true,
		},
		{
			url:           "/contract/1/page",
			method:        "GET",
			isRateLimited: true,
		},
		{
			url:           "/contract/1",
			method:        "GET",
			isRateLimited: true,
		},
		{
			url:           "/tx/send",
			method:        "POST",
			isRateLimited: false,
		},
		{
			url:           "/tx/1",
			method:        "GET",
			isRateLimited: false,
		},
		{
			url:           "/tx",
			method:        "GET",
			isRateLimited: true,
		},
	}

	maxPerSecond := 10

	for _, tc := range tests {
		tc := tc
		t.Run(tc.url, func(t *testing.T) {
			// We assume the loop will complete in less than 1 second.
			for i := 0; i < maxPerSecond*2; i++ {
				request := httptest.NewRequest(tc.method, "http://localhost"+tc.url, nil)
				w, err := serve(gateway.router, request)
				if !assert.NoError(t, err) || !assert.NotNil(t, w) {
					return
				}

				defer func() {
					_ = w.Body.Close()
				}()

				_, err = ioutil.ReadAll(w.Body)
				assert.NoError(t, err)

				if tc.isRateLimited {
					if i < maxPerSecond {
						assert.NotEqual(t, http.StatusTooManyRequests, w.StatusCode)
					} else {
						assert.Equal(t, http.StatusTooManyRequests, w.StatusCode)
					}
				} else {
					assert.NotEqual(t, http.StatusTooManyRequests, w.StatusCode)
				}
			}
		})
	}
}

func TestConnectDisconnect(t *testing.T) {
	network := wavelet.NewTestNetwork(t)
	defer network.Cleanup()

	gateway := New()
	gateway.setup()
	gateway.ledger = network.Faucet().Ledger()
	gateway.client = network.Faucet().Client()

	network.AddNode(t)

	node := network.AddNode(t)

	network.WaitForSync(t)

	currentSecret := conf.GetSecret()
	defer conf.Update(conf.WithSecret(currentSecret))
	conf.Update(conf.WithSecret("secret"))

	body := fmt.Sprintf(`{"address": "%s"}`, node.Addr())

	request := httptest.NewRequest(http.MethodPost, "http://localhost/node/connect", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer secret")
	w, err := serve(gateway.router, request)
	if !assert.NoError(t, err) || !assert.NotNil(t, w) {
		return
	}

	defer func() {
		_ = w.Body.Close()
	}()

	assert.Equal(t, http.StatusOK, w.StatusCode)

	resp, err := ioutil.ReadAll(w.Body)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf(`{"msg":"Successfully connected to %s"}`, node.Addr()), string(resp))

	request = httptest.NewRequest(http.MethodPost, "http://localhost/node/disconnect", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer secret")
	w, err = serve(gateway.router, request)
	assert.NoError(t, err)

	assert.NotNil(t, w)

	assert.Equal(t, http.StatusOK, w.StatusCode)

	resp, err = ioutil.ReadAll(w.Body)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf(`{"msg":"Successfully disconnected from %s"}`, node.Addr()), string(resp))
}

func TestConnectDisconnectErrors(t *testing.T) {
	gateway := New()
	gateway.setup()

	gateway.ledger = createLedger(t)

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)
	gateway.keys = keys

	l, err := net.Listen("tcp", ":0") // nolint:gosec
	if !assert.NoError(t, err) {
		return
	}

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(l.Addr().(*net.TCPAddr).Port))

	gateway.client = skademlia.NewClient(addr, keys,
		skademlia.WithC1(sys.SKademliaC1),
		skademlia.WithC2(sys.SKademliaC2),
	)
	gateway.client.SetCredentials(
		noise.NewCredentials(addr, handshake.NewECDH(), cipher.NewAEAD(), gateway.client.Protocol()),
	)

	currentSecret := conf.GetSecret()
	defer conf.Update(conf.WithSecret(currentSecret))
	conf.Update(conf.WithSecret("secret"))

	authHeader := "Bearer secret"

	testCases := []struct {
		name       string
		uri        string
		body       string
		authHeader string
		errorStr   string
		code       int
	}{
		{
			name:     "error: connect address is missing",
			uri:      "/node/connect",
			body:     "{}",
			errorStr: `Unauthorized`,
			code:     http.StatusUnauthorized,
		},
		{
			name:       "error: disconnect address is missing",
			uri:        "/node/disconnect",
			authHeader: authHeader,
			body:       "{}",
			errorStr:   `{"status":"Bad Request","error":"address is missing"}`,
			code:       http.StatusBadRequest,
		},
		{
			name:       "error: connect address is malformed",
			uri:        "/node/connect",
			authHeader: authHeader,
			body:       `{"address":"aaa"}`,
			errorStr:   `{"status":"Internal Server Error","error":"error connecting to peer: connection could not be identified: failed to ping peer: rpc error: code = Unavailable desc = all SubConns are in TransientFailure, latest connection error: connection error: desc = \"transport: error while dialing: dial tcp: address aaa: missing port in address\""}`,
			code:       http.StatusInternalServerError,
		},
		{
			name:       "error: disconnect address is malformed",
			uri:        "/node/disconnect",
			authHeader: authHeader,
			body:       `{"address":"aaa"}`,
			errorStr:   `{"status":"Internal Server Error","error":"error disconnecting from peer: could not disconnect peer: peer with address aaa not found"}`,
			code:       http.StatusInternalServerError,
		},
		{
			name:       "error: connect address is missing",
			uri:        "/node/connect",
			authHeader: authHeader,
			body:       `{"address":"127.0.0.1:1234"}`,
			errorStr:   `{"status":"Internal Server Error","error":"error connecting to peer: connection could not be identified: failed to ping peer: rpc error: code = Unavailable desc = all SubConns are in TransientFailure, latest connection error: connection error: desc = \"transport: error while dialing: dial tcp 127.0.0.1:1234: connect: connection refused\""}`,
			code:       http.StatusInternalServerError,
		},
		{
			name:       "error: disconnect address is missing",
			uri:        "/node/disconnect",
			authHeader: authHeader,
			body:       `{"address":"127.0.0.1:1234"}`,
			errorStr:   `{"status":"Internal Server Error","error":"error disconnecting from peer: could not disconnect peer: peer with address 127.0.0.1:1234 not found"}`,
			code:       http.StatusInternalServerError,
		},
	}

	for _, testCase := range testCases {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://localhost"+tc.uri, strings.NewReader(tc.body))

			if len(tc.authHeader) > 0 {
				request.Header.Set("Authorization", tc.authHeader)
			}

			w, err := serve(gateway.router, request)
			if !assert.NoError(t, err) || !assert.NotNil(t, w) {
				return
			}

			defer func() {
				_ = w.Body.Close()
			}()

			resp, err := ioutil.ReadAll(w.Body)
			if !assert.NoError(t, err) {
				return
			}

			assert.Equal(t, tc.code, w.StatusCode)
			assert.Equal(t, tc.errorStr, string(resp))
		})
	}
}

func compareJSON(expected []byte, response []byte) error {
	if bytes.Equal(bytes.TrimSpace(response), expected) {
		return nil
	}

	return errors.Errorf("expected response `%s`, found `%s`", string(expected), string(response))
}

func createLedger(t *testing.T) *wavelet.Ledger {
	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ledger := wavelet.NewLedger(store.NewInmem(), skademlia.NewClient(":0", keys))
	return ledger
}

type testErrResponse struct {
	StatusText string `json:"status"`          // user-level status message
	ErrorText  string `json:"error,omitempty"` // application-level error message, for debugging
}

func (t testErrResponse) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	return json.Marshal(t)
}

func serve(router *fasthttprouter.Router, req *http.Request) (*http.Response, error) {
	server := &fasthttp.Server{
		Handler: router.Handler,
	}

	requestString, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		return nil, err
	}

	rw := &readWriter{}
	rw.r.WriteString(string(requestString))

	ch := make(chan error)
	go func() {
		ch <- server.ServeConn(rw)
	}()

	select {
	case err := <-ch:
		if err != nil {
			return nil, err
		}
	case <-time.After(10 * time.Second):
		return nil, errors.New("timeout")
	}

	return http.ReadResponse(bufio.NewReader(&rw.w), req)
}

type readWriter struct {
	net.Conn
	r bytes.Buffer
	w bytes.Buffer
}

func (rw *readWriter) Close() error {
	return nil
}

func (rw *readWriter) Read(b []byte) (int, error) {
	return rw.r.Read(b)
}

func (rw *readWriter) Write(b []byte) (int, error) {
	return rw.w.Write(b)
}

func (rw *readWriter) RemoteAddr() net.Addr {
	return &net.TCPAddr{
		IP: net.IPv4zero,
	}
}

func (rw *readWriter) LocalAddr() net.Addr {
	return &net.TCPAddr{
		IP: net.IPv4zero,
	}
}

func (rw *readWriter) SetReadDeadline(t time.Time) error {
	return nil
}

func (rw *readWriter) SetWriteDeadline(t time.Time) error {
	return nil
}

func newTransaction(keys *skademlia.Keypair, tag sys.Tag, nonce uint64, block uint64, payload []byte) wavelet.Transaction {
	var nonceBuf [8]byte
	binary.BigEndian.PutUint64(nonceBuf[:], nonce)

	var blockBuf [8]byte
	binary.BigEndian.PutUint64(blockBuf[:], block)

	signature := edwards25519.Sign(
		keys.PrivateKey(),
		append(nonceBuf[:], append(blockBuf[:], append([]byte{byte(tag)}, payload...)...)...),
	)

	return wavelet.NewSignedTransaction(
		keys.PublicKey(), nonce, block,
		tag, payload, signature,
	)
}
