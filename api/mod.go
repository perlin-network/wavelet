package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/render"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/pkg/errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Gateway struct {
	node   *noise.Node
	ledger *wavelet.Ledger

	network *skademlia.Protocol
	keys    *skademlia.Keypair

	router chi.Router

	registry *sessionRegistry
	sinks    map[string]*sink
}

func New() *Gateway {
	return &Gateway{registry: newSessionRegistry(), sinks: make(map[string]*sink)}
}

func (g *Gateway) setup() {
	// Setup websocket logging sinks.

	sinkNetwork := g.registerWebsocketSink("ws://network/")
	sinkBroadcaster := g.registerWebsocketSink("ws://broadcaster/")
	sinkConsensus := g.registerWebsocketSink("ws://consensus/")
	sinkStake := g.registerWebsocketSink("ws://stake/")

	sinkAccounts := g.registerWebsocketSink("ws://accounts/?id=account_id")
	sinkContracts := g.registerWebsocketSink("ws://contract/?id=contract_id")
	sinkTransactions := g.registerWebsocketSink("ws://tx/?id=tx_id&sender=sender_id&creator=creator_id")

	log.Register(g)

	// Setup HTTP router.

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	}).Handler)
	r.Use(middleware.Timeout(60 * time.Second))

	// Websocket endpoints.

	r.Get("/network/poll", g.securePoll(sinkNetwork))
	r.Get("/broadcaster/poll", g.securePoll(sinkBroadcaster))
	r.Get("/consensus/poll", g.securePoll(sinkConsensus))
	r.Get("/stake/poll", g.securePoll(sinkStake))
	r.Get("/accounts/poll", g.securePoll(sinkAccounts))
	r.Get("/contract/poll", g.securePoll(sinkContracts))
	r.Get("/tx/poll", g.securePoll(sinkTransactions))

	// HTTP endpoints.

	r.Post("/session/init", g.initSession)
	r.Mount("/debug", middleware.Profiler())

	r.With(g.authenticated).Get("/ledger", g.ledgerStatus)
	r.With(g.authenticated).Get("/accounts/{id}", g.getAccount)

	r.With(g.authenticated).Route("/contract/{id}", func(r chi.Router) {
		r.Use(g.contractScope)

		r.Get("/", g.getContractCode)

		r.Route("/page", func(r chi.Router) {
			r.Get("/", g.getContractPages)
			r.Get("/{index}", g.getContractPages)
		})
	})

	r.With(g.authenticated).Route("/tx", func(r chi.Router) {
		r.Get("/", g.listTransactions)
		r.Get("/{id}", g.getTransaction)
		r.Post("/send", g.sendTransaction)
	})

	g.router = r
}

func (g *Gateway) StartHTTP(port int, n *noise.Node, l *wavelet.Ledger, nn *skademlia.Protocol, k *skademlia.Keypair) {
	g.node = n
	g.ledger = l

	g.network = nn
	g.keys = k

	g.setup()

	logger := log.Node()
	logger.Info().Int("port", port).Msg("Started HTTP API server.")

	if err := http.ListenAndServe(":"+strconv.Itoa(port), g.router); err != nil {
		logger.Fatal().Err(err).Msg("Failed to start HTTP server.")
	}
}

func (g *Gateway) initSession(w http.ResponseWriter, r *http.Request) {
	req := new(SessionInitRequest)

	if err := render.Bind(r, req); err != nil {
		g.render(w, r, ErrBadRequest(err))
		return
	}

	session, err := g.registry.newSession()
	if err != nil {
		g.render(w, r, ErrBadRequest(errors.Wrap(err, "failed to create session")))
		return
	}

	g.render(w, r, &SessionInitResponse{Token: session.id})
}

func (g *Gateway) sendTransaction(w http.ResponseWriter, r *http.Request) {
	req := new(SendTransactionRequest)

	if err := render.Bind(r, req); err != nil {
		g.render(w, r, ErrBadRequest(err))
		return
	}

	evt := wavelet.EventBroadcast{
		Tag:       req.Tag,
		Payload:   req.payload,
		Creator:   req.creator,
		Signature: req.signature,
		Result:    make(chan wavelet.Transaction, 1),
		Error:     make(chan error, 1),
	}

	select {
	case <-time.After(1 * time.Second):
		g.render(w, r, ErrInternal(errors.New("broadcasting queue is full")))
		return
	case g.ledger.BroadcastQueue <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		g.render(w, r, ErrInternal(errors.New("its taking too long to broadcast your transaction")))
		return
	case err := <-evt.Error:
		g.render(w, r, ErrInternal(errors.Wrap(err, "got an error broadcasting yourt ransaction")))
		return
	case tx := <-evt.Result:
		g.render(w, r, &SendTransactionResponse{ledger: g.ledger, tx: &tx})
	}
}

func (g *Gateway) ledgerStatus(w http.ResponseWriter, r *http.Request) {
	g.render(w, r, &LedgerStatusResponse{node: g.node, ledger: g.ledger, network: g.network, publicKey: g.keys.PublicKey()})
}

func (g *Gateway) listTransactions(w http.ResponseWriter, r *http.Request) {
	var sender common.AccountID
	var creator common.AccountID
	var offset, limit uint64
	var err error

	if raw := r.URL.Query().Get("sender"); len(raw) > 0 {
		slice, err := hex.DecodeString(raw)

		if err != nil {
			g.render(w, r, ErrBadRequest(errors.Wrap(err, "sender ID must be presented as valid hex")))
			return
		}

		if len(slice) != common.SizeAccountID {
			g.render(w, r, ErrBadRequest(errors.Errorf("sender ID must be %d bytes long", common.SizeAccountID)))
			return
		}

		copy(sender[:], slice)
	}

	if raw := r.URL.Query().Get("creator"); len(raw) > 0 {
		slice, err := hex.DecodeString(raw)

		if err != nil {
			g.render(w, r, ErrBadRequest(errors.Wrap(err, "creator ID must be presented as valid hex")))
			return
		}

		if len(slice) != common.SizeAccountID {
			g.render(w, r, ErrBadRequest(errors.Errorf("creator ID must be %d bytes long", common.SizeAccountID)))
			return
		}

		copy(creator[:], slice)
	}

	if raw := r.URL.Query().Get("offset"); len(raw) > 0 {
		offset, err = strconv.ParseUint(raw, 10, 64)

		if err != nil {
			g.render(w, r, ErrBadRequest(errors.Wrap(err, "could not parse offset")))
			return
		}
	}

	if raw := r.URL.Query().Get("limit"); len(raw) > 0 {
		limit, err = strconv.ParseUint(raw, 10, 64)

		if err != nil {
			g.render(w, r, ErrBadRequest(errors.Wrap(err, "could not parse limit")))
			return
		}
	}

	var transactions []render.Renderer

	for _, tx := range g.ledger.ListTransactions(offset, limit, sender, creator) {
		transactions = append(transactions, &Transaction{tx: tx})
	}

	g.renderList(w, r, transactions)
}

func (g *Gateway) getTransaction(w http.ResponseWriter, r *http.Request) {
	param := chi.URLParam(r, "id")

	slice, err := hex.DecodeString(param)
	if err != nil {
		g.render(w, r, ErrBadRequest(errors.Wrap(err, "transaction ID must be presented as valid hex")))
		return
	}

	if len(slice) != common.SizeTransactionID {
		g.render(w, r, ErrBadRequest(errors.Errorf("transaction ID must be %d bytes long", common.SizeTransactionID)))
		return
	}

	var id common.TransactionID
	copy(id[:], slice)

	tx, exists := g.ledger.FindTransaction(id)

	if !exists {
		g.render(w, r, ErrBadRequest(errors.Errorf("could not find transaction with ID %x", id)))
		return
	}

	g.render(w, r, &Transaction{tx: tx})
}

func (g *Gateway) getAccount(w http.ResponseWriter, r *http.Request) {
	param := chi.URLParam(r, "id")

	slice, err := hex.DecodeString(param)
	if err != nil {
		g.render(w, r, ErrBadRequest(errors.Wrap(err, "account ID must be presented as valid hex")))
		return
	}

	if len(slice) != common.SizeAccountID {
		g.render(w, r, ErrBadRequest(errors.Errorf("account ID must be %d bytes long", common.SizeAccountID)))
		return
	}

	var id common.AccountID
	copy(id[:], slice)

	g.render(w, r, &Account{ledger: g.ledger, id: id})
}

func (g *Gateway) contractScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		param := chi.URLParam(r, "id")

		slice, err := hex.DecodeString(param)
		if err != nil {
			g.render(w, r, ErrBadRequest(errors.Wrap(err, "contract ID must be presented as valid hex")))
			return
		}

		if len(slice) != common.SizeTransactionID {
			g.render(w, r, ErrBadRequest(errors.Errorf("contract ID must be %d bytes long", common.SizeTransactionID)))
			return
		}

		var contractID common.TransactionID
		copy(contractID[:], slice)

		ctx := context.WithValue(r.Context(), "contract_id", contractID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (g *Gateway) getContractCode(w http.ResponseWriter, r *http.Request) {
	id, ok := r.Context().Value("contract_id").(common.TransactionID)

	if !ok {
		return
	}

	code, available := wavelet.ReadAccountContractCode(g.ledger.Snapshot(), id)

	if len(code) == 0 || !available {
		g.render(w, r, ErrBadRequest(errors.Errorf("could not find contract with ID %x", id)))
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename="+hex.EncodeToString(id[:])+".wasm")
	w.Header().Set("Content-Type", "application/wasm")
	w.Header().Set("Content-Length", strconv.Itoa(hex.EncodedLen(len(code))))

	_, _ = io.Copy(w, strings.NewReader(hex.EncodeToString(code)))
}

func (g *Gateway) getContractPages(w http.ResponseWriter, r *http.Request) {
	id, ok := r.Context().Value("contract_id").(common.TransactionID)

	if !ok {
		return
	}

	var idx uint64
	var err error

	if raw := chi.URLParam(r, "index"); len(raw) != 0 {
		idx, err = strconv.ParseUint(raw, 10, 64)

		if err != nil {
			g.render(w, r, ErrBadRequest(errors.New("could not parse page index")))
			return
		}
	}

	snapshot := g.ledger.Snapshot()

	numPages, available := wavelet.ReadAccountContractNumPages(snapshot, id)

	if !available {
		g.render(w, r, ErrBadRequest(errors.Errorf("could not find any pages for contract with ID %x", id)))
		return
	}

	if idx >= numPages {
		g.render(w, r, ErrBadRequest(errors.Errorf("contract with ID %x only has %d pages, but you requested page %d", id, numPages, idx)))
		return
	}

	page, available := wavelet.ReadAccountContractPage(snapshot, id, idx)

	if len(page) == 0 || !available {
		g.render(w, r, ErrBadRequest(errors.Errorf("page %d is either empty, or does not exist", idx)))
		return
	}

	_, _ = w.Write(page)
}

func (g *Gateway) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(HeaderSessionToken)
		if len(token) == 0 {
			g.render(w, r, ErrBadRequest(errors.Errorf("session token not specified via HTTP header %q", HeaderSessionToken)))
			return
		}

		session, exists := g.registry.getSession(token)
		if !exists {
			g.render(w, r, ErrBadRequest(errors.Errorf("could not find session %s", token)))
			return
		}

		ctx := context.WithValue(r.Context(), KeySession, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (g *Gateway) securePoll(sink *sink) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get(KeyToken)

		if len(token) == 0 {
			g.render(w, r, ErrBadRequest(errors.New("specify a session token through url query params")))
			return
		}

		if _, exists := g.registry.getSession(token); !exists {
			g.render(w, r, ErrBadRequest(errors.Errorf("could not find session %s", token)))
			return
		}

		if err := sink.serve(w, r); err != nil {
			g.render(w, r, ErrBadRequest(errors.Wrap(err, "failed to init websocket session")))
		}
	}
}

func (g *Gateway) registerWebsocketSink(rawURL string) *sink {
	u, err := url.Parse(rawURL)

	if err != nil {
		panic(err)
	}

	// Map JSON log keys to HTTP query parameters.
	filters := make(map[string]string)
	values := u.Query()

	for key := range values {
		filters[key] = values.Get(key)
	}

	sink := &sink{
		filters:   filters,
		broadcast: make(chan broadcastItem),
		join:      make(chan *client),
		leave:     make(chan *client),
		clients:   make(map[*client]struct{}),
	}
	go sink.run()

	g.sinks[u.Hostname()] = sink

	return sink
}

func (g *Gateway) Write(buf []byte) (n int, err error) {
	var fields map[string]interface{}

	decoder := json.NewDecoder(bytes.NewReader(buf))
	decoder.UseNumber()

	err = decoder.Decode(&fields)
	if err != nil {
		return n, errors.Errorf("cannot decode field: %q", err)
	}

	mod, exists := fields[log.KeyModule]
	if !exists {
		return n, errors.Errorf("all logs must have the field %q", log.KeyModule)
	}

	sink, exists := g.sinks[mod.(string)]
	if !exists {
		return len(buf), nil
	}

	cpy := make([]byte, len(buf))
	copy(cpy, buf)

	sink.broadcast <- broadcastItem{fields: fields, buf: cpy}

	return len(buf), nil
}

// A helper to handle the error returned by render.Render()
func (g *Gateway) render(w http.ResponseWriter, r *http.Request, v render.Renderer) {
	err := render.Render(w, r, v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("render error:  " + err.Error()))
	}
}

// A helper to handle the error returned by render.RenderList()
func (g *Gateway) renderList(w http.ResponseWriter, r *http.Request, l []render.Renderer) {
	err := render.RenderList(w, r, l)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("renderList error: " + err.Error()))
	}
}
