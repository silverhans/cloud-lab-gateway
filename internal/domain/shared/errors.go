package shared

import (
	"errors"
	"fmt"
)

// Sentinel errors. Application layer maps these to HTTP status codes
// in adapters/httpapi/errors.go. Stable error codes are used for UI.
var (
	ErrNotFound         = errors.New("not found")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("forbidden")
	ErrPoolEmpty        = errors.New("pool: no free projects available")
	ErrQuotaExceeded    = errors.New("quota: cluster utilization exceeds threshold")
	ErrLabAlreadyActive = errors.New("lab: student already has an active lab in this course")
	ErrCloudUnavailable = errors.New("cloud provider unavailable")
	ErrLMSUnavailable   = errors.New("LMS unavailable")
	ErrInvalidInput     = errors.New("invalid input")
	ErrSecretMismatch   = errors.New("secret AAD mismatch")
	ErrIdempotencyClash = errors.New("idempotency clash: conflicting request with same key")
)

// ErrInvalidTransition is returned by state machines when a requested
// transition is not allowed.
type ErrInvalidTransition struct {
	Entity string
	From   string
	To     string
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("%s: invalid transition %s → %s", e.Entity, e.From, e.To)
}

// ErrorCode returns a stable string identifier for an error suitable for UI
// matching and audit log indexing. Unknown errors return "ERR_INTERNAL".
func ErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNotFound):
		return "ERR_NOT_FOUND"
	case errors.Is(err, ErrUnauthorized):
		return "ERR_UNAUTHORIZED"
	case errors.Is(err, ErrForbidden):
		return "ERR_FORBIDDEN"
	case errors.Is(err, ErrPoolEmpty):
		return "ERR_POOL_EMPTY"
	case errors.Is(err, ErrQuotaExceeded):
		return "ERR_QUOTA_EXCEEDED"
	case errors.Is(err, ErrLabAlreadyActive):
		return "ERR_LAB_ALREADY_ACTIVE"
	case errors.Is(err, ErrCloudUnavailable):
		return "ERR_CLOUD_UNAVAILABLE"
	case errors.Is(err, ErrLMSUnavailable):
		return "ERR_LMS_UNAVAILABLE"
	case errors.Is(err, ErrInvalidInput):
		return "ERR_INVALID_INPUT"
	case errors.Is(err, ErrIdempotencyClash):
		return "ERR_IDEMPOTENCY"
	}
	var it ErrInvalidTransition
	if errors.As(err, &it) {
		return "ERR_INVALID_TRANSITION"
	}
	return "ERR_INTERNAL"
}
