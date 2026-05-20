package envkek

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/sqlcgen"
	"github.com/cloud-lab-gateway/gateway/internal/domain/audit"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Store persists envelope-encrypted secrets in Postgres.
type Store struct {
	q     *sqlcgen.Queries
	kp    ports.KeyProvider
	audit ports.AuditRepo
	clk   ports.Clock
	log   *zap.Logger
}

var _ ports.SecretStore = (*Store)(nil)

// NewStore creates a Postgres-backed SecretStore.
func NewStore(db *pgxpool.Pool, kp ports.KeyProvider, auditRepo ports.AuditRepo, clk ports.Clock) *Store {
	return &Store{
		q:     sqlcgen.New(db),
		kp:    kp,
		audit: auditRepo,
		clk:   clk,
		log:   zap.NewNop(),
	}
}

// Put encrypts payload with a fresh DEK and stores it.
func (s *Store) Put(ctx context.Context, kind string, refID string, payload []byte) (shared.SecretID, error) {
	if s == nil || s.kp == nil || kind == "" || refID == "" {
		return shared.SecretID{}, shared.ErrInvalidInput
	}
	dek, err := randomDEK()
	if err != nil {
		return shared.SecretID{}, err
	}
	defer zeroize(dek)

	aad := secretAAD(kind, refID)
	payloadCiphertext, payloadNonce, err := encryptWithKey(dek, payload, []byte(aad))
	if err != nil {
		return shared.SecretID{}, err
	}

	dekCiphertext, dekNonce, version, err := s.kp.EncryptDEK(ctx, dek, aad)
	if err != nil {
		return shared.SecretID{}, fmt.Errorf("envkek: wrap DEK: %w", err)
	}
	kekVersion, err := kekVersionInt32(version)
	if err != nil {
		return shared.SecretID{}, err
	}

	id := shared.NewSecretID()
	row, err := s.q.InsertEncryptedSecret(ctx, sqlcgen.InsertEncryptedSecretParams{
		ID:                uuid.UUID(id),
		Kind:              kind,
		RefID:             refID,
		DekCiphertext:     dekCiphertext,
		DekNonce:          dekNonce,
		PayloadCiphertext: payloadCiphertext,
		PayloadNonce:      payloadNonce,
		Aad:               aad,
		KekVersion:        kekVersion,
		CreatedAt:         nullableTimeArg(s.now()),
	})
	if err != nil {
		return shared.SecretID{}, fmt.Errorf("envkek: insert secret: %w", err)
	}
	return shared.SecretID(row.ID), nil
}

// Get decrypts a stored secret after verifying its expected identity.
func (s *Store) Get(ctx context.Context, id shared.SecretID, expectedKind, expectedRefID string) ([]byte, error) {
	if s == nil || s.kp == nil {
		return nil, shared.ErrInvalidInput
	}
	row, err := s.q.GetEncryptedSecretByID(ctx, uuid.UUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("envkek: get secret: %w", err)
	}
	if row.Kind != expectedKind || row.RefID != expectedRefID {
		return nil, shared.ErrForbidden
	}

	dek, err := s.kp.DecryptDEK(ctx, row.DekCiphertext, row.DekNonce, int(row.KekVersion), row.Aad)
	if err != nil {
		return nil, fmt.Errorf("envkek: decrypt secret: %w", shared.ErrSecretMismatch)
	}
	defer zeroize(dek)

	plaintext, err := decryptWithKey(dek, row.PayloadCiphertext, row.PayloadNonce, []byte(row.Aad))
	if err != nil {
		return nil, fmt.Errorf("envkek: decrypt secret: %w", shared.ErrSecretMismatch)
	}
	s.appendAccessAudit(ctx, id)
	return plaintext, nil
}

// Delete removes a secret. Missing rows are treated as success.
func (s *Store) Delete(ctx context.Context, id shared.SecretID) error {
	if s == nil {
		return shared.ErrInvalidInput
	}
	if err := s.q.DeleteEncryptedSecret(ctx, uuid.UUID(id)); err != nil {
		return fmt.Errorf("envkek: delete secret: %w", err)
	}
	return nil
}

func (s *Store) appendAccessAudit(ctx context.Context, id shared.SecretID) {
	if s.audit == nil {
		return
	}
	subjectID := id.String()
	if err := s.audit.Append(ctx, audit.AuditEvent{
		ID:          shared.NewAuditEventID(),
		Kind:        audit.KindSecretAccessed,
		SubjectType: "encrypted_secret",
		SubjectID:   &subjectID,
		Payload:     map[string]interface{}{},
		OccurredAt:  s.now(),
	}); err != nil && s.log != nil {
		s.log.Warn("secret access audit append failed", zap.Error(err))
	}
}

func (s *Store) now() time.Time {
	if s.clk == nil {
		return time.Now().UTC()
	}
	return s.clk.Now()
}

func randomDEK() ([]byte, error) {
	buf := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return nil, fmt.Errorf("envkek: generate DEK")
	}
	return buf, nil
}

func kekVersionInt32(version int) (int32, error) {
	if version <= 0 || version > math.MaxInt32 {
		return 0, shared.ErrInvalidInput
	}
	// #nosec G115 -- bounds are checked above before narrowing for the DB column.
	return int32(version), nil
}

func encryptWithKey(key, plaintext, aad []byte) ([]byte, []byte, error) {
	gcm, err := gcmForKey(key)
	if err != nil {
		return nil, nil, err
	}
	nonce, err := randomBytes(gcmNonceSize)
	if err != nil {
		return nil, nil, err
	}
	return gcm.Seal(nil, nonce, plaintext, aad), nonce, nil
}

func decryptWithKey(key, ciphertext, nonce, aad []byte) ([]byte, error) {
	gcm, err := gcmForKey(key)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

func gcmForKey(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("envkek: initialize cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("envkek: initialize GCM")
	}
	return gcm, nil
}

func secretAAD(kind, refID string) string {
	return kind + ":" + refID
}

func nullableTimeArg(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}
