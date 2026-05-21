// Package lti13 verifies Moodle LTI 1.3 Resource Link Launch tokens.
package lti13

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/lti"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

const (
	defaultHTTPTimeout = 5 * time.Second
	defaultNonceTTL    = 10 * time.Minute
	maxJWKSBytes       = 1 << 20
)

// Config contains operator-controlled LTI platform settings.
type Config struct {
	Issuer     string
	ClientID   string
	JWKSURL    string
	HTTPClient *http.Client
	Redis      *redis.Client
	NonceTTL   time.Duration
	Now        func() time.Time
}

// Verifier verifies signed LTI launch id_tokens.
type Verifier struct {
	issuer    string
	clientID  string
	jwksURL   string
	client    *http.Client
	redis     *redis.Client
	nonceTTL  time.Duration
	now       func() time.Time
	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	seenNonce map[string]time.Time
}

var _ ports.LMSProvider = (*Verifier)(nil)

// New creates an LTI 1.3 verifier. Empty required settings fail at verify time
// so the gateway can still boot while Moodle is not enabled in a demo profile.
func New(cfg Config) *Verifier {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	nonceTTL := cfg.NonceTTL
	if nonceTTL <= 0 {
		nonceTTL = defaultNonceTTL
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Verifier{
		issuer:    cfg.Issuer,
		clientID:  cfg.ClientID,
		jwksURL:   cfg.JWKSURL,
		client:    client,
		redis:     cfg.Redis,
		nonceTTL:  nonceTTL,
		now:       now,
		keys:      make(map[string]*rsa.PublicKey),
		seenNonce: make(map[string]time.Time),
	}
}

// VerifyLaunch validates the signed LTI launch token and maps it to the port DTO.
func (v *Verifier) VerifyLaunch(ctx context.Context, idToken, state, formNonce string) (*ports.LTILaunch, error) {
	if v == nil || idToken == "" || v.issuer == "" || v.clientID == "" || v.jwksURL == "" {
		return nil, shared.ErrInvalidInput
	}

	claims := &lti.LaunchClaims{}
	token, err := jwt.ParseWithClaims(idToken, claims, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodRS256 {
			return nil, errors.New("lti13: unexpected signing method")
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("lti13: token kid is required")
		}
		return v.publicKey(ctx, kid)
	}, jwt.WithAudience(v.clientID), jwt.WithIssuer(v.issuer), jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}))
	if err != nil {
		if errors.Is(err, shared.ErrLMSUnavailable) {
			return nil, err
		}
		return nil, fmt.Errorf("lti13: verify token: %w", shared.ErrUnauthorized)
	}
	if token == nil || !token.Valid {
		return nil, shared.ErrUnauthorized
	}
	if err := claims.ValidateResourceLink(); err != nil {
		return nil, fmt.Errorf("lti13: validate claims: %w", shared.ErrUnauthorized)
	}
	if claims.AuthorizedParty != "" && claims.AuthorizedParty != v.clientID {
		return nil, shared.ErrUnauthorized
	}
	if formNonce != "" && formNonce != claims.Nonce {
		return nil, shared.ErrUnauthorized
	}
	if err := v.claimNonce(ctx, claims.Issuer, claims.Nonce); err != nil {
		return nil, err
	}

	aud := make([]string, len(claims.Audience))
	copy(aud, claims.Audience)
	return &ports.LTILaunch{
		Iss:              claims.Issuer,
		Sub:              claims.Subject,
		Aud:              aud,
		Email:            claims.Email,
		Name:             claims.Name,
		CourseExternalID: claims.Context.ID,
		ResourceLinkID:   claims.ResourceLink.ID,
		RolesInContext:   append([]string(nil), claims.Roles...),
		Raw: map[string]any{
			"context_label":     claims.Context.Label,
			"context_title":     claims.Context.Title,
			"deployment_id":     claims.DeploymentID,
			"lab_template_slug": claims.Custom.LabTemplateSlug,
			"resource_title":    claims.ResourceLink.Title,
			"state":             state,
		},
	}, nil
}

// ReportGrade is reserved for AGS integration after the launch flow is live.
func (v *Verifier) ReportGrade(context.Context, *ports.LTILaunch, float64, float64, string) error {
	return fmt.Errorf("lti13: AGS is not wired yet: %w", shared.ErrLMSUnavailable)
}

// GetCourseMembers is reserved for NRPS integration after the launch flow is live.
func (v *Verifier) GetCourseMembers(context.Context, string) ([]ports.LMSMember, error) {
	return nil, fmt.Errorf("lti13: NRPS is not wired yet: %w", shared.ErrLMSUnavailable)
}

func (v *Verifier) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	key := v.keys[kid]
	v.mu.Unlock()
	if key != nil {
		return key, nil
	}
	keys, err := v.fetchJWKS(ctx)
	if err != nil {
		return nil, err
	}
	v.mu.Lock()
	for id, parsed := range keys {
		v.keys[id] = parsed
	}
	key = v.keys[kid]
	v.mu.Unlock()
	if key == nil {
		return nil, errors.New("lti13: jwks kid not found")
	}
	return key, nil
}

func (v *Verifier) fetchJWKS(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	// #nosec G107 -- JWKS URL is operator-controlled LTI platform configuration.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("lti13: build jwks request: %w", shared.ErrLMSUnavailable)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lti13: fetch jwks: %w", shared.ErrLMSUnavailable)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("lti13: jwks returned %d: %w", resp.StatusCode, shared.ErrLMSUnavailable)
	}
	var payload jwksResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJWKSBytes)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("lti13: decode jwks: %w", shared.ErrLMSUnavailable)
	}
	keys := make(map[string]*rsa.PublicKey, len(payload.Keys))
	for _, raw := range payload.Keys {
		key, err := raw.publicKey()
		if err != nil {
			return nil, fmt.Errorf("lti13: parse jwk: %w", shared.ErrLMSUnavailable)
		}
		keys[raw.Kid] = key
	}
	return keys, nil
}

func (v *Verifier) claimNonce(ctx context.Context, issuer, nonce string) error {
	key := nonceKey(issuer, nonce)
	if v.redis != nil {
		ok, err := v.redis.SetNX(ctx, key, "1", v.nonceTTL).Result()
		if err != nil {
			return fmt.Errorf("lti13: store nonce: %w", shared.ErrLMSUnavailable)
		}
		if !ok {
			return shared.ErrUnauthorized
		}
		return nil
	}

	now := v.now()
	v.mu.Lock()
	defer v.mu.Unlock()
	for seen, expiresAt := range v.seenNonce {
		if !now.Before(expiresAt) {
			delete(v.seenNonce, seen)
		}
	}
	if expiresAt, ok := v.seenNonce[key]; ok && now.Before(expiresAt) {
		return shared.ErrUnauthorized
	}
	v.seenNonce[key] = now.Add(v.nonceTTL)
	return nil
}

func nonceKey(issuer, nonce string) string {
	sum := sha256.Sum256([]byte(issuer + "\x00" + nonce))
	return "lti:nonce:" + hex.EncodeToString(sum[:])
}

type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (j jwk) publicKey() (*rsa.PublicKey, error) {
	if j.Kid == "" || j.Kty != "RSA" || j.N == "" || j.E == "" {
		return nil, errors.New("invalid rsa jwk")
	}
	if j.Use != "" && j.Use != "sig" {
		return nil, errors.New("unsupported jwk use")
	}
	if j.Alg != "" && j.Alg != jwt.SigningMethodRS256.Alg() {
		return nil, errors.New("unsupported jwk alg")
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(j.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(j.E)
	if err != nil {
		return nil, err
	}
	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() || e.Int64() <= 1 {
		return nil, errors.New("invalid jwk exponent")
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(e.Int64()),
	}, nil
}
