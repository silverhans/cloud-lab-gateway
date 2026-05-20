package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

var errInvalidRequest = errors.New("invalid request")

func (s *Server) renderProblem(w http.ResponseWriter, r *http.Request, err error) {
	status, title, detail, code := problemMeta(err)
	reqID := middleware.GetReqID(r.Context())
	if status == http.StatusInternalServerError {
		s.deps.log().Error("http handler failed", zap.String("request_id", reqID), zap.Error(err))
	}

	p := Problem{
		Type:     "about:blank",
		Title:    title,
		Status:   status,
		Detail:   &detail,
		Instance: &reqID,
		Code:     &code,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

func problemMeta(err error) (int, string, string, string) {
	code := shared.ErrorCode(err)
	switch {
	case errors.Is(err, errInvalidRequest), errors.Is(err, shared.ErrInvalidInput):
		return http.StatusBadRequest, "Bad Request", "request is invalid", "ERR_INVALID_INPUT"
	case errors.Is(err, shared.ErrUnauthorized):
		return http.StatusUnauthorized, "Unauthorized", "authentication is required", code
	case errors.Is(err, shared.ErrForbidden):
		return http.StatusForbidden, "Forbidden", "access denied", code
	case errors.Is(err, shared.ErrNotFound):
		return http.StatusNotFound, "Not Found", "resource not found", code
	case errors.Is(err, shared.ErrLabAlreadyActive),
		errors.Is(err, shared.ErrPoolEmpty),
		errors.Is(err, shared.ErrQuotaExceeded),
		errors.Is(err, shared.ErrIdempotencyClash),
		isInvalidTransition(err):
		return http.StatusConflict, "Conflict", "request conflicts with current state", code
	case errors.Is(err, shared.ErrCloudUnavailable), errors.Is(err, shared.ErrLMSUnavailable):
		return http.StatusServiceUnavailable, "Service Unavailable", "dependency is temporarily unavailable", code
	case errors.Is(err, errNotImplemented):
		return http.StatusNotImplemented, "Not Implemented", "operation is not implemented yet", "ERR_NOT_IMPLEMENTED"
	default:
		return http.StatusInternalServerError, "Internal Server Error", "internal server error", "ERR_INTERNAL"
	}
}

func isInvalidTransition(err error) bool {
	var target shared.ErrInvalidTransition
	return errors.As(err, &target)
}
