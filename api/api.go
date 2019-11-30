package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/buaazp/fasthttprouter"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/debounce"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/pprofhandler"
	"github.com/valyala/fastjson"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

type Gateway struct {
	*Config
	addr string
	tls  *tls.Config

	router    *fasthttprouter.Router
	server    *fasthttp.Server
	serverTLS *fasthttp.Server

	sinks     map[string]*sink
	sinksLock sync.RWMutex

	enableTimeout bool

	rateLimiter *rateLimiter

	parserPool *fastjson.ParserPool
	arenaPool  *fastjson.ArenaPool
}

type Config struct {
	Port int // for HTTP only, HTTPS is hard-coded to be :443

	Client *skademlia.Client
	Ledger *wavelet.Ledger
	KV     store.KV
	Keys   *skademlia.Keypair

	RequestsPerSecond float64

	// Both needs to be non-empty for TLS to be enabled.
	HostPolicy   string
	CertCacheDir string
}

func New(opts *Config) *Gateway {
	g := &Gateway{
		Config:      opts,
		sinks:       make(map[string]*sink),
		parserPool:  new(fastjson.ParserPool),
		arenaPool:   new(fastjson.ArenaPool),
		rateLimiter: newRateLimiter(1000),
	}

	if opts.RequestsPerSecond > 0 {
		g.rateLimiter = newRateLimiter(opts.RequestsPerSecond)
	}

	g.addr = ":" + strconv.Itoa(opts.Port)

	// Set up TLS if available
	if opts.HostPolicy != "" && opts.CertCacheDir != "" {
		// Copied from autocert.Manager.TLSConfig(), with "h2" removed in
		// NextProtos.
		g.tls = &tls.Config{
			GetCertificate: (&autocert.Manager{
				Prompt:     autocert.AcceptTOS,
				Cache:      autocert.DirCache(opts.CertCacheDir),
				HostPolicy: autocert.HostWhitelist(opts.HostPolicy),
			}).GetCertificate,

			NextProtos: []string{
				"http/1.1",
				acme.ALPNProto,
			},
		}
	}

	// Setup websocket logging sinks.
	sinkNetwork := g.registerWebsocketSink("ws://network/", nil)
	sinkConsensus := g.registerWebsocketSink("ws://consensus/", nil)
	sinkStake := g.registerWebsocketSink("ws://stake/?id=account_id", nil)
	sinkAccounts := g.registerWebsocketSink(
		"ws://accounts/?id=account_id",
		debounce.NewFactory(debounce.TypeDeduper,
			debounce.WithPeriod(500*time.Millisecond),
			debounce.WithKeys("account_id", "event"),
		),
	)
	sinkContracts := g.registerWebsocketSink(
		"ws://contract/?id=contract_id",
		debounce.NewFactory(debounce.TypeDeduper,
			debounce.WithPeriod(500*time.Millisecond),
			debounce.WithKeys("contract_id"),
		),
	)
	sinkTransactions := g.registerWebsocketSink(
		"ws://tx/?id=tx_id&sender=sender_id&creator=creator_id&tag=tag",
		debounce.NewFactory(debounce.TypeLimiter,
			debounce.WithPeriod(2200*time.Millisecond),
			debounce.WithBufferLimit(1638400),
		),
	)
	sinkMetrics := g.registerWebsocketSink("ws://metrics/", nil)

	log.SetLevel("info")
	log.SetWriter(log.LoggerWebsocket, g)

	// Setup HTTP router.
	g.router = fasthttprouter.New()

	// If the route does not exist for a method type (e.g. OPTIONS),
	// fasthttprouter will consider it to not exist. So, we need to override
	// notFound handler for OPTIONS method type to handle CORS.
	g.router.HandleOPTIONS = false
	g.router.NotFound = g.notFound()

	// Websocket endpoints.
	g.routeWithMiddleware("GET", "/poll/network",
		g.poll(sinkNetwork), true)
	g.routeWithMiddleware("GET", "/poll/consensus",
		g.poll(sinkConsensus), true)
	g.routeWithMiddleware("GET", "/poll/stake",
		g.poll(sinkStake), true)
	g.routeWithMiddleware("GET", "/poll/accounts",
		g.poll(sinkAccounts), true)
	g.routeWithMiddleware("GET", "/poll/contract",
		g.poll(sinkContracts), true)
	g.routeWithMiddleware("GET", "/poll/tx",
		g.poll(sinkTransactions), true)
	g.routeWithMiddleware("GET", "/poll/metrics",
		g.poll(sinkMetrics), true)

	// Debug endpoint.
	g.routeWithMiddleware("GET", "/debug/*p",
		pprofhandler.PprofHandler, true)

	// Ledger endpoint.
	g.routeWithMiddleware("GET", "/ledger",
		g.ledgerStatus, true)

	// Account endpoints.
	g.routeWithMiddleware("GET", "/accounts/:id",
		g.getAccount, false)

	// Contract endpoints.
	g.routeWithMiddleware("GET", "/contract/:id/page/:index",
		g.getContractPages, true, g.contractScope)
	g.routeWithMiddleware("GET", "/contract/:id/page",
		g.getContractPages, true, g.contractScope)
	g.routeWithMiddleware("GET", "/contract/:id",
		g.getContractCode, true, g.contractScope)

	// Transaction endpoints.
	g.routeWithMiddleware("POST", "/tx/send",
		g.sendTransaction, false)
	g.routeWithMiddleware("GET", "/tx/:id",
		g.getTransaction, false)
	g.routeWithMiddleware("GET", "/tx",
		g.listTransactions, true)

	// Connectivity endpoints
	g.routeWithMiddleware("POST", "/node/connect",
		g.connect, true, g.auth)
	g.routeWithMiddleware("POST", "/node/disconnect",
		g.disconnect, true, g.auth)
	g.routeWithMiddleware("POST", "/node/restart",
		g.restart, true, g.auth)

	g.server = &fasthttp.Server{Handler: g.router.Handler}

	if g.tls != nil {
		g.serverTLS = &fasthttp.Server{Handler: g.router.Handler}
	}

	// Register node events
	g.registerEvents()

	return g
}

// Start listens to the given port and TLS if given. It does not block.
func (g *Gateway) Start() error {
	// Start the cleanup daemon
	stop := g.rateLimiter.cleanup(10 * time.Minute)

	// Create a new HTTP listener at arbitrary port given in (*Config).Port
	httpLn, err := net.Listen("tcp4", g.addr)
	if err != nil {
		return errors.Wrap(err, "Failed to listen to "+g.addr)
	}

	// Create a new server
	g.server = &fasthttp.Server{Handler: g.router.Handler}
	go func() {
		defer stop()

		// Listen to the listener in the background
		if err := g.server.Serve(httpLn); err != nil {
			log.Node().Fatal().Err(err).
				Str("addr", g.addr).
				Msg("Failed to start the HTTP server.")
		}
	}()

	log.Node().Info().
		Str("addr", g.addr).
		Msg("Started the HTTP API server.")

	if g.tls != nil {
		tlsLn, err := net.Listen("tcp", ":443")
		if err != nil {
			return errors.Wrap(err, "Failed to listen to port 443")
		}

		g.serverTLS = &fasthttp.Server{Handler: g.router.Handler}
		go func() {
			// Listen to the wrapped TLS listener
			if err := g.serverTLS.Serve(tls.NewListener(tlsLn, g.tls)); err != nil {
				log.Node().Fatal().Err(err).
					Str("addr", g.addr).
					Msg("Failed to start the HTTP server.")
			}
		}()

		log.Node().Info().
			Str("addr", ":443").
			Msg("Started the HTTPS/TLS API server.")
	}

	return nil
}

func (g *Gateway) Shutdown() {
	if g.serverTLS != nil {
		if err := g.serverTLS.Shutdown(); err != nil {
			log.Node().Error().
				Err(err).
				Msg("Failed to stop the HTTPS/TLS server")
		}
	}

	if err := g.server.Shutdown(); err != nil {
		log.Node().Error().
			Err(err).
			Msg("Failed to stop the HTTP server")
	}
}

// helper fn to add middlewares
func (g *Gateway) routeWithMiddleware(method, route string,
	h fasthttp.RequestHandler, rateLimit bool, ms ...middleware) {

	// Middlewares to prepend to ms
	var topMs = make([]middleware, 0, 4)

	// Prepend the recoverer
	topMs = append(topMs, recoverer)

	if rateLimit {
		// Prepend the rate limiter middleware
		topMs = append(topMs, g.rateLimiter.limit(route))
	}

	// Prepend the CORS middleware
	topMs = append(topMs, cors())

	if g.enableTimeout {
		topMs = append(topMs, timeout(60*time.Second, "Request timed out."))
	}

	g.router.Handle(method, route, chain(h, append(topMs, ms...)))
}

func (g *Gateway) notFound() func(ctx *fasthttp.RequestCtx) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	notFoundHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.Error(fasthttp.StatusMessage(fasthttp.StatusNotFound),
			fasthttp.StatusNotFound)
	}

	// This cors is only for OPTIONS, so we can pass any handler since it will not be triggered.
	cors := cors()(notFoundHandler)

	lookupCtx := &fasthttp.RequestCtx{}

	return func(ctx *fasthttp.RequestCtx) {
		if string(ctx.Method()) != "OPTIONS" {
			notFoundHandler(ctx)
			return
		}

		path := string(ctx.Path())

		// Only proceed to cors if the route really exist.
		// We try to look the route for other method types.
		for _, m := range methods {
			h, _ := g.router.Lookup(m, path, lookupCtx)
			if h != nil {
				cors(ctx)
				return
			}
		}

		notFoundHandler(ctx)
	}
}

func (g *Gateway) poll(sink *sink) func(ctx *fasthttp.RequestCtx) {
	return func(ctx *fasthttp.RequestCtx) {
		if err := sink.serve(ctx); err != nil {
			g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "failed to init websocket session")))
		}
	}
}

func (g *Gateway) registerWebsocketSink(rawURL string, factory *debounce.Factory) *sink {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}

	values := u.Query()

	filters := make(map[string]string)

	for key := range values {
		filters[key] = values.Get(key)
	}

	sink := &sink{
		ops:     make(chan func(map[*client]struct{})),
		filters: filters,
		join:    make(chan *client),
		leave:   make(chan *client),
	}

	if factory != nil {
		sink.debouncer = factory.Init(context.Background(), debounce.WithAction(sink.debounce))
	}

	go sink.run()

	g.sinksLock.Lock()
	g.sinks[u.Hostname()] = sink
	g.sinksLock.Unlock()

	return sink
}

func (g *Gateway) Write(buf []byte) (n int, err error) {
	p := g.parserPool.Get()
	defer g.parserPool.Put(p)

	v, err := p.ParseBytes(buf)
	if err != nil {
		return n, errors.Errorf("cannot parse: %q", err)
	}

	mod := v.GetStringBytes(log.KeyModule)
	if len(mod) == 0 {
		return n, errors.Errorf("all logs must have the field %q", log.KeyModule)
	}

	sink, exists := g.sinks[string(mod)]
	if !exists {
		return len(buf), nil
	}

	cpy := make([]byte, len(buf))
	copy(cpy, buf)

	sink.broadcast(broadcastItem{value: v, buf: cpy})

	return len(buf), nil
}

func (g *Gateway) render(ctx *fasthttp.RequestCtx, m log.MarshalableArena) {
	g._render(ctx, m, http.StatusOK)
}

func (g *Gateway) renderError(ctx *fasthttp.RequestCtx, e *ErrResponse) {
	g._render(ctx, e, e.HTTPStatusCode)
}

func (g *Gateway) _render(ctx *fasthttp.RequestCtx, m log.MarshalableArena, status int) {
	arena := g.arenaPool.Get()
	defer g.arenaPool.Put(arena)

	b, err := m.MarshalArena(arena)
	if err != nil {
		ctx.Error(fmt.Sprintf(`{ "error": "render error: %s" }`, err.Error()), http.StatusInternalServerError)
		return
	}

	ctx.SetContentType("application/json")
	ctx.Response.SetStatusCode(status)
	ctx.Response.SetBody(b)
}
