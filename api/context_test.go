package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func Test_requestContext_loadSession(t *testing.T) {
	t.Parallel()

	// create a registry with an expired session
	regOutOfDateEntry := newSessionRegistry()

	sess, _ := regOutOfDateEntry.newSession(ClientPermissions{})

	olderTime := sess.renewTime.Add(-(MaxSessionTimeoutMinutes + 1) * time.Minute)

	sess.renewTime = &olderTime

	type fields struct {
		service  *service
		response http.ResponseWriter
		request  *http.Request
		session  *session
	}

	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "missing session",
			fields: fields{
				request: httptest.NewRequest("POST", RouteTransactionList, strings.NewReader(``)),
			},
			wantErr: true,
		},
		{
			name: "bad session format",
			fields: fields{
				request: &http.Request{
					Method: "POST",
					Header: map[string][]string{
						HeaderSessionToken: []string{"bad"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "session not in registry",
			fields: fields{
				request: &http.Request{
					Method: "POST",
					Header: map[string][]string{
						HeaderSessionToken: []string{"0511959c-4715-43ab-baf1-d34f436bb9c7"},
					},
				},
				service: &service{
					registry: newSessionRegistry(),
				},
			},
			wantErr: true,
		},
		{
			name: "expired session in registry",
			fields: fields{
				request: &http.Request{
					Method: "POST",
					Header: map[string][]string{
						HeaderSessionToken: []string{sess.ID},
					},
				},
				service: &service{
					registry: regOutOfDateEntry,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &requestContext{
				service:  tt.fields.service,
				response: tt.fields.response,
				request:  tt.fields.request,
				session:  tt.fields.session,
			}

			if err := c.loadSession(); (err != nil) != tt.wantErr {
				t.Errorf("requestContext.loadSession() name = %s, error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func Test_requestContext_readJSON(t *testing.T) {
	t.Parallel()

	var bigBytes bytes.Buffer

	bigBytes.Grow(MaxRequestBodySize + 1)

	for i := 0; i < MaxRequestBodySize+1; i++ {
		bigBytes.WriteByte((byte)(i % 10))
	}

	var testString string

	testStruct := SendTransactionRequest{
		Tag: "test tag",
	}

	jsonStruct, _ := json.Marshal(testStruct)

	type fields struct {
		service  *service
		response http.ResponseWriter
		request  *http.Request
		session  *session
	}

	type args struct {
		out interface{}
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "a very big payload",
			fields: fields{
				request: httptest.NewRequest("POST", RouteTransactionList, bytes.NewReader(bigBytes.Bytes())),
			},
			args:    args{out: testString},
			wantErr: true,
		},
		{
			name: "bad json",
			fields: fields{
				request: httptest.NewRequest("POST", RouteTransactionList, strings.NewReader(`sdf}`)),
			},
			args:    args{out: testString},
			wantErr: true,
		},
		{
			name: "good string",
			fields: fields{
				request: httptest.NewRequest("POST", RouteTransactionList, strings.NewReader(`"a valid string"`)),
			},
			args:    args{out: &testString},
			wantErr: false,
		},
		{
			name: "good json",
			fields: fields{
				request: httptest.NewRequest("POST", RouteTransactionList, strings.NewReader(string(jsonStruct))),
			},
			args:    args{out: &testStruct},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &requestContext{
				service:  tt.fields.service,
				response: tt.fields.response,
				request:  tt.fields.request,
				session:  tt.fields.session,
			}
			if err := c.readJSON(tt.args.out); (err != nil) != tt.wantErr {
				t.Errorf("requestContext.readJSON() name = %s, error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}
