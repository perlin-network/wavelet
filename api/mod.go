package api

import (
	"context"
	"encoding/hex"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/render"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/pkg/errors"
	"net/http"
	"strconv"
	"time"
)

type hub struct {
	node   *noise.Node
	ledger *wavelet.Ledger

	registry *sessionRegistry
	router   chi.Router
}

func newHub(n *noise.Node) *hub {
	h := &hub{node: n, ledger: node.Ledger(n), registry: newSessionRegistry()}

	h.setRoutes()

	return h
}

func (h *hub) setRoutes() {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	//r.Use(middleware.Logger)
	cors := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	})
	r.Use(cors.Handler)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Handle("/debug/vars", http.DefaultServeMux)

	r.Route("/session", func(r chi.Router) {
		r.Post("/init", h.initSession)
	})

	r.Route("/accounts", func(r chi.Router) {
		r.With(h.authenticated).Get("/{id}", h.getAccount)
	})

	r.Route("/tx", func(r chi.Router) {
		r.With(h.authenticated).Get("/", h.listTransactions)
		r.With(h.authenticated).Get("/{id}", h.getTransaction)
		r.With(h.authenticated).Post("/send", h.sendTransaction)
	})

	r.Route("/contract/{id}", func(r chi.Router) {
		r.Use(h.contractScope)

		r.With(h.authenticated).Get("/", h.getContractCode)

		r.With(h.authenticated).Route("/page", func(r chi.Router) {
			r.Get("/", h.getContractPages)
			r.Get("/{index}", h.getContractPages)
		})

	})

	r.Route("/ledger", func(r chi.Router) {
		r.Get("/", h.ledgerStatus)
	})

	h.router = r
}

func StartHTTP(n *noise.Node, port int) {
	h := newHub(n)

	log.Info().Msgf("Started HTTP API server on port %d.", port)

	if err := http.ListenAndServe(":"+strconv.Itoa(port), h.router); err != nil {
		log.Fatal().Err(err).Msg("failed to start http server")
	}
}

func (h *hub) initSession(w http.ResponseWriter, r *http.Request) {
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

func (h *hub) sendTransaction(w http.ResponseWriter, r *http.Request) {
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

func (h *hub) ledgerStatus(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, &LedgerStatusResponse{node: h.node, ledger: h.ledger})
}

func (h *hub) listTransactions(w http.ResponseWriter, r *http.Request) {
	var offset, limit uint64
	var err error

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

	for _, tx := range h.ledger.Transactions(offset, limit) {
		transactions = append(transactions, &Transaction{tx: tx})
	}

	h.renderList(w, r, transactions)
}

func (h *hub) getTransaction(w http.ResponseWriter, r *http.Request) {
	param := chi.URLParam(r, "id")

	slice, err := hex.DecodeString(param)
	if err != nil {
		h.render(w, r, ErrBadRequest(errors.Wrap(err, "transaction ID must be presented as valid hex")))
		return
	}

	if len(slice) != wavelet.TransactionIDSize {
		h.render(w, r, ErrBadRequest(errors.Errorf("transaction ID must be %d bytes long", wavelet.TransactionIDSize)))
		return
	}

	var id [wavelet.TransactionIDSize]byte
	copy(id[:], slice)

	tx := h.ledger.FindTransaction(id)

	if tx == nil {
		h.render(w, r, ErrBadRequest(errors.Errorf("could not find transaction with ID %x", id)))
		return
	}

	h.render(w, r, &Transaction{tx: tx})
}

func (h *hub) getAccount(w http.ResponseWriter, r *http.Request) {
	param := chi.URLParam(r, "id")

	slice, err := hex.DecodeString(param)
	if err != nil {
		h.render(w, r, ErrBadRequest(errors.Wrap(err, "account ID must be presented as valid hex")))
		return
	}

	if len(slice) != wavelet.PublicKeySize {
		h.render(w, r, ErrBadRequest(errors.Errorf("account ID must be %d bytes long", wavelet.PublicKeySize)))
		return
	}

	var id [wavelet.PublicKeySize]byte
	copy(id[:], slice)

	h.render(w, r, &Account{ledger: h.ledger, id: id})
}

func (h *hub) contractScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		param := chi.URLParam(r, "id")

		slice, err := hex.DecodeString(param)
		if err != nil {
			h.render(w, r, ErrBadRequest(errors.Wrap(err, "contract ID must be presented as valid hex")))
			return
		}

		if len(slice) != wavelet.TransactionIDSize {
			h.render(w, r, ErrBadRequest(errors.Errorf("contract ID must be %d bytes long", wavelet.TransactionIDSize)))
			return
		}

		var contractID [wavelet.TransactionIDSize]byte
		copy(contractID[:], slice)

		ctx := context.WithValue(r.Context(), "contract_id", contractID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *hub) getContractCode(w http.ResponseWriter, r *http.Request) {
	id, ok := r.Context().Value("contract_id").([wavelet.TransactionIDSize]byte)

	if !ok {
		return
	}

	code, available := h.ledger.ReadAccountContractCode(id)

	if len(code) == 0 || !available {
		h.render(w, r, ErrBadRequest(errors.Errorf("could not find contract with ID %x", id)))
		return
	}

	w.Write(code)
}

func (h *hub) getContractPages(w http.ResponseWriter, r *http.Request) {
	id, ok := r.Context().Value("contract_id").([wavelet.TransactionIDSize]byte)

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

func (h *hub) authenticated(next http.Handler) http.Handler {
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

// A helper to handle the error returned by render.Render()
func (h *hub) render(w http.ResponseWriter, r *http.Request, v render.Renderer) {
	err := render.Render(w, r, v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("render error:  " + err.Error()))
	}
}

// A helper to handle the error returned by render.RenderList()
func (h *hub) renderList(w http.ResponseWriter, r *http.Request, l []render.Renderer) {
	err := render.RenderList(w, r, l)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("renderList error: " + err.Error()))
	}
}
