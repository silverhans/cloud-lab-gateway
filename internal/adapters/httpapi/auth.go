package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

const sessionCookieName = "clg_session"

type principalKey struct{}

// Principal is the authenticated browser subject carried in request context.
type Principal struct {
	UserID      uuid.UUID
	DisplayName string
	Email       string
	Role        string
	CourseRoles map[uuid.UUID]string
}

type sessionClaims struct {
	DisplayName string            `json:"display_name,omitempty"`
	Email       string            `json:"email,omitempty"`
	Role        string            `json:"role"`
	CourseRoles map[string]string `json:"course_roles,omitempty"`
	jwt.RegisteredClaims
}

func principalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

func contextWithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// IssueSession mints a signed session JWT and sets the clg_session cookie.
func IssueSession(w http.ResponseWriter, p Principal, ttl time.Duration) error {
	secret := os.Getenv("CLG_JWT_SECRET")
	if secret == "" {
		return shared.ErrInvalidInput
	}
	return issueSession(w, p, ttl, secret, false)
}

func issueSession(w http.ResponseWriter, p Principal, ttl time.Duration, secret string, devMode bool) error {
	if secret == "" || p.UserID == uuid.Nil || p.Role == "" {
		return shared.ErrInvalidInput
	}
	now := time.Now().UTC()
	claims := sessionClaims{
		DisplayName: p.DisplayName,
		Email:       p.Email,
		Role:        p.Role,
		CourseRoles: encodeCourseRoles(p.CourseRoles),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   p.UserID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return err
	}
	// #nosec G124 -- dev mode intentionally allows local HTTP; production cookies are Secure.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !devMode,
		Expires:  claims.ExpiresAt.Time,
	})
	return nil
}

func (s *Server) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAuthExempt(r) {
			next.ServeHTTP(w, r)
			return
		}
		p, err := s.parsePrincipal(r)
		if err != nil {
			s.renderProblem(w, r, shared.ErrUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(contextWithPrincipal(r.Context(), p)))
	})
}

func (s *Server) parsePrincipal(r *http.Request) (Principal, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return Principal{}, shared.ErrUnauthorized
	}
	secret := s.deps.SessionSecret
	if secret == "" {
		secret = os.Getenv("CLG_JWT_SECRET")
	}
	if secret == "" {
		return Principal{}, shared.ErrUnauthorized
	}
	var claims sessionClaims
	token, err := jwt.ParseWithClaims(cookie.Value, &claims, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		return Principal{}, shared.ErrUnauthorized
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return Principal{}, shared.ErrUnauthorized
	}
	if claims.Role == "" {
		return Principal{}, shared.ErrUnauthorized
	}
	return Principal{
		UserID:      id,
		DisplayName: claims.DisplayName,
		Email:       claims.Email,
		Role:        claims.Role,
		CourseRoles: decodeCourseRoles(claims.CourseRoles),
	}, nil
}

func clearSession(w http.ResponseWriter, devMode bool) {
	// #nosec G124 -- dev mode intentionally allows local HTTP; production cookies are Secure.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !devMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
	})
}

func isAuthExempt(r *http.Request) bool {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	return (r.Method == http.MethodPost && path == "/auth/login") ||
		(r.Method == http.MethodGet && (path == "/healthz" || path == "/readyz"))
}

func encodeCourseRoles(in map[uuid.UUID]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for id, role := range in {
		out[id.String()] = role
	}
	return out
}

func decodeCourseRoles(in map[string]string) map[uuid.UUID]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[uuid.UUID]string, len(in))
	for raw, role := range in {
		id, err := uuid.Parse(raw)
		if err == nil {
			out[id] = role
		}
	}
	return out
}
