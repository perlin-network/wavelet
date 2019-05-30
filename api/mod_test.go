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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/buaazp/fasthttprouter"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strconv"
	"testing"
	"testing/quick"
	"time"
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
	_ = wavelet.NewTransaction(keys, sys.TagTransfer, buf[:])
	assert.NoError(t, err)

	// Build an expected response
	var expectedResponse transactionList
	for _, tx := range gateway.ledger.Graph().ListTransactions(0, 0, wavelet.AccountID{}, wavelet.AccountID{}) {
		txRes := &transaction{tx: tx}
		txRes.status = "applied"

		//_, err := txRes.marshal()
		//assert.NoError(t, err)
		expectedResponse = append(expectedResponse, txRes)
	}

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
				StatusText: "Bad request.",
				ErrorText:  "sender ID must be presented as valid hex: encoding/hex: odd length hex string",
			},
		},
		{
			name:     "sender invalid length",
			url:      "/tx?sender=746c703579786279793638626e726a77666574656c6d34386d6739306b7166306565",
			wantCode: http.StatusBadRequest,
			wantResponse: testErrResponse{
				StatusText: "Bad request.",
				ErrorText:  "sender ID must be 32 bytes long",
			},
		},
		{
			name:     "creator not hex",
			url:      "/tx?creator=1",
			wantCode: http.StatusBadRequest,
			wantResponse: testErrResponse{
				StatusText: "Bad request.",
				ErrorText:  "creator ID must be presented as valid hex: encoding/hex: odd length hex string",
			},
		},
		{
			name:     "creator invalid length",
			url:      "/tx?creator=746c703579786279793638626e726a77666574656c6d34386d6739306b7166306565",
			wantCode: http.StatusBadRequest,
			wantResponse: testErrResponse{
				StatusText: "Bad request.",
				ErrorText:  "creator ID must be 32 bytes long",
			},
		},
		{
			name:     "creator not hex",
			url:      "/tx?creator=1",
			wantCode: http.StatusBadRequest,
			wantResponse: testErrResponse{
				StatusText: "Bad request.",
				ErrorText:  "creator ID must be presented as valid hex: encoding/hex: odd length hex string",
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
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest("GET", "http://localhost"+tc.url, nil)
			assert.NoError(t, err)

			w, err := serve(gateway.router, request)
			if err != nil {
				t.Fatal(t)
			}
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
	_ = wavelet.NewTransaction(keys, sys.TagTransfer, buf[:])
	assert.NoError(t, err)

	var txId wavelet.TransactionID
	for _, tx := range gateway.ledger.Graph().ListTransactions(0, 0, wavelet.AccountID{}, wavelet.AccountID{}) {
		txId = tx.ID
		break
	}

	tx := gateway.ledger.Graph().FindTransaction(txId)
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
				StatusText: "Bad request.",
				ErrorText:  fmt.Sprintf("transaction ID must be %d bytes long", wavelet.SizeTransactionID),
			},
		},
		{
			name:         "success",
			id:           hex.EncodeToString(txId[:]),
			wantCode:     http.StatusOK,
			wantResponse: txRes,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest("GET", "http://localhost/tx/"+tc.id, nil)
			assert.NoError(t, err)

			w, err := serve(gateway.router, request)
			assert.NoError(t, err)
			assert.NotNil(t, w)

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
		t.Run(tc.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tc.req)
			assert.NoError(t, err)

			request := httptest.NewRequest("POST", "http://localhost/tx/send", bytes.NewReader(reqBody))

			w, err := serve(gateway.router, request)
			assert.NoError(t, err)
			assert.NotNil(t, w)

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
		assert.NoError(t, err)

		assert.NotNil(t, res)
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

				assert.NoError(t, err)

				assert.NotNil(t, res)
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://localhost"+tc.url, nil)

			w, err := serve(gateway.router, request)
			assert.NoError(t, err)
			assert.NotNil(t, w)

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
			wantCode: http.StatusBadRequest,
			wantError: testErrResponse{
				StatusText: "Bad request.",
				ErrorText:  fmt.Sprintf("could not find contract with ID %s", "3132333435363738393031323334353637383930313233343536373839303132"),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://localhost"+tc.url, nil)

			w, err := serve(gateway.router, request)
			assert.NoError(t, err)
			assert.NotNil(t, w)

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
				StatusText: "Bad request.",
				ErrorText:  "could not parse page index",
			},
		},
		{
			name:     "id not exist",
			url:      "/contract/3132333435363738393031323334353637383930313233343536373839303132/page/1",
			wantCode: http.StatusBadRequest,
			wantError: testErrResponse{
				StatusText: "Bad request.",
				ErrorText:  fmt.Sprintf("could not find any pages for contract with ID %s", "3132333435363738393031323334353637383930313233343536373839303132"),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://localhost"+tc.url, nil)

			w, err := serve(gateway.router, request)
			assert.NoError(t, err)
			assert.NotNil(t, w)

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

	listener, err := net.Listen("tcp", ":0")
	assert.NoError(t, err)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))

	gateway.client = skademlia.NewClient(addr, keys,
		skademlia.WithC1(sys.SKademliaC1),
		skademlia.WithC2(sys.SKademliaC2),
	)

	//n, err := xnoise.ListenTCP(0)
	//assert.NoError(t, err)
	//gateway.node = n

	//gateway.network = skademlia.New(net.JoinHostPort("127.0.0.1", strconv.Itoa(n.Addr().(*net.TCPAddr).Port)), keys, xnoise.DialTCP)

	request := httptest.NewRequest("GET", "http://localhost/ledger", nil)

	w, err := serve(gateway.router, request)
	assert.NoError(t, err)
	assert.NotNil(t, w)

	response, err := ioutil.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, w.StatusCode)

	publicKey := keys.PublicKey()

	ledgerStatusResponse := struct {
		PublicKey     string   `json:"public_key"`
		HostAddress   string   `json:"address"`
		RootID        string   `json:"root_id"`
		RoundID       uint64   `json:"round_id"`
		Difficulty    uint64   `json:"difficulty"`
		PeerAddresses []string `json:"peers"`
	}{
		PublicKey:     hex.EncodeToString(publicKey[:]),
		HostAddress:   "127.0.0.1:" + strconv.Itoa(listener.Addr().(*net.TCPAddr).Port),
		PeerAddresses: nil,
		RootID:        "403517ca121f7638349cc92d654d20ac0f63d1958c897bc0cbcc2cdfe8bc74cc",
		RoundID:       0,
		Difficulty:    uint64(sys.MinDifficulty),
	}

	assert.NoError(t, compareJson(ledgerStatusResponse, response))
}

// Test the rate limit on all endpoints
func TestEndpointsRateLimit(t *testing.T) {
	gateway := New()
	gateway.rateLimiter = newRatelimiter(10, 10*time.Minute)
	gateway.setup()

	gateway.ledger = createLedger(t)

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)
	gateway.keys = keys

	listener, err := net.Listen("tcp", ":0")
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
			url:           "/poll/broadcaster",
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
		t.Run(tc.url, func(t *testing.T) {
			// We assume the loop will complete in less than 1 second.
			for i := 0; i < maxPerSecond*2; i++ {
				request := httptest.NewRequest(tc.method, "http://localhost"+tc.url, nil)
				w, err := serve(gateway.router, request)
				assert.NoError(t, err)
				assert.NotNil(t, w)

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

func compareJson(expected interface{}, response []byte) error {
	b, err := json.Marshal(expected)
	if err != nil {
		return err
	}

	if bytes.Equal(bytes.TrimSpace(response), b) {
		return nil
	}

	return errors.Errorf("expected response `%s`, found `%s`", string(b), string(response))
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
