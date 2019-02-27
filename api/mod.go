package api

import (
	"context"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/net"
	"github.com/pkg/errors"
	"net/http"
	"strconv"
	"time"
)

type hub struct {
	node   *noise.Node
	ledger *wavelet.Ledger

	registry *sessionRegistry
}

func StartHTTP(node *noise.Node, port int) {
	h := &hub{node: node, ledger: net.Ledger(node), registry: newSessionRegistry()}

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Use(middleware.Timeout(60 * time.Second))

	r.Handle("/debug/vars", http.DefaultServeMux)

	r.Route("/session", func(r chi.Router) {
		r.Post("/init", h.initSession)
	})

	r.Route("/transaction", func(r chi.Router) {
		r.With(h.authenticated).Post("/send", h.sendTransaction)
	})

	log.Info().Msgf("Started HTTP API server on port %d.", port)

	http.ListenAndServe(":"+strconv.Itoa(port), r)
}

func (h *hub) initSession(w http.ResponseWriter, r *http.Request) {
	req := new(SessionInitRequest)

	if err := render.Bind(r, req); err != nil {
		render.Render(w, r, ErrBadRequest(err))
		return
	}

	session, err := h.registry.newSession()
	if err != nil {
		render.Render(w, r, ErrBadRequest(errors.Wrap(err, "failed to create session")))
	}

	render.Render(w, r, &SessionInitResponse{Token: session.id})
}

func (h *hub) sendTransaction(w http.ResponseWriter, r *http.Request) {
	req := new(SendTransactionRequest)

	if err := render.Bind(r, req); err != nil {
		render.Render(w, r, ErrBadRequest(err))
		return
	}

	tx := &wavelet.Transaction{
		Creator:          req.creator,
		CreatorSignature: req.signature,

		Tag:     req.Tag,
		Payload: req.Payload,
	}

	if err := h.ledger.AttachSenderToTransaction(h.node.Keys, tx); err != nil {
		render.Render(w, r, ErrInternal(errors.Wrap(err, "failed to attach sender to transaction")))
		return
	}

	if err := net.BroadcastTransaction(h.node, tx); err != nil {
		render.Render(w, r, ErrInternal(errors.Wrap(err, "failed to broadcast transaction")))
		return
	}

	render.Render(w, r, &SendTransactionResponse{ledger: h.ledger, tx: tx})
}

func (h *hub) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(HeaderSessionToken)

		if len(token) == 0 {
			render.Render(w, r, ErrBadRequest(errors.Errorf("missing HTTP header %q", HeaderSessionToken)))
			return
		}

		session, exists := h.registry.getSession(token)
		if !exists {
			render.Render(w, r, ErrBadRequest(errors.Errorf("could not find session %q", token)))
			return
		}

		ctx := context.WithValue(r.Context(), "session", session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
