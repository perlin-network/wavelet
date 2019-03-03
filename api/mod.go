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
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"gopkg.in/olahol/melody.v1"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

type Gateway struct {
	node   *noise.Node
	ledger *wavelet.Ledger

	registry *sessionRegistry
	router   chi.Router

	accountsPoller    atomic.Value
	broadcasterPoller atomic.Value
	consensusPoller   atomic.Value
	contractPoller    atomic.Value
	stakePoller       atomic.Value
	txPoller          atomic.Value
}

func New() *Gateway {
	return &Gateway{registry: newSessionRegistry()}
}

func (g *Gateway) Write(p []byte) (n int, err error) {
	var event map[string]interface{}

	decoder := json.NewDecoder(bytes.NewReader(p))
	decoder.UseNumber()

	err = decoder.Decode(&event)
	if err != nil {
		return n, errors.Errorf("cannot decode event: %s", err)
	}

	mod, exists := event["mod"]
	if !exists {
		return n, errors.New("mod does not exist")
	}

	line := make([]byte, len(p))
	copy(line, p)

	switch mod {
	case log.ModuleAccounts:
		accountID, exists := event["account_id"]
		if !exists {
			return n, errors.New("accounts log does not have field 'account_id'")
		}

		if poller := g.accountsPoller.Load(); poller != nil {
			err := poller.(*melody.Melody).BroadcastFilter(line, func(s *melody.Session) bool {
				if expectedID, ok := s.Get("account_id"); ok && accountID != expectedID {
					return false
				}

				return true
			})

			if err != nil {
				return n, err
			}
		}
	case log.ModuleBroadcaster:
		if poller := g.broadcasterPoller.Load(); poller != nil {
			if err := poller.(*melody.Melody).Broadcast(line); err != nil {
				return n, err
			}
		}
	case log.ModuleConsensus:
		if poller := g.consensusPoller.Load(); poller != nil {
			if err := poller.(*melody.Melody).Broadcast(line); err != nil {
				return n, err
			}
		}
	case log.ModuleContract:
		contractID, exists := event["contract_id"]
		if !exists {
			return n, errors.New("contract log does not have field 'contract_id'")
		}

		if poller := g.contractPoller.Load(); poller != nil {
			err := poller.(*melody.Melody).BroadcastFilter(line, func(s *melody.Session) bool {
				if expectedID, ok := s.Get("contract_id"); ok && contractID != expectedID {
					return false
				}

				return true
			})

			if err != nil {
				return n, err
			}
		}
	case log.ModuleStake:
		if poller := g.stakePoller.Load(); poller != nil {
			if err := poller.(*melody.Melody).Broadcast(line); err != nil {
				return n, err
			}
		}
	case log.ModuleTx:
		txID, exists := event["tx_id"]
		if !exists {
			return n, errors.New("tx log does not have field 'tx_id'")
		}

		senderID, exists := event["sender_id"]
		if !exists {
			return n, errors.New("tx log does not have field 'sender_id'")
		}

		creatorID, exists := event["creator_id"]
		if !exists {
			return n, errors.New("tx log does not have field 'creator_id'")
		}

		if poller := g.txPoller.Load(); poller != nil {
			err := poller.(*melody.Melody).BroadcastFilter(line, func(s *melody.Session) bool {
				if expectedID, ok := s.Get("tx_id"); ok && txID != expectedID {
					return false
				}

				if expectedID, ok := s.Get("sender_id"); ok && senderID != expectedID {
					return false
				}

				if expectedID, ok := s.Get("creator_id"); ok && creatorID != expectedID {
					return false
				}

				return true
			})

			if err != nil {
				return n, err
			}
		}
	}

	return len(p), nil
}

func (g *Gateway) setupWebsocketPoller(poller *atomic.Value, params map[string]string) {
	hub := melody.New()
	hub.HandleConnect(g.parseWebsocketParams(params))
	poller.Store(hub)
}

func (g *Gateway) setupRouter() {
	// Setup websocket routers.

	g.setupWebsocketPoller(&g.broadcasterPoller, nil)
	g.setupWebsocketPoller(&g.consensusPoller, nil)
	g.setupWebsocketPoller(&g.stakePoller, nil)

	g.setupWebsocketPoller(&g.accountsPoller, map[string]string{
		"id": "account_id",
	})

	g.setupWebsocketPoller(&g.contractPoller, map[string]string{
		"id": "contract_id",
	})

	g.setupWebsocketPoller(&g.txPoller, map[string]string{
		"id":      "tx_id",
		"sender":  "sender_id",
		"creator": "creator_id",
	})

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

	r.Get("/broadcaster/poll", g.poll(g.broadcasterPoller))
	r.Get("/consensus/poll", g.poll(g.consensusPoller))
	r.Get("/stake/poll", g.poll(g.stakePoller))
	r.Get("/accounts/poll", g.poll(g.accountsPoller))
	r.Get("/contract/poll", g.poll(g.contractPoller))
	r.Get("/tx/poll", g.poll(g.txPoller))

	// HTTP endpoints.

	r.Post("/session/init", g.initSession)
	r.Handle("/debug/vars", http.DefaultServeMux)

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

func (g *Gateway) StartHTTP(n *noise.Node, port int) {
	log.Register(g)

	g.ledger = node.Ledger(n)
	g.node = n

	g.setupRouter()

	logger := log.Node()
	logger.Info().Msgf("Started HTTP API server on port %d.", port)

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
	}

	g.render(w, r, &SessionInitResponse{Token: session.id})
}

func (g *Gateway) sendTransaction(w http.ResponseWriter, r *http.Request) {
	req := new(SendTransactionRequest)

	if err := render.Bind(r, req); err != nil {
		g.render(w, r, ErrBadRequest(err))
		return
	}

	tx := &wavelet.Transaction{
		Creator:          req.creator,
		CreatorSignature: req.signature,

		Tag:     req.Tag,
		Payload: req.payload,
	}

	if err := g.ledger.AttachSenderToTransaction(g.node.Keys, tx); err != nil {
		g.render(w, r, ErrInternal(errors.Wrap(err, "failed to attach sender to transaction")))
		return
	}

	if err := node.BroadcastTransaction(g.node, tx); err != nil {
		g.render(w, r, ErrInternal(errors.Wrap(err, "failed to broadcast transaction")))
		return
	}

	g.render(w, r, &SendTransactionResponse{ledger: g.ledger, tx: tx})
}

func (g *Gateway) ledgerStatus(w http.ResponseWriter, r *http.Request) {
	g.render(w, r, &LedgerStatusResponse{node: g.node, ledger: g.ledger})
}

func (g *Gateway) listTransactions(w http.ResponseWriter, r *http.Request) {
	var sender [sys.PublicKeySize]byte
	var creator [sys.PublicKeySize]byte
	var offset, limit uint64
	var err error

	if raw := r.URL.Query().Get("sender"); len(raw) > 0 {
		slice, err := hex.DecodeString(raw)

		if err != nil {
			g.render(w, r, ErrBadRequest(errors.Wrap(err, "sender ID must be presented as valid hex")))
			return
		}

		if len(slice) != sys.PublicKeySize {
			g.render(w, r, ErrBadRequest(errors.Errorf("sender ID must be %d bytes long", sys.PublicKeySize)))
			return
		}

		if err != nil {
			g.render(w, r, ErrBadRequest(errors.Wrap(err, "could not parse sender")))
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

		if len(slice) != sys.PublicKeySize {
			g.render(w, r, ErrBadRequest(errors.Errorf("creator ID must be %d bytes long", sys.PublicKeySize)))
			return
		}

		if err != nil {
			g.render(w, r, ErrBadRequest(errors.Wrap(err, "could not parse creator")))
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

	for _, tx := range g.ledger.Transactions(offset, limit, sender, creator) {
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

	if len(slice) != sys.TransactionIDSize {
		g.render(w, r, ErrBadRequest(errors.Errorf("transaction ID must be %d bytes long", sys.TransactionIDSize)))
		return
	}

	var id [sys.TransactionIDSize]byte
	copy(id[:], slice)

	tx := g.ledger.FindTransaction(id)

	if tx == nil {
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

	if len(slice) != sys.PublicKeySize {
		g.render(w, r, ErrBadRequest(errors.Errorf("account ID must be %d bytes long", sys.PublicKeySize)))
		return
	}

	var id [sys.PublicKeySize]byte
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

		if len(slice) != sys.TransactionIDSize {
			g.render(w, r, ErrBadRequest(errors.Errorf("contract ID must be %d bytes long", sys.TransactionIDSize)))
			return
		}

		var contractID [sys.TransactionIDSize]byte
		copy(contractID[:], slice)

		ctx := context.WithValue(r.Context(), "contract_id", contractID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (g *Gateway) getContractCode(w http.ResponseWriter, r *http.Request) {
	id, ok := r.Context().Value("contract_id").([sys.TransactionIDSize]byte)

	if !ok {
		return
	}

	code, available := g.ledger.ReadAccountContractCode(id)

	if len(code) == 0 || !available {
		g.render(w, r, ErrBadRequest(errors.Errorf("could not find contract with ID %x", id)))
		return
	}

	_, _ = w.Write(code)
}

func (g *Gateway) getContractPages(w http.ResponseWriter, r *http.Request) {
	id, ok := r.Context().Value("contract_id").([sys.TransactionIDSize]byte)

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

	numPages, available := g.ledger.ReadAccountContractNumPages(id)

	if !available {
		g.render(w, r, ErrBadRequest(errors.Errorf("could not find any pages for contract with ID %x", id)))
		return
	}

	if idx >= numPages {
		g.render(w, r, ErrBadRequest(errors.Errorf("contract with ID %x only has %d pages, but you requested page %d", id, numPages, idx)))
		return
	}

	page, available := g.ledger.ReadAccountContractPage(id, idx)

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

		ctx := context.WithValue(r.Context(), "session", session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (g *Gateway) parseWebsocketParams(params map[string]string) func(s *melody.Session) {
	return func(s *melody.Session) {
		for query, key := range params {
			if condition := s.Request.URL.Query().Get(query); len(condition) > 0 {
				s.Set(key, condition)
			}
		}
	}
}

func (g *Gateway) poll(poller atomic.Value) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if len(token) == 0 {
			g.render(w, r, ErrBadRequest(errors.New("specify a session token through url query params")))
			return
		}

		_, exists := g.registry.getSession(token)
		if !exists {
			g.render(w, r, ErrBadRequest(errors.Errorf("could not find session %s", token)))
			return
		}

		poller := poller.Load().(*melody.Melody)

		if err := poller.HandleRequest(w, r); err != nil {
			g.render(w, r, ErrInternal(err))
			return
		}
	}
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
