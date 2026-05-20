package pgxrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/sqlcgen"
	applab "github.com/cloud-lab-gateway/gateway/internal/app/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// QuotaCacheRepo persists the single-row quota snapshot cache in Postgres.
type QuotaCacheRepo struct {
	q   *sqlcgen.Queries
	clk ports.Clock
}

var _ applab.QuotaCache = (*QuotaCacheRepo)(nil)

// NewQuotaCacheRepo creates a QuotaCacheRepo backed by a pgx pool.
func NewQuotaCacheRepo(db *pgxpool.Pool, clk ports.Clock) *QuotaCacheRepo {
	return &QuotaCacheRepo{q: sqlcgen.New(db), clk: clk}
}

// Read returns a quota snapshot and its cache age. A cold cache is stale, not an error.
func (r *QuotaCacheRepo) Read(ctx context.Context) (shared.QuotaSnapshot, time.Duration, error) {
	row, err := r.q.GetQuotaCache(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return shared.QuotaSnapshot{}, time.Duration(1 << 62), nil
	}
	if err != nil {
		return shared.QuotaSnapshot{}, 0, fmt.Errorf("pgxrepo: read quota cache: %w", err)
	}

	var snap shared.QuotaSnapshot
	if err := json.Unmarshal(row.Snapshot, &snap); err != nil {
		return shared.QuotaSnapshot{}, 0, fmt.Errorf("pgxrepo: unmarshal quota cache: %w", err)
	}
	fetchedAt := timeFromTimestamptz(row.FetchedAt)
	return snap, r.now().Sub(fetchedAt), nil
}

// Write stores the latest quota snapshot.
func (r *QuotaCacheRepo) Write(ctx context.Context, snap shared.QuotaSnapshot) error {
	payload, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("pgxrepo: marshal quota cache: %w", err)
	}
	if err := r.q.UpsertQuotaCache(ctx, payload); err != nil {
		return fmt.Errorf("pgxrepo: write quota cache: %w", err)
	}
	return nil
}

func (r *QuotaCacheRepo) now() time.Time {
	if r.clk == nil {
		return time.Now().UTC()
	}
	return r.clk.Now()
}
