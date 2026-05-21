package sse

import (
	"errors"
	"net/http"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/httpapi"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type HandlerDeps struct {
	Broker        *Broker
	SessionSecret string
	Logger        *zap.Logger
}

func NewMux(deps HandlerDeps) http.Handler {
	r := chi.NewRouter()
	h := handler{deps: deps}
	r.Get("/labs", h.labs)
	return r
}

type handler struct {
	deps HandlerDeps
}

func (h handler) labs(w http.ResponseWriter, r *http.Request) {
	if h.deps.Broker == nil {
		http.Error(w, "sse broker unavailable", http.StatusServiceUnavailable)
		return
	}
	principal, err := httpapi.ParsePrincipal(r, h.deps.SessionSecret)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	err = h.deps.Broker.Subscribe(r.Context(), []string{"user:" + principal.UserID.String(), audienceAll}, w)
	if err != nil && !errors.Is(err, r.Context().Err()) {
		h.log().Debug("sse client disconnected", zap.Error(err))
	}
}

func (h handler) log() *zap.Logger {
	if h.deps.Logger == nil {
		return zap.NewNop()
	}
	return h.deps.Logger
}
