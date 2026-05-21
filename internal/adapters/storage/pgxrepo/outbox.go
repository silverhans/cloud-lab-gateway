package pgxrepo

import (
	"context"
	"fmt"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/sqlcgen"
	"github.com/cloud-lab-gateway/gateway/internal/app/outbox"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxRepo exposes unpublished outbox rows to the publisher.
type OutboxRepo struct {
	q *sqlcgen.Queries
}

var _ outbox.Repo = (*OutboxRepo)(nil)

func NewOutboxRepo(db *pgxpool.Pool) *OutboxRepo {
	return &OutboxRepo{q: sqlcgen.New(db)}
}

func (r *OutboxRepo) Fetch(ctx context.Context, limit int, maxAttempts int) ([]outbox.Row, error) {
	limit32, err := listLimitInt32(limit)
	if err != nil {
		return nil, err
	}
	maxAttempts32, err := listLimitInt32(maxAttempts)
	if err != nil {
		return nil, err
	}
	if limit32 == 0 || maxAttempts32 == 0 {
		return []outbox.Row{}, nil
	}
	rows, err := r.q.FetchOutbox(ctx, sqlcgen.FetchOutboxParams{
		Limit:       limit32,
		MaxAttempts: maxAttempts32,
	})
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: fetch outbox: %w", err)
	}
	out := make([]outbox.Row, 0, len(rows))
	for _, row := range rows {
		out = append(out, outbox.Row{
			ID:         row.ID,
			Topic:      row.Topic,
			Payload:    append([]byte(nil), row.Payload...),
			OccurredAt: timeFromTimestamptz(row.OccurredAt),
			Attempts:   int(row.Attempts),
		})
	}
	return out, nil
}

func (r *OutboxRepo) MarkPublished(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	if err := r.q.MarkOutboxPublished(ctx, ids); err != nil {
		return fmt.Errorf("pgxrepo: mark outbox published: %w", err)
	}
	return nil
}

func (r *OutboxRepo) BumpAttempts(ctx context.Context, id int64) error {
	if id <= 0 {
		return shared.ErrInvalidInput
	}
	if err := r.q.BumpOutboxAttempts(ctx, id); err != nil {
		return fmt.Errorf("pgxrepo: bump outbox attempts: %w", err)
	}
	return nil
}
