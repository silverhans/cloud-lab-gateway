package main

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/cloud-lab-gateway/gateway/cmd/moodle-emulator/templates"
	"github.com/cloud-lab-gateway/gateway/internal/lti"
)

func TestLTILaunchSignVerifyRoundTrip(t *testing.T) {
	t.Parallel()

	signer := testSigner(t)
	launch, err := signer.issueLaunch(time.Now().UTC(), "https://moodle-emulator.local", templates.Users[0], templates.Courses[0], templates.Courses[0].Labs[0])
	if err != nil {
		t.Fatalf("issue launch: %v", err)
	}
	if launch.IDToken == "" || launch.State == "" || launch.Nonce == "" {
		t.Fatal("launch payload has empty fields")
	}

	publicKey := publicKeyFromJWKS(t, signer)
	claims := &lti.LaunchClaims{}
	token, err := jwt.ParseWithClaims(launch.IDToken, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodRS256 {
			t.Fatalf("method = %v, want RS256", token.Method.Alg())
		}
		if token.Header["kid"] != signer.kid {
			t.Fatalf("kid = %v, want %s", token.Header["kid"], signer.kid)
		}
		return publicKey, nil
	}, jwt.WithAudience(clientID), jwt.WithIssuer("https://moodle-emulator.local"))
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if !token.Valid {
		t.Fatal("token is invalid")
	}
	if err := claims.ValidateResourceLink(); err != nil {
		t.Fatalf("validate claims: %v", err)
	}
	if claims.Context.ID != "linux-101" {
		t.Fatalf("context id = %q", claims.Context.ID)
	}
	if claims.Custom.LabTemplateSlug != "linux-basics-1" {
		t.Fatalf("lab slug = %q", claims.Custom.LabTemplateSlug)
	}
}

func testSigner(t *testing.T) *signer {
	t.Helper()

	signer, err := loadSigner(filepath.Join(t.TempDir(), "key.pem"), "test-kid")
	if err != nil {
		t.Fatalf("load signer: %v", err)
	}
	return signer
}

func publicKeyFromJWKS(t *testing.T, signer *signer) *rsa.PublicKey {
	t.Helper()

	raw, err := signer.jwksJSON()
	if err != nil {
		t.Fatalf("jwks json: %v", err)
	}
	var response jwksResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("unmarshal jwks: %v", err)
	}
	if len(response.Keys) != 1 {
		t.Fatalf("jwks keys len = %d, want 1", len(response.Keys))
	}
	n, err := base64.RawURLEncoding.DecodeString(response.Keys[0].N)
	if err != nil {
		t.Fatalf("decode n: %v", err)
	}
	e, err := base64.RawURLEncoding.DecodeString(response.Keys[0].E)
	if err != nil {
		t.Fatalf("decode e: %v", err)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(n),
		E: int(new(big.Int).SetBytes(e).Int64()),
	}
}
