package api

import (
	_ "expvar"
	"net/http"
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
	wavelet  node.NodeInterface
	network  *network.Network
	upgrader websocket.Upgrader
}

// init registers routes to the HTTP serve mux.
func (s *service) init(mux *http.ServeMux) {
	mux.Handle("/debug/vars", http.DefaultServeMux)

	mux.HandleFunc(RouteSessionInit, s.wrap(s.sessionInitHandler))

	mux.HandleFunc(RouteLedgerState, s.wrap(s.ledgerStateHandler))

	mux.HandleFunc(RouteTransactionList, s.wrap(s.listTransactionHandler))
	mux.HandleFunc(RouteTransactionPoll, s.wrap(s.pollTransactionHandler))
	mux.HandleFunc(RouteTransactionSend, s.wrap(s.sendTransactionHandler))
	mux.HandleFunc(RouteTransaction, s.wrap(s.getTransactionHandler))
	mux.HandleFunc(RouteTransactionForward, s.wrap(s.forwardTransactionHandler))
	mux.HandleFunc(RouteTransactionFindParents, s.wrap(s.findParentsHandler))

	mux.HandleFunc(RouteContractSend, s.wrap(s.sendContractHandler))
	mux.HandleFunc(RouteContractGet, s.wrap(s.getContractHandler))
	mux.HandleFunc(RouteContractList, s.wrap(s.listContractsHandler))
	mux.HandleFunc(RouteContractExecute, s.wrap(s.executeContractHandler))

	mux.HandleFunc(RouteStatsReset, s.wrap(s.resetStatsHandler))

	mux.HandleFunc(RouteAccountGet, s.wrap(s.getAccountHandler))
	mux.HandleFunc(RouteAccountPoll, s.wrap(s.pollAccountHandler))

	mux.HandleFunc(RouteServerVersion, s.wrap(s.serverVersionHandler))
}

// Run runs the API server with a specified set of options.
func Run(net *network.Network, wavelet node.NodeInterface, sc chan *http.Server, opts Options) {
	if wavelet == nil {
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
		wavelet:  wavelet,
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

	if sc != nil {
		sc <- server
	}

	if err := server.ListenAndServe(); err != nil {
		log.Error().Err(err).Msg(" ")
	}

}
