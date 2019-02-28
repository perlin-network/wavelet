package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/perlin-network/noise/identity/ed25519"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/wavelet"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInitSession(t *testing.T) {
	randomKeyPair := ed25519.RandomKeys()

	hub := &hub{registry: newSessionRegistry()}
	hub.setRoutes()

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
			if err != nil {
				t.Fatal(err)
			}

			request := httptest.NewRequest("POST", "/session/init", bytes.NewReader(body))
			request.Header.Add("Content-Type", "application/json")

			w := httptest.NewRecorder()

			hub.router.ServeHTTP(w, request)

			raw, err := ioutil.ReadAll(w.Body)
			if err != nil {
				t.Fatal(err)
			}

			if w.Code != tc.wantCode {
				t.Fatalf("expected status code %d, found %d: %s", tc.wantCode, w.Code, string(raw))
			}
		})
	}
}

func TestListTransaction(t *testing.T) {
	hub := &hub{registry: newSessionRegistry()}
	hub.setRoutes()

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

			raw, err := ioutil.ReadAll(w.Body)
			if err != nil {
				t.Fatal(err)
			}

			if w.Code != tc.wantCode {
				t.Fatalf("expected status code %d, found %d: %s", tc.wantCode, w.Code, string(raw))
			}
		})
	}
}

func TestGetTransaction(t *testing.T) {
	hub := &hub{registry: newSessionRegistry()}
	hub.setRoutes()

	sess, err := hub.registry.newSession()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		sessionToken string
		id           string
		wantCode     int
		wantError    string
	}{
		{
			name:         "invalid id length",
			sessionToken: sess.id,
			id:           "1c331c1d",
			wantCode:     http.StatusBadRequest,
			wantError:    fmt.Sprintf("transaction ID must be %d bytes long", wavelet.TransactionIDSize),
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
			if err != nil {
				t.Fatal(err)
			}

			var response ErrResponse
			err = json.Unmarshal(raw, &response)
			if err != nil {
				t.Fatal(err)
			}

			if w.Code != tc.wantCode {
				t.Fatalf("expected status code %d, found %d: %s", tc.wantCode, w.Code, string(raw))
			}

			if response.ErrorText != tc.wantError {
				t.Fatalf("expected error %s, found %s: %s", tc.wantError, response.ErrorText, string(raw))

			}
		})
	}
}

func TestSendTransaction(t *testing.T) {
	hub := &hub{registry: newSessionRegistry()}
	hub.setRoutes()

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
			if err != nil {
				t.Fatal(err)
			}

			request := httptest.NewRequest("POST", "/tx/send", bytes.NewReader(reqBody))

			if tc.sessionToken != "" {
				request.Header.Add(HeaderSessionToken, tc.sessionToken)
			}

			w := httptest.NewRecorder()

			hub.router.ServeHTTP(w, request)

			raw, err := ioutil.ReadAll(w.Body)
			if err != nil {
				t.Fatal(err)
			}

			if w.Code != tc.wantCode {
				t.Fatalf("expected status code %d, found %d: %s", tc.wantCode, w.Code, string(raw))
			}
		})
	}
}

func getGoodCredentialRequest(t *testing.T, keypair *ed25519.Keypair) SessionInitRequest {
	millis := time.Now().Unix() * 1000
	authStr := fmt.Sprintf("%s%d", SessionInitMessage, millis)

	sig, err := eddsa.Sign(keypair.PrivateKey(), []byte(authStr))
	if err != nil {
		t.Fatal(err)
	}

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
