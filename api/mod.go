package api

import (
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/wavelet/log"
	"net/http"
	"strconv"
	"time"
)

func StartHTTP(node *noise.Node, port int) {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hi"))
	})

	log.Info().Msgf("Started HTTP API server on port %d.", port)

	http.ListenAndServe(":"+strconv.Itoa(port), r)
}
