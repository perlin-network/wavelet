package api

import (
	_ "expvar"

	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/perlin-network/noise/network"

	"github.com/gorilla/websocket"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/rs/cors"
	"net/http/pprof"
)

// service represents a service.
type service struct {
	clients  map[string]*ClientInfo
	registry *registry
	wavelet  *node.Wavelet
	network  *network.Network
	upgrader websocket.Upgrader
}

// requestContext represents a context for a request.
type requestContext struct {
	service  *service
	response http.ResponseWriter
	request  *http.Request

	session *session
}

type requestTermination struct{}

// readJSON decodes a HTTP requests JSON body into a struct.
func (c *requestContext) readJSON(out interface{}) error {
	r := io.LimitReader(c.request.Body, 4096*1024)
	defer c.request.Body.Close()

	data, err := ioutil.ReadAll(r)
	if err != nil {
		c.WriteJSON(http.StatusBadRequest, "bad request body")
		return err
	}

	if err = json.Unmarshal(data, out); err != nil {
		c.WriteJSON(http.StatusBadRequest, "malformed json")
		return err
	}
	return nil
}

// WriteJSON will write a given status code & JSON to a response.
func (c *requestContext) WriteJSON(status int, data interface{}) {
	out, err := json.Marshal(data)
	if err != nil {
		c.WriteJSON(http.StatusInternalServerError, "server error")
		return
	}
	c.response.Header().Set("Content-Type", "application/json")
	c.response.WriteHeader(status)
	c.response.Write(out)
}

// requireHeader returns a header value if presents or stops request with a bad request response.
func (c *requestContext) requireHeader(names ...string) string {
	for _, name := range names {
		values, ok := c.request.Header[name]

		if ok && len(values) > 0 {
			return values[0]
		}
	}

	c.WriteJSON(http.StatusBadRequest, "required header not found")
	return ""
}

// loadSession sets a session for a request.
func (c *requestContext) loadSession() bool {
	token := c.requireHeader("X-Session-Token", "Sec-Websocket-Protocol")

	session, ok := c.service.registry.getSession(token)

	if !ok {
		c.WriteJSON(http.StatusForbidden, "session not found")
		return false
	}

	session.renew()

	c.session = session
	return true
}

// init registers routes to the HTTP serve mux.
func (s *service) init(mux *http.ServeMux) {
	mux.Handle("/debug/vars", http.DefaultServeMux)

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	mux.HandleFunc("/session/init", s.wrap(s.sessionInitHandler))
	mux.HandleFunc("/ledger/state", s.wrap(s.ledgerStateHandler))
	mux.HandleFunc("/transaction/list", s.wrap(s.listTransactionHandler))
	mux.HandleFunc("/transaction/poll", s.wrap(s.pollTransactionHandler))
	mux.HandleFunc("/transaction/send", s.wrap(s.sendTransactionHandler))
	mux.HandleFunc("/stats/reset", s.wrap(s.resetStatsHandler))
	mux.HandleFunc("/account/load", s.wrap(s.loadAccountHandler))
	mux.HandleFunc("/account/poll", s.wrap(s.pollAccountHandler))
}

// wrap applies middleware to a HTTP request handler.
func (s *service) wrap(inner func(*requestContext)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				if _, ok := err.(requestTermination); ok {
					return
				}

				log.Error().
					Interface("error", err).
					Interface("url", r.URL).
					Str("stack_trace", string(debug.Stack())).
					Msg("An error occured from the API.")
			}
		}()

		inner(&requestContext{
			service:  s,
			response: w,
			request:  r,
		})
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
		panic(err)
	}
}
