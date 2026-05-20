//go:build integration

package pgxrepo

import (
	"context"
	"testing"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/pkg/clock"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestQuotaCacheRepoWriteThenRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	withQuotaCacheLock(t, db, func() {
		now := time.Now().UTC().Add(time.Hour)
		repo := NewQuotaCacheRepo(db, clock.Fixed{T: now})
		snap := shared.QuotaSnapshot{
			VCPUs: shared.Capacity{Used: 10, Total: 100, Unit: "vcpus"},
			RAM:   shared.Capacity{Used: 2048, Total: 8192, Unit: "mb"},
			Disk:  shared.Capacity{Used: 20, Total: 200, Unit: "gb"},
		}

		if err := repo.Write(ctx, snap); err != nil {
			t.Fatalf("write quota cache: %v", err)
		}
		got, age, err := repo.Read(ctx)
		if err != nil {
			t.Fatalf("read quota cache: %v", err)
		}
		if got.VCPUs != snap.VCPUs || got.RAM != snap.RAM || got.Disk != snap.Disk {
			t.Fatalf("quota snapshot mismatch: got %+v want %+v", got, snap)
		}
		if age < 0 {
			t.Fatalf("expected non-negative age, got %s", age)
		}
	})
}

func TestQuotaCacheRepoReadEmptyIsStaleNotError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	withQuotaCacheLock(t, db, func() {
		if _, err := db.Exec(ctx, "DELETE FROM quota_cache"); err != nil {
			t.Fatalf("clear quota cache: %v", err)
		}
		repo := NewQuotaCacheRepo(db, clock.Fixed{T: time.Now().UTC()})

		_, age, err := repo.Read(ctx)
		if err != nil {
			t.Fatalf("read empty quota cache: %v", err)
		}
		if age < time.Duration(1<<61) {
			t.Fatalf("expected huge stale age, got %s", age)
		}
	})
}

func withQuotaCacheLock(t *testing.T, db *pgxpool.Pool, fn func()) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin quota cache lock tx: %v", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			t.Fatalf("rollback quota cache lock tx: %v", err)
		}
	}()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(42424242)"); err != nil {
		t.Fatalf("acquire quota cache lock: %v", err)
	}
	fn()
}
