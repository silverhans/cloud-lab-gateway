package ports

import (
	"context"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

// KeyProvider is the envelope-encryption KEK provider. Implementations:
//   - envkek:        KEK from CLG_KEK_BASE64 env (MVP)
//   - vaulttransit:  HashiCorp Vault Transit (prod, out of scope for hackathon)
//   - awskms:        AWS KMS Encrypt/Decrypt (alt for prod)
type KeyProvider interface {
	// EncryptDEK wraps a freshly generated DEK with the current KEK version.
	EncryptDEK(ctx context.Context, dek []byte, aad string) (ciphertext, nonce []byte, version int, err error)

	// DecryptDEK unwraps the DEK using the KEK of the recorded version.
	DecryptDEK(ctx context.Context, ciphertext, nonce []byte, version int, aad string) ([]byte, error)

	// CurrentVersion returns the KEK version used for new records.
	CurrentVersion(ctx context.Context) (int, error)
}

// SecretStore persists envelope-encrypted secrets and serves decrypted values
// to the application layer on demand. Each decryption emits a secret.accessed
// audit event.
type SecretStore interface {
	// Put encrypts payload with a fresh DEK and stores the record. Returns
	// the new SecretID. The caller MUST zeroize payload after this call.
	Put(ctx context.Context, kind string, refID string, payload []byte) (shared.SecretID, error)

	// Get decrypts and returns the payload. Caller MUST zeroize after use.
	Get(ctx context.Context, id shared.SecretID, expectedKind, expectedRefID string) ([]byte, error)

	// Delete removes the secret (e.g. on lab cleanup).
	Delete(ctx context.Context, id shared.SecretID) error
}
