package api

import (
	_ "expvar"
	"net/http"
	"net/http/pprof"
	"runtime/debug"
	"time"

	"github.com/gorilla/websocket"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/rs/cors"
)

// service represents a service.
type service struct {
	clients  map[string]*ClientInfo
	registry *registry
	wavelet  *node.Wavelet
	network  *network.Network
	upgrader websocket.Upgrader
}

type requestTermination struct{}

// init registers routes to the HTTP serve mux.
func (s *service) init(mux *http.ServeMux) {
	mux.Handle("/debug/vars", http.DefaultServeMux)

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	mux.HandleFunc(RouteSessionInit, s.wrap(s.sessionInitHandler))
	mux.HandleFunc(RouteLedgerState, s.wrap(s.ledgerStateHandler))
	mux.HandleFunc(RouteTransactionList, s.wrap(s.listTransactionHandler))
	mux.HandleFunc(RouteTransactionPoll, s.wrap(s.pollTransactionHandler))
	mux.HandleFunc(RouteTransactionSend, s.wrap(s.sendTransactionHandler))
	mux.HandleFunc(RouteStatsReset, s.wrap(s.resetStatsHandler))
	mux.HandleFunc(RouteAccountLoad, s.wrap(s.loadAccountHandler))
	mux.HandleFunc(RouteAccountPoll, s.wrap(s.pollAccountHandler))
	mux.HandleFunc(RouteServerVersion, s.wrap(s.serverVersionHandler))
}

// wrap applies middleware to a HTTP request handler.
func (s *service) wrap(handler func(*requestContext) (int, interface{}, error)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c := &requestContext{
			service:  s,
			response: w,
			request:  r,
		}
		defer func() {
			if err := recover(); err != nil {
				if _, ok := err.(requestTermination); ok {
					return
				}

				log.Error().
					Interface("url", r.URL).
					Msgf("An error occured from the API: %s", string(debug.Stack()))

				// return a 500 on a panic
				c.WriteJSON(http.StatusInternalServerError, ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Error:      err,
				})
			}
		}()
		statusCode, data, err := handler(c)
		if err != nil {
			log.Error().
				Interface("url", r.URL).
				Interface("statusCode", statusCode).
				Msgf("An error occured from the API: %+v", err)

			c.WriteJSON(statusCode, ErrorResponse{
				StatusCode: statusCode,
				Error:      err.Error(),
			})
		} else {
			c.WriteJSON(statusCode, data)
		}
	}
}

// Run runs the API server with a specified set of options.
func Run(net *network.Network, opts Options) {
	plugin, exists := net.Plugin(node.PluginID)
	if !exists {
		panic("ledger plugin not found")
	}

	registry := newSessionRegistry()

	go func() {
		for range time.Tick(10 * time.Second) {
			registry.Recycle()
		}
	}()

	clients := make(map[string]*ClientInfo)

	for _, client := range opts.Clients {
		clients[client.PublicKey] = client
	}

	mux := http.NewServeMux()

	service := &service{
		clients:  clients,
		registry: newSessionRegistry(),
		wavelet:  plugin.(*node.Wavelet),
		network:  net,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}

	service.init(mux)

	handler := cors.AllowAll().Handler(mux)

	server := &http.Server{
		Addr:    opts.ListenAddr,
		Handler: handler,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatal().Err(err).Msg("")
	}
}
