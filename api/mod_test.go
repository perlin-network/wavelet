package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/perlin-network/noise/identity/ed25519"
	"github.com/perlin-network/noise/signature/eddsa"
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

	hub := &Gateway{registry: newSessionRegistry()}
	hub.setupRouter()

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

			hub.router.ServeHTTP(w, request)

			_, err = ioutil.ReadAll(w.Body)
			assert.Nil(t, err)

			assert.Equal(t, tc.wantCode, w.Code, "status code")

		})
	}
}

func TestListTransaction(t *testing.T) {
	hub := &Gateway{registry: newSessionRegistry()}
	hub.setupRouter()

	tests := []struct {
		name         string
		sessionToken string
		wantCode     int
	}{
		{
			name:     "missing token",
			wantCode: http.StatusBadRequest,
		},
		{
			name:         "token not exist",
			sessionToken: "invalid token",
			wantCode:     http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/tx/", nil)

			if tc.sessionToken != "" {
				request.Header.Add(HeaderSessionToken, tc.sessionToken)
			}

			w := httptest.NewRecorder()

			hub.router.ServeHTTP(w, request)

			_, err := ioutil.ReadAll(w.Body)
			assert.Nil(t, err)

			assert.Equal(t, tc.wantCode, w.Code, "status code")
		})
	}
}

func TestGetTransaction(t *testing.T) {
	hub := &Gateway{registry: newSessionRegistry()}
	hub.setupRouter()

	sess, err := hub.registry.newSession()
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

			hub.router.ServeHTTP(w, request)

			raw, err := ioutil.ReadAll(w.Body)
			assert.Nil(t, err)

			assert.Equal(t, tc.wantCode, w.Code, "status code")

			if tc.wantResponse != nil {
				assert.NoError(t, compareJson(tc.wantResponse, raw))
			}
		})
	}
}

func TestSendTransaction(t *testing.T) {
	hub := &Gateway{registry: newSessionRegistry()}
	hub.setupRouter()

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

			hub.router.ServeHTTP(w, request)

			_, err = ioutil.ReadAll(w.Body)
			assert.Nil(t, err)

			assert.Equal(t, tc.wantCode, w.Code, "status code")
		})
	}
}

func TestGetAccount(t *testing.T) {
	hub := &Gateway{registry: newSessionRegistry()}
	hub.setupRouter()

	sess, err := hub.registry.newSession()
	assert.NoError(t, err)

	hub.ledger = createLedger()

	idHex := "1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d"
	idBytes, err := hex.DecodeString(idHex)
	assert.NoError(t, err)

	var id32 common.AccountID
	copy(id32[:], idBytes)

	hub.ledger.WriteAccountBalance(id32, 10)
	hub.ledger.WriteAccountStake(id32, 11)
	hub.ledger.WriteAccountContractNumPages(id32, 12)

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

			hub.router.ServeHTTP(w, request)

			raw, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.Code, "status code")

			if tc.wantResponse != nil {
				assert.NoError(t, compareJson(tc.wantResponse, raw))
			}
		})
	}
}

func TestGetContractCode(t *testing.T) {
	hub := &Gateway{registry: newSessionRegistry()}
	hub.setupRouter()

	sess, err := hub.registry.newSession()
	assert.NoError(t, err)

	hub.ledger = createLedger()

	idHex := "1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d"
	idBytes, err := hex.DecodeString(idHex)
	assert.NoError(t, err)

	var id32 common.AccountID
	copy(id32[:], idBytes)

	hub.ledger.WriteAccountContractCode(id32, []byte("contract code"))

	tests := []struct {
		name         string
		url          string
		wantCode     int
		wantError    string
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
			name:         "valid id",
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

			hub.router.ServeHTTP(w, request)

			raw, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantCode, w.Code, "status code")

			if tc.wantResponse != "" {
				assert.Equal(t, tc.wantResponse, string(raw))
			}
		})
	}
}

// Test the authenticate checking of all the APIs that require authentication
func TestAuthenticatedAPI(t *testing.T) {
	hub := &Gateway{registry: newSessionRegistry()}
	hub.setupRouter()

	contractId := "1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d1c331c1d"

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
			url:    "/contract/" + contractId,
			method: "GET",
		},
		{
			url:    "/contract/" + contractId + "/page",
			method: "GET",
		},
		{
			url:    "/contract/" + contractId + "/page/1",
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

				testAuthenticatedAPI(t, hub, request, ErrResponse{
					StatusText: "Bad request.",
					ErrorText:  "session token not specified via HTTP header \"X-Session-Token\"",
				})
			}

			// With invalid session
			{
				request := httptest.NewRequest(tc.method, tc.url, nil)
				request.Header.Add(HeaderSessionToken, "invalid token")

				testAuthenticatedAPI(t, hub, request, ErrResponse{
					StatusText: "Bad request.",
					ErrorText:  "could not find session invalid token",
				})
			}
		})
	}
}

func testAuthenticatedAPI(t *testing.T, hub *Gateway, request *http.Request, res ErrResponse) {
	w := httptest.NewRecorder()

	hub.router.ServeHTTP(w, request)

	raw, err := ioutil.ReadAll(w.Body)
	assert.Nil(t, err)

	assert.Equal(t, http.StatusBadRequest, w.Code, "status code")

	assert.NoError(t, compareJson(res, raw))
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

func compareJson(expected interface{}, found []byte) error {
	b, err := json.Marshal(expected)
	if err != nil {
		return err
	}

	if bytes.Equal(bytes.TrimSpace(found), b) {
		return nil
	}

	return errors.Errorf("expected response `%s`, found `%s`", string(b), string(found))
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
