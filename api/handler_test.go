package api

import (
	"net/http"
	"reflect"
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
