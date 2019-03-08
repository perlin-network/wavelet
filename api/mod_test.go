package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var genesisPath = "../cmd/wavelet/config/genesis.json"

func TestInitSession(t *testing.T) {
	randomKeyPair := ed25519.RandomKeys()

	gateway := New()
	gateway.setup()

	tests := []struct {
		name     string
		req      SessionInitRequest
		wantCode int
	}{

		{
			name:     "bad request",
			req:      getBadCredentialRequest(),
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "good request",
			req:      getGoodCredentialRequest(t, randomKeyPair),
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.req)
			assert.NoError(t, err)

			request := httptest.NewRequest("POST", "/session/init", bytes.NewReader(body))
			request.Header.Add("Content-Type", "application/json")

			w := httptest.NewRecorder()

			gateway.router.ServeHTTP(w, request)

			_, err = ioutil.ReadAll(w.Body)
			assert.Nil(t, err)

			assert.Equal(t, tc.wantCode, w.Code, "status code")
		})
	}
}

func TestListTransaction(t *testing.T) {
	gateway := New()
	gateway.setup()

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

	tests := []struct {
		name         string
		url          string
		wantCode     int
		wantError    string
		wantResponse interface{}
	}{
		{
			name:     "sender not hex",
			url:      "/tx/?sender=1",
			wantCode: http.StatusBadRequest,
			wantResponse: ErrResponse{
				StatusText: "Bad request.",
				ErrorText:  "sender ID must be presented as valid hex: encoding/hex: odd length hex string",
			},
		},
		{
			name:     "sender invalid length",
			url:      "/tx/?sender=746c703579786279793638626e726a77666574656c6d34386d6739306b7166306565",
			wantCode: http.StatusBadRequest,
			wantResponse: ErrResponse{
				StatusText: "Bad request.",
				ErrorText:  "sender ID must be 32 bytes long",
			},
		},
		{
			name:     "creator not hex",
			url:      "/tx/?creator=1",
			wantCode: http.StatusBadRequest,
			wantResponse: ErrResponse{
				StatusText: "Bad request.",
				ErrorText:  "creator ID must be presented as valid hex: encoding/hex: odd length hex string",
			},
		},
		{
			name:     "creator invalid length",
			url:      "/tx/?creator=746c703579786279793638626e726a77666574656c6d34386d6739306b7166306565",
			wantCode: http.StatusBadRequest,
			wantResponse: ErrResponse{
				StatusText: "Bad request.",
				ErrorText:  "creator ID must be 32 bytes long",
			},
		},
		{
			name:     "creator not hex",
			url:      "/tx/?creator=1",
			wantCode: http.StatusBadRequest,
			wantResponse: ErrResponse{
				StatusText: "Bad request.",
				ErrorText:  "creator ID must be presented as valid hex: encoding/hex: odd length hex string",
			},
		},
		{
			name:     "offset negative invalid",
			url:      "/tx/?offset=-1",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "limit negative invalid",
			url:      "/tx/?limit=-1",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", tc.url, nil)
			request.Header.Add(HeaderSessionToken, sess.id)

			w := httptest.NewRecorder()

			gateway.router.ServeHTTP(w, request)

			response, err := ioutil.ReadAll(w.Body)
			assert.Nil(t, err)

			t.Log(string(response))

			assert.Equal(t, tc.wantCode, w.Code, "status code")

			if tc.wantResponse != nil {
				assert.NoError(t, compareJson(tc.wantResponse, response))
			}
		})
	}
}

func TestGetTransaction(t *testing.T) {
	gateway := New()
	gateway.setup()

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

	tests := []struct {
		name         string
		sessionToken string
		id           string
		wantCode     int
		wantResponse interface{}
	}{
		{
			name:         "invalid id length",
			sessionToken: sess.id,
			id:           "1c331c1d",
			wantCode:     http.StatusBadRequest,
			wantResponse: &ErrResponse{
				StatusText: "Bad request.",
				ErrorText:  fmt.Sprintf("transaction ID must be %d bytes long", common.SizeTransactionID),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/tx/"+tc.id, nil)

			if tc.sessionToken != "" {
				request.Header.Add(HeaderSessionToken, tc.sessionToken)
			}

			w := httptest.NewRecorder()

			gateway.router.ServeHTTP(w, request)

			response, err := ioutil.ReadAll(w.Body)
			assert.Nil(t, err)

			assert.Equal(t, tc.wantCode, w.Code, "status code")

			if tc.wantResponse != nil {
				assert.NoError(t, compareJson(tc.wantResponse, response))
			}
		})
	}
}

func TestSendTransaction(t *testing.T) {
	gateway := New()
	gateway.setup()

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

			request := httptest.NewRequest("POST", "/tx/send", bytes.NewReader(reqBody))

			if tc.sessionToken != "" {
				request.Header.Add(HeaderSessionToken, tc.sessionToken)
			}

			w := httptest.NewRecorder()

			gateway.router.ServeHTTP(w, request)

			_, err = ioutil.ReadAll(w.Body)
			assert.Nil(t, err)

			assert.Equal(t, tc.wantCode, w.Code, "status code")
		})
	}
}

func TestGetAccount(t *testing.T) {
	gateway := New()
	gateway.setup()

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

	gateway.ledger = createLedger()

	idHex := "1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d"
	idBytes, err := hex.DecodeString(idHex)
	assert.NoError(t, err)

	var id32 common.AccountID
	copy(id32[:], idBytes)

	gateway.ledger.Accounts.WriteAccountBalance(id32, 10)
	gateway.ledger.Accounts.WriteAccountStake(id32, 11)
	gateway.ledger.Accounts.WriteAccountContractNumPages(id32, 12)

	tests := []struct {
		name         string
		url          string
		wantCode     int
		wantError    string
		wantResponse interface{}
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
			name:     "valid id",
			url:      "/accounts/" + idHex,
			wantCode: http.StatusOK,
			wantResponse: &Account{
				PublicKey:  idHex,
				Balance:    10,
				Stake:      11,
				IsContract: true,
				NumPages:   12,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", tc.url, nil)
			request.Header.Add(HeaderSessionToken, sess.id)

			w := httptest.NewRecorder()

			gateway.router.ServeHTTP(w, request)

			response, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.Code, "status code")

			if tc.wantResponse != nil {
				assert.NoError(t, compareJson(tc.wantResponse, response))
			}
		})
	}
}

func TestGetContractCode(t *testing.T) {
	gateway := New()
	gateway.setup()

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

	gateway.ledger = createLedger()

	idHex := "1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d"
	idBytes, err := hex.DecodeString(idHex)
	assert.NoError(t, err)

	var id32 common.AccountID
	copy(id32[:], idBytes)

	gateway.ledger.Accounts.WriteAccountContractCode(id32, []byte("contract code"))

	tests := []struct {
		name         string
		url          string
		wantCode     int
		wantError    interface{}
		wantResponse string
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
			wantError: ErrResponse{
				StatusText: "Bad request.",
				ErrorText:  fmt.Sprintf("could not find contract with ID %s", "3132333435363738393031323334353637383930313233343536373839303132"),
			},
		},
		{
			name:         "id exist",
			url:          "/contract/" + idHex,
			wantCode:     http.StatusOK,
			wantResponse: "contract code",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", tc.url, nil)
			request.Header.Add(HeaderSessionToken, sess.id)

			w := httptest.NewRecorder()

			gateway.router.ServeHTTP(w, request)

			response, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.Code, "status code")

			if tc.wantResponse != "" {
				assert.Equal(t, tc.wantResponse, string(response))
			}

			if tc.wantError != nil {
				assert.NoError(t, compareJson(tc.wantError, response))
			}
		})
	}
}

func TestGetContractPages(t *testing.T) {
	gateway := New()
	gateway.setup()

	sess, err := gateway.registry.newSession()
	assert.NoError(t, err)

	gateway.ledger = createLedger()

	// string: u1mf2g3b2477y5btco22txqxuc41cav6
	var id = "75316d66326733623234373779356274636f3232747871787563343163617636"
	{
		var id32 common.AccountID
		copy(id32[:], []byte("u1mf2g3b2477y5btco22txqxuc41cav6"))

		gateway.ledger.Accounts.WriteAccountContractPage(id32, 1, []byte("page"))
		gateway.ledger.Accounts.WriteAccountContractNumPages(id32, 2)
	}

	// string: limo9msslzoeecf2tiy4qdsk42s3t2hd
	var idEmpty = "6c696d6f396d73736c7a6f6565636632746979347164736b3432733374326864"
	{
		var id32 common.AccountID
		copy(id32[:], []byte("limo9msslzoeecf2tiy4qdsk42s3t2hd"))
		gateway.ledger.Accounts.WriteAccountContractPage(id32, 1, nil)
		gateway.ledger.Accounts.WriteAccountContractNumPages(id32, 2)
	}

	tests := []struct {
		name         string
		url          string
		wantCode     int
		wantError    interface{}
		wantResponse string
	}{
		{
			name:     "id not uint",
			url:      "/contract/" + id + "/page/-1",
			wantCode: http.StatusBadRequest,
			wantError: ErrResponse{
				StatusText: "Bad request.",
				ErrorText:  "could not parse page index",
			},
		},
		{
			name:     "id not exist",
			url:      "/contract/3132333435363738393031323334353637383930313233343536373839303132/page/1",
			wantCode: http.StatusBadRequest,
			wantError: ErrResponse{
				StatusText: "Bad request.",
				ErrorText:  fmt.Sprintf("could not find any pages for contract with ID %s", "3132333435363738393031323334353637383930313233343536373839303132"),
			},
		},
		{
			name:     "index not exist",
			url:      "/contract/" + id + "/page/3",
			wantCode: http.StatusBadRequest,
			wantError: ErrResponse{
				StatusText: "Bad request.",
				ErrorText:  fmt.Sprintf("contract with ID %s only has %d pages, but you requested page %d", id, 2, 3),
			},
		},
		{
			name:     "empty page",
			url:      "/contract/" + idEmpty + "/page/1",
			wantCode: http.StatusBadRequest,
			wantError: ErrResponse{
				StatusText: "Bad request.",
				ErrorText:  fmt.Sprintf("page %d is either empty, or does not exist", 1),
			},
		},
		{
			name:         "index ok",
			url:          "/contract/" + id + "/page/1",
			wantCode:     http.StatusOK,
			wantResponse: "page",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", tc.url, nil)
			request.Header.Add(HeaderSessionToken, sess.id)

			w := httptest.NewRecorder()

			gateway.router.ServeHTTP(w, request)

			response, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.Code, "status code")

			if tc.wantResponse != "" {
				assert.Equal(t, tc.wantResponse, string(response))
			}

			if tc.wantError != nil {
				assert.NoError(t, compareJson(tc.wantError, response))
			}
		})
	}
}

func TestGetLedger(t *testing.T) {
	gateway := New()
	gateway.setup()

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

	request := httptest.NewRequest("GET", "/ledger", nil)
	request.Header.Add(HeaderSessionToken, sess.id)

	w := httptest.NewRecorder()

	gateway.router.ServeHTTP(w, request)

	response, err := ioutil.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, w.Code)

	expected := LedgerStatusResponse{
		PublicKey:     hex.EncodeToString(keys.PublicKey()),
		HostAddress:   "127.0.0.1:9000",
		PeerAddresses: nil,
		RootID:        "15981d398c1642c7601992e79dd6e4f208ec96f59548c82dccfc4c011b22d210",
		ViewID:        0,
		Difficulty:    5,
	}

	assert.NoError(t, compareJson(expected, response))
}

// Test the authenticate checking of all the APIs that require authentication
func TestAuthenticatedAPI(t *testing.T) {
	gateway := New()
	gateway.setup()

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
				request := httptest.NewRequest(tc.method, tc.url, nil)

				testAuthenticatedAPI(t, gateway, request, ErrResponse{
					StatusText: "Bad request.",
					ErrorText:  "session token not specified via HTTP header \"X-Session-Token\"",
				})
			}

			// With invalid session
			{
				request := httptest.NewRequest(tc.method, tc.url, nil)
				request.Header.Add(HeaderSessionToken, "invalid token")

				testAuthenticatedAPI(t, gateway, request, ErrResponse{
					StatusText: "Bad request.",
					ErrorText:  "could not find session invalid token",
				})
			}
		})
	}
}

func testAuthenticatedAPI(t *testing.T, gateway *Gateway, request *http.Request, res ErrResponse) {
	w := httptest.NewRecorder()

	gateway.router.ServeHTTP(w, request)

	response, err := ioutil.ReadAll(w.Body)
	assert.Nil(t, err)

	assert.Equal(t, http.StatusBadRequest, w.Code, "status code")

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
	kv := store.NewInmem()

	ledger := wavelet.NewLedger(kv, genesisPath)
	ledger.RegisterProcessor(sys.TagNop, new(wavelet.NopProcessor))
	ledger.RegisterProcessor(sys.TagTransfer, new(wavelet.TransferProcessor))
	ledger.RegisterProcessor(sys.TagContract, new(wavelet.ContractProcessor))
	ledger.RegisterProcessor(sys.TagStake, new(wavelet.StakeProcessor))

	return ledger
}
