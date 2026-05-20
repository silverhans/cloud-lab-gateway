//go:build integration

package envkek

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/audit"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeAuditRepo struct {
	events []audit.AuditEvent
	err    error
}

var _ ports.AuditRepo = (*fakeAuditRepo)(nil)

func (r *fakeAuditRepo) Append(_ context.Context, ev audit.AuditEvent) error {
	r.events = append(r.events, ev)
	return r.err
}

func (r *fakeAuditRepo) AppendInTx(context.Context, ports.Tx, audit.AuditEvent) error {
	return nil
}

func (r *fakeAuditRepo) Query(context.Context, ports.AuditFilter) ([]audit.AuditEvent, error) {
	return nil, nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestStorePutGetRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	auditRepo := &fakeAuditRepo{}
	store := newTestStore(t, db, auditRepo)
	payload := []byte("private key payload")

	id, err := store.Put(ctx, "ssh_private_key", "lab-1", payload)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get(ctx, id, "ssh_private_key", "lab-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %q want %q", got, payload)
	}
	if len(auditRepo.events) != 1 || auditRepo.events[0].Kind != audit.KindSecretAccessed {
		t.Fatalf("expected secret access audit event, got %+v", auditRepo.events)
	}
}

func TestStoreGetWrongExpectedKind(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	store := newTestStore(t, db, &fakeAuditRepo{})

	id, err := store.Put(ctx, "ssh_private_key", "lab-1", []byte("payload"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	_, err = store.Get(ctx, id, "moodle_token", "lab-1")
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestStoreGetMissing(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, connectTestDB(t), &fakeAuditRepo{})

	_, err := store.Get(context.Background(), shared.SecretID(uuid.New()), "kind", "ref")
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreDeleteIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	store := newTestStore(t, db, &fakeAuditRepo{})

	id, err := store.Put(ctx, "ssh_private_key", "lab-1", []byte("payload"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = store.Get(ctx, id, "ssh_private_key", "lab-1")
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	if err := store.Delete(ctx, id); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
}

func TestStoreTamperedPayloadReturnsSecretMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	store := newTestStore(t, db, &fakeAuditRepo{})

	id, err := store.Put(ctx, "ssh_private_key", "lab-1", []byte("payload"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE encrypted_secrets
		SET payload_ciphertext = set_byte(payload_ciphertext, 0, get_byte(payload_ciphertext, 0) # 1)
		WHERE id = $1
	`, uuid.UUID(id)); err != nil {
		t.Fatalf("tamper payload: %v", err)
	}

	_, err = store.Get(ctx, id, "ssh_private_key", "lab-1")
	if !errors.Is(err, shared.ErrSecretMismatch) {
		t.Fatalf("expected ErrSecretMismatch, got %v", err)
	}
}

func TestStoreFreshDEKPerPut(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	store := newTestStore(t, db, &fakeAuditRepo{})
	payload := []byte("same payload")

	first, err := store.Put(ctx, "ssh_private_key", "lab-1", payload)
	if err != nil {
		t.Fatalf("Put first: %v", err)
	}
	second, err := store.Put(ctx, "ssh_private_key", "lab-2", payload)
	if err != nil {
		t.Fatalf("Put second: %v", err)
	}

	var firstDEK, firstPayload, secondDEK, secondPayload []byte
	if err := db.QueryRow(ctx, "SELECT dek_ciphertext, payload_ciphertext FROM encrypted_secrets WHERE id = $1", uuid.UUID(first)).Scan(&firstDEK, &firstPayload); err != nil {
		t.Fatalf("query first ciphertexts: %v", err)
	}
	if err := db.QueryRow(ctx, "SELECT dek_ciphertext, payload_ciphertext FROM encrypted_secrets WHERE id = $1", uuid.UUID(second)).Scan(&secondDEK, &secondPayload); err != nil {
		t.Fatalf("query second ciphertexts: %v", err)
	}
	if bytes.Equal(firstDEK, secondDEK) {
		t.Fatal("expected different DEK ciphertexts")
	}
	if bytes.Equal(firstPayload, secondPayload) {
		t.Fatal("expected different payload ciphertexts")
	}
}

func connectTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN is not set")
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(db.Close)
	if err := db.Ping(context.Background()); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	return db
}

func newTestStore(t *testing.T, db *pgxpool.Pool, auditRepo ports.AuditRepo) *Store {
	t.Helper()
	kp, err := NewKeyProviderFromKey(bytes.Repeat([]byte{3}, keySize))
	if err != nil {
		t.Fatalf("NewKeyProviderFromKey: %v", err)
	}
	return NewStore(db, kp, auditRepo, fixedClock{now: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)})
}
