package api

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/buaazp/fasthttprouter"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher/aead"
	"github.com/perlin-network/noise/handshake/ecdh"
	"github.com/perlin-network/noise/identity/ed25519"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/noise/transport"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
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
	"testing"
	"testing/quick"
	"time"
)

func TestInitSession(t *testing.T) {
	randomKeyPair := ed25519.RandomKeys()

	gateway := New()
	gateway.setup(false)

	tests := []struct {
		name     string
		url      string
		req      SessionInitRequest
		wantCode int
	}{
		{
			url:      "/session/init",
			name:     "bad request",
			req:      getBadCredentialRequest(),
			wantCode: http.StatusBadRequest,
		},
		{
			url:      "/session/init",
			name:     "good request",
			req:      getGoodCredentialRequest(t, randomKeyPair),
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.req)
			assert.NoError(t, err)

			request, err := http.NewRequest("POST", "http://localhost"+tc.url, bytes.NewReader(body))
			assert.Nil(t, err)

			w, err := serve(gateway.router, request)
			assert.NoError(t, err)
			assert.NotNil(t, w)

			assert.NotNil(t, w.Body)

			_, err = ioutil.ReadAll(w.Body)
			assert.Nil(t, err)

			assert.Equal(t, tc.wantCode, w.StatusCode, "status code")
		})
	}
}

func TestListTransaction(t *testing.T) {
	gateway := New()
	gateway.setup(false)

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

	gateway.ledger = createLedger()

	// Create a transaction
	keys := skademlia.RandomKeys()
	var buf [200]byte
	_, err = rand.Read(buf[:])
	assert.NoError(t, err)
	_, err = wavelet.NewTransaction(keys, sys.TagTransfer, buf[:])
	assert.NoError(t, err)

	// Build an expected response
	var expectedResponse TransactionList
	for _, tx := range gateway.ledger.ListTransactions(0, 0, common.AccountID{}, common.AccountID{}) {
		txRes := &Transaction{tx: tx}

		//_, err := txRes.marshal()
		//assert.NoError(t, err)
		expectedResponse = append(expectedResponse, txRes)
	}

	tests := []struct {
		name         string
		url          string
		wantCode     int
		wantResponse fastjsonMarshal
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
			request.Header.Add(HeaderSessionToken, sess.id)

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
				r, err := tc.wantResponse.marshal(new(fastjson.ArenaPool).Get())
				assert.NoError(t, err)
				assert.Equal(t, string(r), string(bytes.TrimSpace(response)))
			}
		})
	}
}

func TestGetTransaction(t *testing.T) {
	gateway := New()
	gateway.setup(false)

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

	gateway.ledger = createLedger()

	// Create a transaction
	keys := skademlia.RandomKeys()
	var buf [200]byte
	_, err = rand.Read(buf[:])
	assert.NoError(t, err)
	_, err = wavelet.NewTransaction(keys, sys.TagTransfer, buf[:])
	assert.NoError(t, err)

	var txId common.TransactionID
	for _, tx := range gateway.ledger.ListTransactions(0, 0, common.AccountID{}, common.AccountID{}) {
		txId = tx.ID
		break
	}

	tx, found := gateway.ledger.FindTransaction(txId)
	if !found {
		t.Fatal("not found")
	}

	tests := []struct {
		name         string
		sessionToken string
		id           string
		wantCode     int
		wantResponse fastjsonMarshal
	}{
		{
			name:         "invalid id length",
			sessionToken: sess.id,
			id:           "1c331c1d",
			wantCode:     http.StatusBadRequest,
			wantResponse: &testErrResponse{
				StatusText: "Bad request.",
				ErrorText:  fmt.Sprintf("transaction ID must be %d bytes long", common.SizeTransactionID),
			},
		},
		{
			name:         "success",
			sessionToken: sess.id,
			id:           hex.EncodeToString(txId[:]),
			wantCode:     http.StatusOK,
			wantResponse: &Transaction{tx: tx},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest("GET", "http://localhost/tx/"+tc.id, nil)
			assert.NoError(t, err)

			if tc.sessionToken != "" {
				request.Header.Add(HeaderSessionToken, tc.sessionToken)
			}

			w, err := serve(gateway.router, request)
			assert.NoError(t, err)
			assert.NotNil(t, w)

			response, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.StatusCode, "status code")

			if tc.wantResponse != nil {
				r, err := tc.wantResponse.marshal(new(fastjson.ArenaPool).Get())
				assert.Nil(t, err)
				assert.Equal(t, string(r), string(bytes.TrimSpace(response)))
			}
		})
	}
}

func TestSendTransaction(t *testing.T) {
	gateway := New()
	gateway.setup(false)

	tests := []struct {
		name         string
		wantCode     int
		sessionToken string
		req          SendTransactionRequest
	}{
		{
			name:     "missing token",
			wantCode: http.StatusBadRequest,
		},
		{
			name:         "token not exist",
			wantCode:     http.StatusBadRequest,
			sessionToken: "invalid token",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tc.req)
			assert.NoError(t, err)

			request := httptest.NewRequest("POST", "http://localhost/tx/send", bytes.NewReader(reqBody))

			if tc.sessionToken != "" {
				request.Header.Add(HeaderSessionToken, tc.sessionToken)
			}

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
	gateway.setup(false)

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

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
		request.Header.Add(HeaderSessionToken, sess.id)

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

func TestInitSessionRandom(t *testing.T) {
	gateway := New()
	gateway.setup(false)

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

	type request struct {
		PublicKey  string `json:"public_key"`
		Signature  string `json:"signature"`
		TimeMillis uint64 `json:"time_millis"`
	}

	f := func(req request) bool {
		reqBody, err := json.Marshal(req)
		assert.NoError(t, err)

		request := httptest.NewRequest("POST", "http://localhost/session/init", bytes.NewReader(reqBody))
		request.Header.Add(HeaderSessionToken, sess.id)

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
	gateway.setup(false)

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

	tests := []struct {
		url          string
		sessionToken string
	}{
		{
			url: "/session/init",
		},
		{
			url:          "/tx/send",
			sessionToken: sess.id,
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
				if tc.sessionToken != "" {
					request.Header.Add(HeaderSessionToken, tc.sessionToken)
				}

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
	gateway.setup(false)

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

	gateway.ledger = createLedger()

	idHex := "1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d"
	idBytes, err := hex.DecodeString(idHex)
	assert.NoError(t, err)

	var id32 common.AccountID
	copy(id32[:], idBytes)

	wavelet.WriteAccountBalance(gateway.ledger.Snapshot(), id32, 10)
	wavelet.WriteAccountStake(gateway.ledger.Snapshot(), id32, 11)
	wavelet.WriteAccountContractNumPages(gateway.ledger.Snapshot(), id32, 12)

	var id common.AccountID
	copy(id[:], idBytes)

	tests := []struct {
		name         string
		url          string
		wantCode     int
		wantResponse fastjsonMarshal
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
			wantResponse: &Account{ledger: gateway.ledger, id: id},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://localhost"+tc.url, nil)
			request.Header.Add(HeaderSessionToken, sess.id)

			w, err := serve(gateway.router, request)
			assert.NoError(t, err)
			assert.NotNil(t, w)

			response, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.StatusCode, "status code")

			if tc.wantResponse != nil {
				r, err := tc.wantResponse.marshal(new(fastjson.ArenaPool).Get())
				assert.Nil(t, err)
				assert.Equal(t, string(r), string(bytes.TrimSpace(response)))
			}
		})
	}
}

func TestGetContractCode(t *testing.T) {
	gateway := New()
	gateway.setup(false)

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

	gateway.ledger = createLedger()

	idHex := "1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d"
	idBytes, err := hex.DecodeString(idHex)
	assert.NoError(t, err)

	var id32 common.AccountID
	copy(id32[:], idBytes)

	s := gateway.ledger.Snapshot()
	wavelet.WriteAccountContractCode(s, id32, []byte("contract code"))

	tests := []struct {
		name      string
		url       string
		wantCode  int
		wantError fastjsonMarshal
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
			request.Header.Add(HeaderSessionToken, sess.id)

			w, err := serve(gateway.router, request)
			assert.NoError(t, err)
			assert.NotNil(t, w)

			response, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.StatusCode, "status code")

			if tc.wantError != nil {
				r, err := tc.wantError.marshal(new(fastjson.ArenaPool).Get())
				assert.Nil(t, err)
				assert.Equal(t, string(r), string(bytes.TrimSpace(response)))
			}
		})
	}
}

func TestGetContractPages(t *testing.T) {
	gateway := New()
	gateway.setup(false)

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

	gateway.ledger = createLedger()

	// string: u1mf2g3b2477y5btco22txqxuc41cav6
	var id = "75316d66326733623234373779356274636f3232747871787563343163617636"

	tests := []struct {
		name      string
		url       string
		wantCode  int
		wantError fastjsonMarshal
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
			request.Header.Add(HeaderSessionToken, sess.id)

			w, err := serve(gateway.router, request)
			assert.NoError(t, err)
			assert.NotNil(t, w)

			response, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.StatusCode, "status code")

			if tc.wantError != nil {
				r, err := tc.wantError.marshal(new(fastjson.ArenaPool).Get())
				assert.Nil(t, err)
				assert.Equal(t, string(r), string(bytes.TrimSpace(response)))
			}
		})
	}
}

func TestGetLedger(t *testing.T) {
	gateway := New()
	gateway.setup(false)

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

	gateway.ledger = createLedger()

	keys := skademlia.RandomKeys()

	// create a node
	params := noise.DefaultParams()
	params.Keys = keys
	params.Port = 9000
	params.Transport = transport.NewBuffered()
	node, err := noise.NewNode(params)
	assert.Nil(t, err)
	p := protocol.New()
	p.Register(ecdh.New())
	p.Register(aead.New())
	p.Register(skademlia.New())
	p.Enforce(node)

	gateway.node = node

	request := httptest.NewRequest("GET", "http://localhost/ledger", nil)
	request.Header.Add(HeaderSessionToken, sess.id)

	w, err := serve(gateway.router, request)
	assert.NoError(t, err)
	assert.NotNil(t, w)

	response, err := ioutil.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, w.StatusCode)

	ledgerStatusResponse := struct {
		PublicKey     string   `json:"public_key"`
		HostAddress   string   `json:"address"`
		RootID        string   `json:"root_id"`
		ViewID        uint64   `json:"view_id"`
		Difficulty    uint64   `json:"difficulty"`
		PeerAddresses []string `json:"peers"`
	}{
		PublicKey:     hex.EncodeToString(keys.PublicKey()),
		HostAddress:   "127.0.0.1:9000",
		PeerAddresses: nil,
		RootID:        "fe8b107e4d2972677c4012d7e11ce7f8722c9dbba30b8c4a73bf56ac20e6a789",
		ViewID:        1,
		Difficulty:    uint64(sys.MinDifficulty),
	}

	assert.NoError(t, compareJson(ledgerStatusResponse, response))
}

// Test the authenticate checking of all the APIs that require authentication
func TestAuthenticatedAPI(t *testing.T) {
	gateway := New()
	gateway.setup(false)

	contractID := "1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d"

	tests := []struct {
		url    string
		method string
	}{
		{
			url:    "/ledger",
			method: "GET",
		},
		{
			url:    "/accounts/1",
			method: "GET",
		},
		{
			url:    "/contract/" + contractID,
			method: "GET",
		},
		{
			url:    "/contract/" + contractID + "/page",
			method: "GET",
		},
		{
			url:    "/contract/" + contractID + "/page/1",
			method: "GET",
		},
		{
			url:    "/tx",
			method: "GET",
		},
		{
			url:    "/tx/1",
			method: "GET",
		},
		{
			url:    "/tx/send",
			method: "POST",
		},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			// Without session header
			{
				request := httptest.NewRequest(tc.method, "http://localhost"+tc.url, nil)

				testAuthenticatedAPI(t, gateway, request, testErrResponse{
					StatusText: "Bad request.",
					ErrorText:  "session token not specified via HTTP header \"X-Session-Token\"",
				})
			}

			// With invalid session
			{
				request := httptest.NewRequest(tc.method, "http://localhost"+tc.url, nil)
				request.Header.Add(HeaderSessionToken, "invalid token")

				testAuthenticatedAPI(t, gateway, request, testErrResponse{
					StatusText: "Bad request.",
					ErrorText:  "could not find session invalid token",
				})
			}
		})
	}
}

func testAuthenticatedAPI(t *testing.T, gateway *Gateway, request *http.Request, res testErrResponse) {
	w, err := serve(gateway.router, request)
	assert.NoError(t, err)
	assert.NotNil(t, w)

	response, err := ioutil.ReadAll(w.Body)
	assert.Nil(t, err)

	assert.Equal(t, http.StatusBadRequest, w.StatusCode, "status code")

	assert.NoError(t, compareJson(res, response))
}

func getGoodCredentialRequest(t *testing.T, keypair *ed25519.Keypair) SessionInitRequest {
	millis := time.Now().Unix() * 1000
	authStr := fmt.Sprintf("%s%d", SessionInitMessage, millis)

	sig, err := eddsa.Sign(keypair.PrivateKey(), []byte(authStr))
	assert.Nil(t, err)

	return SessionInitRequest{
		PublicKey:  hex.EncodeToString(keypair.PublicKey()),
		TimeMillis: uint64(millis),
		Signature:  hex.EncodeToString(sig),
	}
}

func getBadCredentialRequest() SessionInitRequest {
	return SessionInitRequest{
		PublicKey:  "bad key",
		TimeMillis: uint64(time.Now().Unix() * 1000),
		Signature:  hex.EncodeToString([]byte("bad sig")),
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

func createLedger() *wavelet.Ledger {
	ledger := wavelet.NewLedger(ed25519.RandomKeys(), store.NewInmem())
	return ledger
}

type testErrResponse struct {
	StatusText string `json:"status"`          // user-level status message
	ErrorText  string `json:"error,omitempty"` // application-level error message, for debugging
}

func (t testErrResponse) marshal(arena *fastjson.Arena) ([]byte, error) {
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
