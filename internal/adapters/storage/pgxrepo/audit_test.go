//go:build integration

package pgxrepo

import (
	"context"
	"testing"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/audit"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
)

func TestAuditRepoAppendThenQueryByKind(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewAuditRepo(db)
	kind := "test.audit." + uuid.NewString()

	if err := repo.Append(ctx, audit.AuditEvent{
		ID:          shared.NewAuditEventID(),
		Kind:        kind,
		SubjectType: "test",
		SubjectID:   strPtr("subject-" + uuid.NewString()),
		Payload:     map[string]any{"ok": true},
		OccurredAt:  time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("append audit event: %v", err)
	}

	events, err := repo.Query(ctx, ports.AuditFilter{Kind: &kind})
	if err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	if len(events) != 1 || events[0].Kind != kind {
		t.Fatalf("expected one event of kind %s, got %+v", kind, events)
	}
}

func TestAuditRepoAppendInTx(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewAuditRepo(db)
	uow := NewUoW(db)
	kind := "test.audit.tx." + uuid.NewString()

	if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return repo.AppendInTx(ctx, tx, audit.AuditEvent{
			ID:          shared.NewAuditEventID(),
			Kind:        kind,
			SubjectType: "test",
			SubjectID:   strPtr("subject-" + uuid.NewString()),
			Payload:     map[string]any{"tx": true},
			OccurredAt:  time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
		})
	}); err != nil {
		t.Fatalf("append audit event in tx: %v", err)
	}

	events, err := repo.Query(ctx, ports.AuditFilter{Kind: &kind})
	if err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one tx event, got %+v", events)
	}
}

func TestAuditRepoQuerySinceAndLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewAuditRepo(db)
	kind := "test.audit.limit." + uuid.NewString()
	base := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)

	for i, occurredAt := range []time.Time{base, base.Add(time.Minute), base.Add(2 * time.Minute)} {
		if err := repo.Append(ctx, audit.AuditEvent{
			ID:          shared.NewAuditEventID(),
			Kind:        kind,
			SubjectType: "test",
			SubjectID:   strPtr("subject-" + uuid.NewString()),
			Payload:     map[string]any{"seq": i},
			OccurredAt:  occurredAt,
		}); err != nil {
			t.Fatalf("append audit event %d: %v", i, err)
		}
	}

	since := base.Add(time.Minute)
	events, err := repo.Query(ctx, ports.AuditFilter{Kind: &kind, Since: &since, Limit: 1})
	if err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	if len(events) != 1 || !events[0].OccurredAt.Equal(base.Add(2*time.Minute)) {
		t.Fatalf("expected newest event after since with limit 1, got %+v", events)
	}
}

func strPtr(s string) *string {
	return &s
}
