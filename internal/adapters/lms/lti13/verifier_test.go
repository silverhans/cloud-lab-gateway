package lti13

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/lti"
)

const (
	testIssuer   = "https://moodle-emulator.local"
	testClientID = "cloud-lab-gateway"
	testKID      = "test-key-1"
)

func TestVerifierVerifyLaunchAndRejectReplay(t *testing.T) {
	t.Parallel()
	key := generateRSAKey(t)
	jwks := newJWKSServer(t, key, testKID, http.StatusOK)
	verifier := New(Config{Issuer: testIssuer, ClientID: testClientID, JWKSURL: jwks.URL})
	token := signLaunch(t, key, testKID, "nonce-1")

	launch, err := verifier.VerifyLaunch(context.Background(), token, "opaque-state", "nonce-1")
	if err != nil {
		t.Fatalf("VerifyLaunch: %v", err)
	}
	if launch.Iss != testIssuer || launch.Sub != "user-student-001" || launch.CourseExternalID != "linux-101" {
		t.Fatalf("unexpected launch: %+v", launch)
	}
	if launch.Raw["context_title"] != "Linux basics" || launch.Raw["lab_template_slug"] != "linux-users" {
		t.Fatalf("unexpected raw claims: %+v", launch.Raw)
	}

	_, err = verifier.VerifyLaunch(context.Background(), token, "opaque-state", "nonce-1")
	if !errors.Is(err, shared.ErrUnauthorized) {
		t.Fatalf("replay err = %v, want unauthorized", err)
	}
}

func TestVerifierRejectsNonceMismatch(t *testing.T) {
	t.Parallel()
	key := generateRSAKey(t)
	jwks := newJWKSServer(t, key, testKID, http.StatusOK)
	verifier := New(Config{Issuer: testIssuer, ClientID: testClientID, JWKSURL: jwks.URL})
	token := signLaunch(t, key, testKID, "nonce-2")

	_, err := verifier.VerifyLaunch(context.Background(), token, "", "other-nonce")
	if !errors.Is(err, shared.ErrUnauthorized) {
		t.Fatalf("err = %v, want unauthorized", err)
	}
}

func TestVerifierMapsJWKSFailureToLMSUnavailable(t *testing.T) {
	t.Parallel()
	key := generateRSAKey(t)
	jwks := newJWKSServer(t, key, testKID, http.StatusInternalServerError)
	verifier := New(Config{Issuer: testIssuer, ClientID: testClientID, JWKSURL: jwks.URL})
	token := signLaunch(t, key, testKID, "nonce-3")

	_, err := verifier.VerifyLaunch(context.Background(), token, "", "nonce-3")
	if !errors.Is(err, shared.ErrLMSUnavailable) {
		t.Fatalf("err = %v, want LMS unavailable", err)
	}
}

func generateRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func newJWKSServer(t *testing.T, key *rsa.PrivateKey, kid string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwksResponse{Keys: []jwk{publicJWK(key, kid)}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func publicJWK(key *rsa.PrivateKey, kid string) jwk {
	pub := key.PublicKey
	return jwk{
		Kty: "RSA",
		Use: "sig",
		Kid: kid,
		Alg: jwt.SigningMethodRS256.Alg(),
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

func signLaunch(t *testing.T, key *rsa.PrivateKey, kid, nonce string) string {
	t.Helper()
	now := time.Now().UTC()
	claims := lti.LaunchClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			Subject:   "user-student-001",
			Audience:  jwt.ClaimStrings{testClientID},
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		AuthorizedParty: testClientID,
		Nonce:           nonce,
		DeploymentID:    "deployment-1",
		MessageType:     lti.MessageTypeResourceLink,
		Version:         lti.Version13,
		Roles:           []string{lti.RoleLearner},
		Context: lti.Context{
			ID:    "linux-101",
			Label: "linux",
			Title: "Linux basics",
		},
		ResourceLink: lti.Resource{
			ID:    "linux-users",
			Title: "Users and permissions",
		},
		Custom: lti.Custom{
			LabTemplateSlug: "linux-users",
		},
		Email: "student-001@emulator.local",
		Name:  "Student One",
	}
	if err := claims.ValidateResourceLink(); err != nil {
		t.Fatalf("claims: %v", err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}
