package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
)

const defaultPrivateKeyPath = "/tmp/moodle-emulator-key.pem"

type signer struct {
	kid string
	key *rsa.PrivateKey
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

func loadSigner(path, kid string) (*signer, error) {
	if kid == "" {
		return nil, errors.New("moodle-emulator: kid is required")
	}
	if path == "" {
		path = defaultPrivateKeyPath
	}
	key, err := readPrivateKey(path)
	if err == nil {
		return &signer{kid: kid, key: key}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("moodle-emulator: generate rsa key: %w", err)
	}
	if err := writePrivateKey(path, key); err != nil {
		return nil, err
	}
	return &signer{kid: kid, key: key}, nil
}

func readPrivateKey(path string) (*rsa.PrivateKey, error) {
	// #nosec G304 -- path is an operator-controlled env/config value, not user input.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("moodle-emulator: private key pem is invalid")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("moodle-emulator: parse private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("moodle-emulator: private key is not RSA")
	}
	return key, nil
}

func writePrivateKey(path string, key *rsa.PrivateKey) error {
	raw, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("moodle-emulator: marshal private key: %w", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: raw}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return fmt.Errorf("moodle-emulator: write private key: %w", err)
	}
	return nil
}

func (s *signer) jwksJSON() ([]byte, error) {
	return json.Marshal(jwksResponse{Keys: []jwk{s.publicJWK()}})
}

func (s *signer) publicJWK() jwk {
	pub := s.key.PublicKey
	return jwk{
		Kty: "RSA",
		Use: "sig",
		Kid: s.kid,
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}
