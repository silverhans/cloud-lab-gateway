package envkek

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

const (
	keySize        = 32
	gcmNonceSize   = 12
	currentVersion = 1
	envKEKBase64   = "CLG_KEK_BASE64"
)

// KeyProvider wraps DEKs with a KEK loaded from the environment.
type KeyProvider struct {
	kek []byte
}

var _ ports.KeyProvider = (*KeyProvider)(nil)

// NewKeyProvider reads CLG_KEK_BASE64 and constructs a KeyProvider.
func NewKeyProvider() (*KeyProvider, error) {
	raw := os.Getenv(envKEKBase64)
	if raw == "" {
		return nil, fmt.Errorf("envkek: %s is required", envKEKBase64)
	}
	kek, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("envkek: invalid KEK encoding")
	}
	return NewKeyProviderFromKey(kek)
}

// NewKeyProviderFromKey constructs a KeyProvider from a raw 32-byte KEK.
func NewKeyProviderFromKey(kek []byte) (*KeyProvider, error) {
	if len(kek) != keySize {
		return nil, fmt.Errorf("envkek: KEK must be 32 bytes")
	}
	cp := append([]byte(nil), kek...)
	return &KeyProvider{kek: cp}, nil
}

// EncryptDEK wraps dek with the current KEK version.
func (p *KeyProvider) EncryptDEK(_ context.Context, dek []byte, aad string) ([]byte, []byte, int, error) {
	if len(dek) != keySize {
		return nil, nil, 0, fmt.Errorf("envkek: DEK must be 32 bytes")
	}
	gcm, err := p.gcm()
	if err != nil {
		return nil, nil, 0, err
	}
	nonce, err := randomBytes(gcmNonceSize)
	if err != nil {
		return nil, nil, 0, err
	}
	ciphertext := gcm.Seal(nil, nonce, dek, []byte(aad))
	return ciphertext, nonce, currentVersion, nil
}

// DecryptDEK unwraps a DEK encrypted by EncryptDEK.
func (p *KeyProvider) DecryptDEK(_ context.Context, ciphertext, nonce []byte, version int, aad string) ([]byte, error) {
	if version != currentVersion {
		return nil, fmt.Errorf("envkek: unsupported KEK version")
	}
	gcm, err := p.gcm()
	if err != nil {
		return nil, err
	}
	dek, err := gcm.Open(nil, nonce, ciphertext, []byte(aad))
	if err != nil {
		return nil, fmt.Errorf("envkek: decrypt DEK failed")
	}
	return dek, nil
}

// CurrentVersion returns the KEK version used for new records.
func (p *KeyProvider) CurrentVersion(context.Context) (int, error) {
	return currentVersion, nil
}

func (p *KeyProvider) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(p.kek)
	if err != nil {
		return nil, fmt.Errorf("envkek: initialize KEK cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("envkek: initialize KEK GCM")
	}
	return gcm, nil
}

func randomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return nil, fmt.Errorf("envkek: random bytes")
	}
	return buf, nil
}

func zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
