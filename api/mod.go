package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	"sync"
	"sync/atomic"
	"time"
)

var (
	pollBufPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 100))
		},
	}
)

type Hub struct {
	node   *noise.Node
	ledger *wavelet.Ledger

	registry *sessionRegistry
	router   chi.Router

	nodePoller        atomic.Value
	accountsPoller    atomic.Value
	broadcasterPoller atomic.Value
	consensusPoller   atomic.Value
	contractPoller    atomic.Value
	stakePoller       atomic.Value
	txPoller          atomic.Value
}

func New() *Hub {
	return &Hub{registry: newSessionRegistry()}
}

func (h *Hub) Write(p []byte) (n int, err error) {
	var buf = pollBufPool.Get().(*bytes.Buffer)
	defer pollBufPool.Put(buf)

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
	case log.ModuleNode:
		if poller := h.nodePoller.Load(); poller != nil {
			if err := poller.(*melody.Melody).Broadcast(line); err != nil {
				return n, err
			}
		}
	case log.ModuleAccounts:
		accountID, exists := event["account_id"]
		if !exists {
			return n, errors.New("accounts log does not have field 'account_id'")
		}

		if poller := h.accountsPoller.Load(); poller != nil {
			err := poller.(*melody.Melody).BroadcastFilter(line, func(s *melody.Session) bool {
				if expectedID, ok := s.Get("id"); ok && accountID != expectedID {
					return false
				}

				return true
			})

			if err != nil {
				return n, err
			}
		}
	case log.ModuleBroadcaster:
		if poller := h.broadcasterPoller.Load(); poller != nil {
			if err := poller.(*melody.Melody).Broadcast(line); err != nil {
				return n, err
			}
		}
	case log.ModuleConsensus:
		if poller := h.consensusPoller.Load(); poller != nil {
			if err := poller.(*melody.Melody).Broadcast(line); err != nil {
				return n, err
			}
		}
	case log.ModuleContract:
		contractID, exists := event["contract_id"]
		if !exists {
			return n, errors.New("contract log does not have field 'contract_id'")
		}

		if poller := h.contractPoller.Load(); poller != nil {
			err := poller.(*melody.Melody).BroadcastFilter(line, func(s *melody.Session) bool {
				if expectedID, ok := s.Get("id"); ok && contractID != expectedID {
					return false
				}

				return true
			})

			if err != nil {
				return n, err
			}
		}
	case log.ModuleStake:
		if poller := h.stakePoller.Load(); poller != nil {
			if err := poller.(*melody.Melody).Broadcast(line); err != nil {
				return n, err
			}
		}
	case log.ModuleTx:
		txID, exists := event["tx_id"]
		if !exists {
			return n, errors.New("tx log does not have field 'tx_id'")
		}

		if poller := h.txPoller.Load(); poller != nil {
			err := poller.(*melody.Melody).BroadcastFilter(line, func(s *melody.Session) bool {
				if expectedID, ok := s.Get("id"); ok && txID != expectedID {
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

func (h *Hub) setupRouter() {
	// Setup websocket routers.
	h.nodePoller.Store(melody.New())
	h.broadcasterPoller.Store(melody.New())
	h.consensusPoller.Store(melody.New())
	h.stakePoller.Store(melody.New())

	accountsPoller := melody.New()
	accountsPoller.HandleConnect(h.parseWebsocketParams(sys.PublicKeySize))

	contractPoller := melody.New()
	contractPoller.HandleConnect(h.parseWebsocketParams(sys.TransactionIDSize))

	txPoller := melody.New()
	txPoller.HandleConnect(h.parseWebsocketParams(sys.TransactionIDSize))

	h.accountsPoller.Store(accountsPoller)
	h.contractPoller.Store(contractPoller)
	h.txPoller.Store(txPoller)

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

	r.Handle("/debug/vars", http.DefaultServeMux)

	r.Route("/session", func(r chi.Router) {
		r.Post("/init", h.initSession)
	})

	r.Route("/ledger", func(r chi.Router) {
		r.With(h.authenticated).Get("/", h.ledgerStatus)
	})

	r.Route("/broadcaster", func(r chi.Router) {
		r.With(h.authenticated).Get("/poll", h.poll(h.broadcasterPoller))
	})

	r.Route("/consensus", func(r chi.Router) {
		r.With(h.authenticated).Get("/poll", h.poll(h.consensusPoller))
	})

	r.Route("/stake", func(r chi.Router) {
		r.With(h.authenticated).Get("/poll", h.poll(h.stakePoller))
	})

	r.Route("/accounts", func(r chi.Router) {
		r.With(h.authenticated).Get("/{id}", h.getAccount)
		r.With(h.authenticated).Get("/poll", h.poll(h.accountsPoller))
	})

	r.Route("/contract/{id}", func(r chi.Router) {
		r.Use(h.contractScope)

		r.With(h.authenticated).Get("/", h.getContractCode)

		r.With(h.authenticated).Route("/page", func(r chi.Router) {
			r.Get("/", h.getContractPages)
			r.Get("/{index}", h.getContractPages)
		})

		r.With(h.authenticated).Get("/poll", h.poll(h.contractPoller))
	})

	r.Route("/tx", func(r chi.Router) {
		r.With(h.authenticated).Get("/", h.listTransactions)
		r.With(h.authenticated).Get("/{id}", h.getTransaction)
		r.With(h.authenticated).Post("/send", h.sendTransaction)

		r.With(h.authenticated).Get("/poll", h.poll(h.txPoller))
	})

	h.router = r
}

func (h *Hub) StartHTTP(n *noise.Node, port int) {
	h.ledger = node.Ledger(n)
	h.node = n

	h.setupRouter()

	log.Node().Info().Msgf("Started HTTP API server on port %d.", port)

	if err := http.ListenAndServe(":"+strconv.Itoa(port), h.router); err != nil {
		log.Node().Fatal().Err(err).Msg("Failed to start HTTP server.")
	}
}

func (h *Hub) initSession(w http.ResponseWriter, r *http.Request) {
	req := new(SessionInitRequest)

	if err := render.Bind(r, req); err != nil {
		h.render(w, r, ErrBadRequest(err))
		return
	}

	session, err := h.registry.newSession()
	if err != nil {
		h.render(w, r, ErrBadRequest(errors.Wrap(err, "failed to create session")))
	}

	h.render(w, r, &SessionInitResponse{Token: session.id})
}

func (h *Hub) sendTransaction(w http.ResponseWriter, r *http.Request) {
	req := new(SendTransactionRequest)

	if err := render.Bind(r, req); err != nil {
		h.render(w, r, ErrBadRequest(err))
		return
	}

	tx := &wavelet.Transaction{
		Creator:          req.creator,
		CreatorSignature: req.signature,

		Tag:     req.Tag,
		Payload: req.Payload,
	}

	if err := h.ledger.AttachSenderToTransaction(h.node.Keys, tx); err != nil {
		h.render(w, r, ErrInternal(errors.Wrap(err, "failed to attach sender to transaction")))
		return
	}

	if err := node.BroadcastTransaction(h.node, tx); err != nil {
		h.render(w, r, ErrInternal(errors.Wrap(err, "failed to broadcast transaction")))
		return
	}

	h.render(w, r, &SendTransactionResponse{ledger: h.ledger, tx: tx})
}

func (h *Hub) ledgerStatus(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, &LedgerStatusResponse{node: h.node, ledger: h.ledger})
}

func (h *Hub) listTransactions(w http.ResponseWriter, r *http.Request) {
	var sender [sys.PublicKeySize]byte
	var creator [sys.PublicKeySize]byte
	var offset, limit uint64
	var err error

	if raw := r.URL.Query().Get("sender"); len(raw) > 0 {
		slice, err := hex.DecodeString(raw)

		if err != nil {
			h.render(w, r, ErrBadRequest(errors.Wrap(err, "sender ID must be presented as valid hex")))
			return
		}

		if len(slice) != sys.PublicKeySize {
			h.render(w, r, ErrBadRequest(errors.Errorf("sender ID must be %d bytes long", sys.PublicKeySize)))
			return
		}

		if err != nil {
			h.render(w, r, ErrBadRequest(errors.Wrap(err, "could not parse sender")))
			return
		}

		copy(sender[:], slice)
	}

	if raw := r.URL.Query().Get("creator"); len(raw) > 0 {
		slice, err := hex.DecodeString(raw)

		if err != nil {
			h.render(w, r, ErrBadRequest(errors.Wrap(err, "creator ID must be presented as valid hex")))
			return
		}

		if len(slice) != sys.PublicKeySize {
			h.render(w, r, ErrBadRequest(errors.Errorf("creator ID must be %d bytes long", sys.PublicKeySize)))
			return
		}

		if err != nil {
			h.render(w, r, ErrBadRequest(errors.Wrap(err, "could not parse creator")))
			return
		}

		copy(creator[:], slice)
	}

	if raw := r.URL.Query().Get("offset"); len(raw) > 0 {
		offset, err = strconv.ParseUint(raw, 10, 64)

		if err != nil {
			h.render(w, r, ErrBadRequest(errors.Wrap(err, "could not parse offset")))
			return
		}
	}

	if raw := r.URL.Query().Get("limit"); len(raw) > 0 {
		limit, err = strconv.ParseUint(raw, 10, 64)

		if err != nil {
			h.render(w, r, ErrBadRequest(errors.Wrap(err, "could not parse limit")))
			return
		}
	}

	var transactions []render.Renderer

	for _, tx := range h.ledger.Transactions(offset, limit, sender, creator) {
		transactions = append(transactions, &Transaction{tx: tx})
	}

	h.renderList(w, r, transactions)
}

func (h *Hub) getTransaction(w http.ResponseWriter, r *http.Request) {
	param := chi.URLParam(r, "id")

	slice, err := hex.DecodeString(param)
	if err != nil {
		h.render(w, r, ErrBadRequest(errors.Wrap(err, "transaction ID must be presented as valid hex")))
		return
	}

	if len(slice) != sys.TransactionIDSize {
		h.render(w, r, ErrBadRequest(errors.Errorf("transaction ID must be %d bytes long", sys.TransactionIDSize)))
		return
	}

	var id [sys.TransactionIDSize]byte
	copy(id[:], slice)

	tx := h.ledger.FindTransaction(id)

	if tx == nil {
		h.render(w, r, ErrBadRequest(errors.Errorf("could not find transaction with ID %x", id)))
		return
	}

	h.render(w, r, &Transaction{tx: tx})
}

func (h *Hub) getAccount(w http.ResponseWriter, r *http.Request) {
	param := chi.URLParam(r, "id")

	slice, err := hex.DecodeString(param)
	if err != nil {
		h.render(w, r, ErrBadRequest(errors.Wrap(err, "account ID must be presented as valid hex")))
		return
	}

	if len(slice) != sys.PublicKeySize {
		h.render(w, r, ErrBadRequest(errors.Errorf("account ID must be %d bytes long", sys.PublicKeySize)))
		return
	}

	var id [sys.PublicKeySize]byte
	copy(id[:], slice)

	h.render(w, r, &Account{ledger: h.ledger, id: id})
}

func (h *Hub) contractScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		param := chi.URLParam(r, "id")

		slice, err := hex.DecodeString(param)
		if err != nil {
			h.render(w, r, ErrBadRequest(errors.Wrap(err, "contract ID must be presented as valid hex")))
			return
		}

		if len(slice) != sys.TransactionIDSize {
			h.render(w, r, ErrBadRequest(errors.Errorf("contract ID must be %d bytes long", sys.TransactionIDSize)))
			return
		}

		var contractID [sys.TransactionIDSize]byte
		copy(contractID[:], slice)

		ctx := context.WithValue(r.Context(), "contract_id", contractID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Hub) getContractCode(w http.ResponseWriter, r *http.Request) {
	id, ok := r.Context().Value("contract_id").([sys.TransactionIDSize]byte)

	if !ok {
		return
	}

	code, available := h.ledger.ReadAccountContractCode(id)

	if len(code) == 0 || !available {
		h.render(w, r, ErrBadRequest(errors.Errorf("could not find contract with ID %x", id)))
		return
	}

	_, _ = w.Write(code)
}

func (h *Hub) getContractPages(w http.ResponseWriter, r *http.Request) {
	id, ok := r.Context().Value("contract_id").([sys.TransactionIDSize]byte)

	if !ok {
		return
	}

	var idx uint64
	var err error

	if raw := chi.URLParam(r, "index"); len(raw) != 0 {
		idx, err = strconv.ParseUint(raw, 10, 64)

		if err != nil {
			h.render(w, r, ErrBadRequest(errors.New("could not parse page index")))
			return
		}
	}

	numPages, available := h.ledger.ReadAccountContractNumPages(id)

	if !available {
		h.render(w, r, ErrBadRequest(errors.Errorf("could not find any pages for contract with ID %x", id)))
		return
	}

	if idx >= numPages {
		h.render(w, r, ErrBadRequest(errors.Errorf("contract with ID %x only has %d pages, but you requested page %d", id, numPages, idx)))
		return
	}

	page, available := h.ledger.ReadAccountContractPage(id, idx)

	if len(page) == 0 || !available {
		h.render(w, r, ErrBadRequest(errors.Errorf("page %d is either empty, or does not exist", idx)))
		return
	}

	_, _ = w.Write(page)
}

func (h *Hub) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(HeaderSessionToken)

		if len(token) == 0 {
			h.render(w, r, ErrBadRequest(errors.Errorf("missing HTTP header %s", HeaderSessionToken)))
			return
		}

		session, exists := h.registry.getSession(token)
		if !exists {
			h.render(w, r, ErrBadRequest(errors.Errorf("could not find session %s", token)))
			return
		}

		ctx := context.WithValue(r.Context(), "session", session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Hub) pollNode(w http.ResponseWriter, r *http.Request) {
	poller := h.nodePoller.Load().(*melody.Melody)

	if err := poller.HandleRequest(w, r); err != nil {
		h.render(w, r, ErrInternal(err))
		return
	}
}

func (h *Hub) parseWebsocketParams(size int) func(*melody.Session) {
	return func(s *melody.Session) {
		param := s.Request.URL.Query().Get("id")

		if len(param) > 0 {
			slice, err := hex.DecodeString(param)
			if err != nil {
				_ = s.CloseWithMsg([]byte(errors.Wrap(err, "id must be presented as valid hex").Error()))
				return
			}

			if len(slice) != size {
				_ = s.CloseWithMsg([]byte(fmt.Sprintf("id must be %d bytes long", size)))
				return
			}

			s.Set("id", param)
		}
	}
}

func (h *Hub) poll(poller atomic.Value) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		poller := poller.Load().(*melody.Melody)

		if err := poller.HandleRequest(w, r); err != nil {
			h.render(w, r, ErrInternal(err))
			return
		}
	}
}

// A helper to handle the error returned by render.Render()
func (h *Hub) render(w http.ResponseWriter, r *http.Request, v render.Renderer) {
	err := render.Render(w, r, v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("render error:  " + err.Error()))
	}
}

// A helper to handle the error returned by render.RenderList()
func (h *Hub) renderList(w http.ResponseWriter, r *http.Request, l []render.Renderer) {
	err := render.RenderList(w, r, l)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("renderList error: " + err.Error()))
	}
}
