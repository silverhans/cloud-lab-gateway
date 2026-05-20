package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewMux builds the /api/v1 REST mux from the OpenAPI-generated strict server.
func NewMux(deps Deps) http.Handler {
	r := chi.NewRouter()
	s := NewServer(deps)
	r.Use(middleware.RequestID)
	r.Use(s.RequireAuth)

	strict := NewStrictHandlerWithOptions(s, []StrictMiddlewareFunc{withResponseWriter}, StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			s.renderProblem(w, r, errInvalidRequest)
		},
		ResponseErrorHandlerFunc: s.renderProblem,
	})
	return HandlerFromMux(strict, r)
}

func withResponseWriter(next StrictHandlerFunc, _ string) StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request interface{}) (interface{}, error) {
		return next(contextWithResponseWriter(ctx, w), w, r, request)
	}
}
