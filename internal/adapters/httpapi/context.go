package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

var nilUUID uuid.UUID

type responseWriterKey struct{}

func contextWithResponseWriter(ctx context.Context, w http.ResponseWriter) context.Context {
	return context.WithValue(ctx, responseWriterKey{}, w)
}

func responseWriterFromContext(ctx context.Context) http.ResponseWriter {
	w, _ := ctx.Value(responseWriterKey{}).(http.ResponseWriter)
	return w
}

func requestID(ctx context.Context) string {
	return middleware.GetReqID(ctx)
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
