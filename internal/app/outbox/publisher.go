// Package outbox implements the publisher half of the transactional outbox
// pattern.
//
// Domain mutations and event publishing happen in two stages:
//
//  1. The aggregate's repository, inside a DB transaction, INSERTs into the
//     `outbox` table together with the state change (atomic).
//  2. This publisher polls `outbox` for unpublished rows and forwards each to
//     the EventBus (Redis pub/sub in production).
//
// Two stages are required because a single transaction cannot atomically
// commit to Postgres AND publish to Redis — if Redis is down or the process
// dies between the two, events would be lost. The outbox row is the durable
// queue that bridges the two systems with at-least-once semantics.
package outbox

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// Row is one persisted outbox entry as seen by the publisher.
type Row struct {
	ID         int64
	Topic      string
	Payload    []byte
	OccurredAt time.Time
	Attempts   int
}

// Repo is the publisher's narrow view of the outbox table. Implemented by
// internal/adapters/storage/pgxrepo/outbox.go in production.
type Repo interface {
	// Fetch returns up to `limit` unpublished rows in (id ASC) order. Rows
	// with attempts >= maxAttempts MUST be excluded from the result so the
	// publisher does not loop on a poison message.
	Fetch(ctx context.Context, limit int, maxAttempts int) ([]Row, error)

	// MarkPublished sets published_at = now() for the given ids.
	MarkPublished(ctx context.Context, ids []int64) error

	// BumpAttempts increments the attempts counter for a failed publish.
	BumpAttempts(ctx context.Context, id int64) error
}

// Publisher drives the outbox → bus forwarding loop.
type Publisher struct {
	Repo         Repo
	Bus          ports.EventBus
	Logger       *zap.Logger
	PollInterval time.Duration // default 1s
	BatchSize    int           // default 100
	MaxAttempts  int           // default 10
}

// New returns a Publisher with sensible defaults filled in.
func New(repo Repo, bus ports.EventBus, log *zap.Logger) *Publisher {
	return &Publisher{
		Repo:         repo,
		Bus:          bus,
		Logger:       log,
		PollInterval: 1 * time.Second,
		BatchSize:    100,
		MaxAttempts:  10,
	}
}

// Run polls and publishes until ctx is cancelled. Returns ctx.Err on cancel.
func (p *Publisher) Run(ctx context.Context) error {
	if p.PollInterval <= 0 {
		p.PollInterval = 1 * time.Second
	}
	if p.BatchSize <= 0 {
		p.BatchSize = 100
	}
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 10
	}
	ticker := time.NewTicker(p.PollInterval)
	defer ticker.Stop()

	// Tick once immediately on startup so any rows accumulated during the
	// previous downtime do not sit idle for PollInterval.
	if err := p.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
		p.log().Warn("outbox initial tick failed", zap.Error(err))
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := p.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
				p.log().Warn("outbox tick failed", zap.Error(err))
			}
		}
	}
}

// Tick performs one polling iteration. Exposed for tests and for one-shot CLI
// invocations ("flush the outbox now").
func (p *Publisher) Tick(ctx context.Context) error {
	rows, err := p.Repo.Fetch(ctx, p.BatchSize, p.MaxAttempts)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	published := make([]int64, 0, len(rows))
	for _, r := range rows {
		if err := ctx.Err(); err != nil {
			break
		}
		if err := p.Bus.Publish(ctx, r.Topic, r.Payload); err != nil {
			p.log().Warn("publish failed; bumping attempts",
				zap.Int64("outbox_id", r.ID),
				zap.String("topic", r.Topic),
				zap.Int("attempts", r.Attempts+1),
				zap.Error(err),
			)
			if bumpErr := p.Repo.BumpAttempts(ctx, r.ID); bumpErr != nil {
				p.log().Error("bump attempts failed",
					zap.Int64("outbox_id", r.ID),
					zap.Error(bumpErr),
				)
			}
			continue
		}
		published = append(published, r.ID)
	}

	if len(published) == 0 {
		return nil
	}
	if err := p.Repo.MarkPublished(ctx, published); err != nil {
		return err
	}
	p.log().Debug("outbox flushed", zap.Int("count", len(published)))
	return nil
}

func (p *Publisher) log() *zap.Logger {
	if p.Logger == nil {
		return zap.NewNop()
	}
	return p.Logger
}
