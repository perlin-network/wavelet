package api

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/wavelet/node"
)

func Test_service_resetStatsHandler(t *testing.T) {
	t.Parallel()
	type fields struct {
		clients  map[string]*ClientInfo
		registry *registry
		wavelet  node.NodeInterface
		network  *network.Network
		upgrader websocket.Upgrader
	}
	type args struct {
		ctx *requestContext
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int
		want1   interface{}
		wantErr bool
	}{
		{
			name: "has permission",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanControlStats: true,
						},
					},
				},
			},
			want:    http.StatusOK,
			want1:   "OK",
			wantErr: false,
		},
		{
			name: "does not have permission",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanControlStats: false,
						},
					},
				},
			},
			want:    http.StatusForbidden,
			want1:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &service{
				clients:  tt.fields.clients,
				registry: tt.fields.registry,
				wavelet:  tt.fields.wavelet,
				network:  tt.fields.network,
				upgrader: tt.fields.upgrader,
			}
			got, got1, err := s.resetStatsHandler(tt.args.ctx)
			assert.Equal(t, (err != nil), tt.wantErr, "service.resetStatsHandler() error = %v, wantErr %v", err, tt.wantErr)
			assert.Equal(t, got, tt.want, "service.resetStatsHandler() got = %v, want %v", got, tt.want)
			assert.True(t, reflect.DeepEqual(got1, tt.want1), "service.resetStatsHandler() got1 = %v, want %v", got1, tt.want1)
		})
	}
}

func Test_service_listTransactionHandler(t *testing.T) {
	t.Parallel()
	type fields struct {
		clients  map[string]*ClientInfo
		registry *registry
		wavelet  node.NodeInterface
		network  *network.Network
		upgrader websocket.Upgrader
	}
	type args struct {
		ctx *requestContext
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int
		want1   interface{}
		wantErr bool
	}{
		{
			name: "does not have permission",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanPollTransaction: false,
						},
					},
				},
			},
			want:    http.StatusForbidden,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "blank input",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanPollTransaction: true,
						},
					},
					request: httptest.NewRequest("POST", RouteTransactionList, strings.NewReader(``)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "offset with string",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanPollTransaction: true,
						},
					},
					request: httptest.NewRequest("POST", RouteTransactionList, strings.NewReader(`{"offset":"123"}`)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "limit with string",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanPollTransaction: true,
						},
					},
					request: httptest.NewRequest("POST", RouteTransactionList, strings.NewReader(`{"limit":"9"}`)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "pagination limit too big",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanPollTransaction: true,
						},
					},
					request: httptest.NewRequest("POST", RouteTransactionList, strings.NewReader(`{"limit":5000}`)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		// TODO: mock of the ledger to test for a good case
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &service{
				clients:  tt.fields.clients,
				registry: tt.fields.registry,
				wavelet:  tt.fields.wavelet,
				network:  tt.fields.network,
				upgrader: tt.fields.upgrader,
			}
			got, got1, err := s.listTransactionHandler(tt.args.ctx)
			assert.Equal(t, (err != nil), tt.wantErr, "service.listTransactionHandler() name = %s, error = %v, wantErr %v", tt.name, err, tt.wantErr)
			assert.Equal(t, got, tt.want, "service.listTransactionHandler() name = %s, got = %v, want %v", tt.name, got, tt.want)
			assert.True(t, reflect.DeepEqual(got1, tt.want1), "service.listTransactionHandler() name = %s, got1 = %v, want %v", tt.name, got1, tt.want1)
		})
	}
}

func Test_service_sessionInitHandler(t *testing.T) {
	t.Parallel()

	regTooManyEntries := newSessionRegistry()
	for i := 0; i < MaxAllowableSessions; i++ {
		regTooManyEntries.newSession(ClientPermissions{})
	}

	type fields struct {
		clients  map[string]*ClientInfo
		registry *registry
		wavelet  node.NodeInterface
		network  *network.Network
		upgrader websocket.Upgrader
	}
	type args struct {
		ctx *requestContext
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int
		want1   interface{}
		wantErr bool
	}{
		{
			name: "blank input",
			args: args{
				ctx: &requestContext{
					request: httptest.NewRequest("POST", RouteSessionInit, strings.NewReader(``)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "missing fields in request",
			args: args{
				ctx: &requestContext{
					request: httptest.NewRequest("POST", RouteSessionInit, strings.NewReader(`
						{
							"public_key":"71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858",
							"time_millis": 1540385725600
						}`)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "no permission",
			args: args{
				ctx: &requestContext{
					request: httptest.NewRequest("POST", RouteSessionInit, strings.NewReader(`
						{
							"public_key":"71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858",
							"time_millis": 1540385725600,
							"signature": "cd11300ad10025ea83adedc686665b25e697dcd76c83bcfc37a7d268182dd5033dd479c24bd28b23d142d62f0b677caeace1552046432fc6992d24cf4633cb0c"
						}`)),
				},
			},
			fields: fields{
				clients: map[string]*ClientInfo{},
			},
			want:    http.StatusForbidden,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "bad signature",
			args: args{
				ctx: &requestContext{
					request: httptest.NewRequest("POST", RouteSessionInit, strings.NewReader(`
						{
							"public_key":"71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858",
							"time_millis": 1540385725600,
							"signature": "bad_signature_ea83adedc686665b25e697dcd76c83bcfc37a7d268182dd5033dd479c24bd28b23d142d62f0b677caeace1552046432fc6992d24cf4633cb0c"
						}`)),
				},
			},
			fields: fields{
				clients: map[string]*ClientInfo{
					"71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858": &ClientInfo{
						PublicKey:   "71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858",
						Permissions: ClientPermissions{},
					},
				},
				registry: newSessionRegistry(),
			},
			want:    http.StatusForbidden,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "too many sessions already",
			args: args{
				ctx: &requestContext{
					request: httptest.NewRequest("POST", RouteSessionInit, strings.NewReader(`
						{
							"public_key":"71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858",
							"time_millis": 1540385725600,
							"signature": "cd11300ad10025ea83adedc686665b25e697dcd76c83bcfc37a7d268182dd5033dd479c24bd28b23d142d62f0b677caeace1552046432fc6992d24cf4633cb0c"
						}`)),
				},
			},
			fields: fields{
				clients: map[string]*ClientInfo{
					"71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858": &ClientInfo{
						PublicKey:   "71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858",
						Permissions: ClientPermissions{},
					},
				},
				registry: regTooManyEntries,
			},
			want:    http.StatusForbidden,
			want1:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &service{
				clients:  tt.fields.clients,
				registry: tt.fields.registry,
				wavelet:  tt.fields.wavelet,
				network:  tt.fields.network,
				upgrader: tt.fields.upgrader,
			}
			got, got1, err := s.sessionInitHandler(tt.args.ctx)
			assert.Equal(t, (err != nil), tt.wantErr, "service.sessionInitHandler() name = %s, error = %v, wantErr %v", tt.name, err, tt.wantErr)
			assert.Equal(t, got, tt.want, "service.sessionInitHandler() name = %s, got = %v, want %v", tt.name, got, tt.want)
			assert.True(t, reflect.DeepEqual(got1, tt.want1), "service.sessionInitHandler() name = %s, got1 = %v, want %v", tt.name, got1, tt.want1)
		})
	}
}

func Test_service_sendTransactionHandler(t *testing.T) {
	t.Parallel()

	var bigBytes bytes.Buffer
	bigBytes.Grow(1500)
	for i := 0; i < 1500; i++ {
		bigBytes.WriteByte((byte)(i % 10))
	}

	type fields struct {
		clients  map[string]*ClientInfo
		registry *registry
		wavelet  node.NodeInterface
		network  *network.Network
		upgrader websocket.Upgrader
	}
	type args struct {
		ctx *requestContext
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int
		want1   interface{}
		wantErr bool
	}{
		{
			name: "permission denied",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: false,
						},
					},
					request: httptest.NewRequest("POST", RouteTransactionSend, strings.NewReader(``)),
				},
			},
			want:    http.StatusForbidden,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "blank request",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: true,
						},
					},
					request: httptest.NewRequest("POST", RouteTransactionSend, strings.NewReader(`{}`)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "bad tag field",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: true,
						},
					},
					request: httptest.NewRequest("POST", RouteTransactionSend, strings.NewReader(`
					{
						"tag": "too-long-field-1234567890123456789012345678901",
						"payload": "doesn't matter"
					}`)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "bad payload field",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: true,
						},
					},
					request: httptest.NewRequest("POST", RouteTransactionSend, strings.NewReader(`
					{
						"tag": "transfer",
						"payload": "`+string(bigBytes.Bytes())+`"
					}`)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &service{
				clients:  tt.fields.clients,
				registry: tt.fields.registry,
				wavelet:  tt.fields.wavelet,
				network:  tt.fields.network,
				upgrader: tt.fields.upgrader,
			}
			got, got1, err := s.sendTransactionHandler(tt.args.ctx)
			assert.Equal(t, (err != nil), tt.wantErr, "service.sendTransactionHandler() name = %s, error = %v, wantErr %v", tt.name, err, tt.wantErr)
			assert.Equal(t, got, tt.want, "service.sendTransactionHandler() name = %s, got = %v, want %v", tt.name, got, tt.want)
			assert.True(t, reflect.DeepEqual(got1, tt.want1), "service.sendTransactionHandler() name = %s, got1 = %v, want %v", tt.name, got1, tt.want1)
		})
	}
}

func Test_service_sendContractHandler(t *testing.T) {
	t.Parallel()

	var contentType string
	bigBytes := &bytes.Buffer{}
	badFormField := &bytes.Buffer{}

	{
		bodyWriter := multipart.NewWriter(badFormField)
		bodyWriter.CreateFormFile("bad_form_field", "some_filename")
		bodyWriter.Close()
		contentType = bodyWriter.FormDataContentType()
	}
	{
		bodyWriter := multipart.NewWriter(bigBytes)
		fileWriter, _ := bodyWriter.CreateFormFile(UploadFormField, "some_filename")
		for i := 0; i < MaxRequestBodySize+1; i++ {
			fileWriter.Write([]byte("x"))
		}
		bodyWriter.Close()
	}

	type fields struct {
		clients  map[string]*ClientInfo
		registry *registry
		wavelet  node.NodeInterface
		network  *network.Network
		upgrader websocket.Upgrader
	}
	type args struct {
		ctx *requestContext
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int
		want1   interface{}
		wantErr bool
	}{
		{
			name: "permission denied",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: false,
						},
					},
					request: httptest.NewRequest("POST", RouteContractSend, strings.NewReader(``)),
				},
			},
			want:    http.StatusForbidden,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "blank request",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: true,
						},
					},
					request: httptest.NewRequest("POST", RouteContractSend, strings.NewReader(``)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "form type but wrong content type",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: true,
						},
					},
					request: &http.Request{
						Header: map[string][]string{
							"Content-type": []string{"bad"},
						},
						Body: ioutil.NopCloser(strings.NewReader(`doesn't matter`)),
					},
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "bad form field",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: true,
						},
					},
					request: &http.Request{
						Header: map[string][]string{
							"Content-type": []string{contentType},
						},
						Body: ioutil.NopCloser(bytes.NewReader(badFormField.Bytes())),
					},
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "content too big",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: true,
						},
					},
					request: &http.Request{
						Header: map[string][]string{
							"Content-type": []string{contentType},
						},
						Body: ioutil.NopCloser(bytes.NewReader(bigBytes.Bytes())),
					},
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &service{
				clients:  tt.fields.clients,
				registry: tt.fields.registry,
				wavelet:  tt.fields.wavelet,
				network:  tt.fields.network,
				upgrader: tt.fields.upgrader,
			}
			got, got1, err := s.sendContractHandler(tt.args.ctx)
			assert.Equal(t, (err != nil), tt.wantErr, "service.sendContractHandler() name = %s, error = %v, wantErr %v", tt.name, err, tt.wantErr)
			assert.Equal(t, got, tt.want, "service.sendContractHandler() name = %s, got = %v, want %v", tt.name, got, tt.want)
			assert.True(t, reflect.DeepEqual(got1, tt.want1), "service.sendContractHandler() name = %s, got1 = %v, want %v", tt.name, got1, tt.want1)
		})
	}
}

func Test_service_getContractHandler(t *testing.T) {
	type fields struct {
		clients  map[string]*ClientInfo
		registry *registry
		wavelet  node.NodeInterface
		network  *network.Network
		upgrader websocket.Upgrader
	}
	type args struct {
		ctx *requestContext
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int
		want1   interface{}
		wantErr bool
	}{
		{
			name: "permission denied",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanAccessLedger: false,
						},
					},
					request: httptest.NewRequest("POST", RouteContractGet, strings.NewReader(``)),
				},
			},
			want:    http.StatusForbidden,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "blank request",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanAccessLedger: true,
						},
					},
					request: httptest.NewRequest("POST", RouteContractGet, strings.NewReader(``)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "bad transaction id",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanAccessLedger: true,
						},
					},
					request: httptest.NewRequest("POST", RouteContractGet, strings.NewReader(`{"transaction_id":"too_short"}`)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &service{
				clients:  tt.fields.clients,
				registry: tt.fields.registry,
				wavelet:  tt.fields.wavelet,
				network:  tt.fields.network,
				upgrader: tt.fields.upgrader,
			}
			got, got1, err := s.getContractHandler(tt.args.ctx)
			assert.Equal(t, (err != nil), tt.wantErr, "service.getContractHandler() name = %s, error = %v, wantErr %v", tt.name, err, tt.wantErr)
			assert.Equal(t, got, tt.want, "service.getContractHandler() name = %s, got = %v, want %v", tt.name, got, tt.want)
			assert.True(t, reflect.DeepEqual(got1, tt.want1), "service.getContractHandler() name = %s, got1 = %v, want %v", tt.name, got1, tt.want1)
		})
	}
}

func Test_service_findParentsHandler(t *testing.T) {
	type fields struct {
		clients  map[string]*ClientInfo
		registry *registry
		wavelet  node.NodeInterface
		network  *network.Network
		upgrader websocket.Upgrader
	}
	type args struct {
		ctx *requestContext
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int
		want1   interface{}
		wantErr bool
	}{
		{
			name: "permission denied",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: false,
						},
					},
					request: httptest.NewRequest("POST", RouteTransactionForward, strings.NewReader(``)),
				},
			},
			want:    http.StatusForbidden,
			want1:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &service{
				clients:  tt.fields.clients,
				registry: tt.fields.registry,
				wavelet:  tt.fields.wavelet,
				network:  tt.fields.network,
				upgrader: tt.fields.upgrader,
			}
			got, got1, err := s.findParentsHandler(tt.args.ctx)
			assert.Equal(t, (err != nil), tt.wantErr, "service.findParentsHandler() name = %s, error = %v, wantErr %v", tt.name, err, tt.wantErr)
			assert.Equal(t, got, tt.want, "service.findParentsHandler() name = %s, got = %v, want %v", tt.name, got, tt.want)
			assert.True(t, reflect.DeepEqual(got1, tt.want1), "service.findParentsHandler() name = %s, got1 = %v, want %v", tt.name, got1, tt.want1)
		})
	}
}

func Test_service_forwardTransactionHandler(t *testing.T) {
	type fields struct {
		clients  map[string]*ClientInfo
		registry *registry
		wavelet  node.NodeInterface
		network  *network.Network
		upgrader websocket.Upgrader
	}
	type args struct {
		ctx *requestContext
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int
		want1   interface{}
		wantErr bool
	}{
		{
			name: "permission denied",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: false,
						},
					},
					request: httptest.NewRequest("POST", RouteTransactionForward, strings.NewReader(``)),
				},
			},
			want:    http.StatusForbidden,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "blank request",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: true,
						},
					},
					request: httptest.NewRequest("POST", RouteTransactionForward, strings.NewReader(``)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "bad signature, bad sender hex",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: true,
						},
					},
					request: httptest.NewRequest("POST", RouteSessionInit, strings.NewReader(`
						{
							"sender": "bad_1d",
							"nonce": 1,
							"parents": ["d0b587b71268b267d846d38b4aa1d5f1f524c51a9a4a98cf4fd5c0951204620f"],
							"tag": 1,
							"payload": "",
							"signature": "a63b290c91162f692c22c70ebda5e9218c542a32879f6229984b1ae6eda1188b324aafa2150d25a20a71dbcddc395caeb6b5373272bd6462f2cc347e63377c0c"
						}`)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "bad signature, bad parent hex",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: true,
						},
					},
					request: httptest.NewRequest("POST", RouteSessionInit, strings.NewReader(`
						{
							"sender": "71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858",
							"nonce": 2,
							"parents": ["123"],
							"tag": 1,
							"payload": "",
							"signature": "a63b290c91162f692c22c70ebda5e9218c542a32879f6229984b1ae6eda1188b324aafa2150d25a20a71dbcddc395caeb6b5373272bd6462f2cc347e63377c0c"
						}`)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "bad signature, nonce was incremented",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanSendTransaction: true,
						},
					},
					request: httptest.NewRequest("POST", RouteSessionInit, strings.NewReader(`
						{
							"sender": "71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858",
							"nonce": 2,
							"parents": ["d0b587b71268b267d846d38b4aa1d5f1f524c51a9a4a98cf4fd5c0951204620f"],
							"tag": 1,
							"payload": "",
							"signature": "a63b290c91162f692c22c70ebda5e9218c542a32879f6229984b1ae6eda1188b324aafa2150d25a20a71dbcddc395caeb6b5373272bd6462f2cc347e63377c0c"
						}`)),
				},
			},
			want:    http.StatusBadRequest,
			want1:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &service{
				clients:  tt.fields.clients,
				registry: tt.fields.registry,
				wavelet:  tt.fields.wavelet,
				network:  tt.fields.network,
				upgrader: tt.fields.upgrader,
			}
			got, got1, err := s.forwardTransactionHandler(tt.args.ctx)
			assert.Equal(t, (err != nil), tt.wantErr, "service.forwardTransactionHandler() name = %s, error = %v, wantErr %v", tt.name, err, tt.wantErr)
			assert.Equal(t, got, tt.want, "service.forwardTransactionHandler() name = %s, got = %v, want %v", tt.name, got, tt.want)
			assert.True(t, reflect.DeepEqual(got1, tt.want1), "service.forwardTransactionHandler() name = %s, got1 = %v, want %v", tt.name, got1, tt.want1)
		})
	}
}
