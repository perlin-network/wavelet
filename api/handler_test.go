package api

import (
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
		wavelet  *node.Wavelet
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
			if (err != nil) != tt.wantErr {
				t.Errorf("service.resetStatsHandler() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("service.resetStatsHandler() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("service.resetStatsHandler() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_service_listTransactionHandler(t *testing.T) {
	t.Parallel()
	type fields struct {
		clients  map[string]*ClientInfo
		registry *registry
		wavelet  *node.Wavelet
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
			name: "malformed input",
			args: args{
				ctx: &requestContext{
					session: &session{
						Permissions: ClientPermissions{
							CanPollTransaction: true,
						},
					},
					request: httptest.NewRequest("POST", RouteTransactionList, strings.NewReader(`{"broken:json`)),
				},
			},
			want:    http.StatusBadRequest,
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
			if (err != nil) != tt.wantErr {
				t.Errorf("service.listTransactionHandler() name = %s, error = %v, wantErr %v", tt.name, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("service.listTransactionHandler() name = %s, got = %v, want %v", tt.name, got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("service.listTransactionHandler() name = %s, got1 = %v, want %v", tt.name, got1, tt.want1)
			}
		})
	}
}
