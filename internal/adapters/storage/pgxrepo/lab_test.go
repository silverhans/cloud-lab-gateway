//go:build integration

package pgxrepo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type labRefs struct {
	StudentID  shared.UserID
	CourseID   shared.CourseID
	TemplateID shared.LabTemplateID
}

func TestLabRepoCreateGetRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewLabRepo(db)
	uow := NewUoW(db)
	refs := seedLabRefs(t, db)
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	inst := labdomain.New(shared.NewLabInstanceID(), refs.StudentID, refs.CourseID, refs.TemplateID, now)

	if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return repo.Create(ctx, tx, inst)
	}); err != nil {
		t.Fatalf("create lab: %v", err)
	}

	got, err := repo.GetByID(ctx, inst.ID)
	if err != nil {
		t.Fatalf("get lab: %v", err)
	}
	if got.ID != inst.ID || got.StudentUserID != refs.StudentID || got.CourseID != refs.CourseID || got.State != labdomain.StatePendingQuota {
		t.Fatalf("round trip mismatch: got %+v want id=%s student=%s course=%s state=%s", got, inst.ID, refs.StudentID, refs.CourseID, labdomain.StatePendingQuota)
	}
}

func TestLabRepoCreateMapsActiveUniqueViolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewLabRepo(db)
	uow := NewUoW(db)
	refs := seedLabRefs(t, db)
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)

	first := labdomain.New(shared.NewLabInstanceID(), refs.StudentID, refs.CourseID, refs.TemplateID, now)
	second := labdomain.New(shared.NewLabInstanceID(), refs.StudentID, refs.CourseID, refs.TemplateID, now.Add(time.Second))

	if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return repo.Create(ctx, tx, first)
	}); err != nil {
		t.Fatalf("create first lab: %v", err)
	}
	err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return repo.Create(ctx, tx, second)
	})
	if !errors.Is(err, shared.ErrLabAlreadyActive) {
		t.Fatalf("expected ErrLabAlreadyActive, got %v", err)
	}
}

func TestLabRepoSaveFlushesEventsToOutbox(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewLabRepo(db)
	uow := NewUoW(db)
	refs := seedLabRefs(t, db)
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	inst := labdomain.New(shared.NewLabInstanceID(), refs.StudentID, refs.CourseID, refs.TemplateID, now)

	if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return repo.Create(ctx, tx, inst)
	}); err != nil {
		t.Fatalf("create lab: %v", err)
	}
	if err := inst.Transition(labdomain.StatePendingProject, "quota_ok", now.Add(time.Minute)); err != nil {
		t.Fatalf("transition lab: %v", err)
	}
	if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return repo.Save(ctx, tx, inst)
	}); err != nil {
		t.Fatalf("save lab: %v", err)
	}

	var outboxCount int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE topic = 'lab.state_changed' AND payload->>'lab_id' = $1", inst.ID.String()).Scan(&outboxCount); err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("expected one lab.state_changed outbox event, got %d", outboxCount)
	}

	var auditCount int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE kind = 'lab.state_changed' AND subject_id = $1", inst.ID.String()).Scan(&auditCount); err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one lab.state_changed audit event, got %d", auditCount)
	}
}

func TestLabRepoFindActiveByStudentAndCourse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewLabRepo(db)
	uow := NewUoW(db)
	refs := seedLabRefs(t, db)
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)

	missing, err := repo.FindActiveByStudentAndCourse(ctx, refs.StudentID, refs.CourseID)
	if err != nil {
		t.Fatalf("find missing active lab: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected no active lab, got %+v", missing)
	}

	inst := labdomain.New(shared.NewLabInstanceID(), refs.StudentID, refs.CourseID, refs.TemplateID, now)
	if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return repo.Create(ctx, tx, inst)
	}); err != nil {
		t.Fatalf("create lab: %v", err)
	}

	active, err := repo.FindActiveByStudentAndCourse(ctx, refs.StudentID, refs.CourseID)
	if err != nil {
		t.Fatalf("find active lab: %v", err)
	}
	if active == nil || active.ID != inst.ID {
		t.Fatalf("expected active lab %s, got %+v", inst.ID, active)
	}
}

func TestLabRepoListPendingCleanup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewLabRepo(db)
	uow := NewUoW(db)
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)

	overdue := readyLab(t, seedLabRefs(t, db), now, now.Add(-time.Minute))
	future := readyLab(t, seedLabRefs(t, db), now, now.Add(time.Hour))
	frozen := frozenLab(t, seedLabRefs(t, db), now, now.Add(-time.Minute))

	for _, inst := range []*labdomain.LabInstance{overdue, future, frozen} {
		if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
			return repo.Create(ctx, tx, inst)
		}); err != nil {
			t.Fatalf("create lab %s: %v", inst.ID, err)
		}
	}

	labs, err := repo.ListPendingCleanup(ctx, now, 10)
	if err != nil {
		t.Fatalf("list pending cleanup: %v", err)
	}
	if len(labs) != 1 || labs[0].ID != overdue.ID {
		t.Fatalf("expected only overdue ready lab %s, got %+v; future=%s frozen=%s", overdue.ID, labs, future.ID, frozen.ID)
	}
}

func seedLabRefs(t *testing.T, db *pgxpool.Pool) labRefs {
	t.Helper()
	ctx := context.Background()
	studentID := insertTestUser(t, db, identity.RoleStudent)
	courseID := shared.CourseID(uuid.New())
	templateID := shared.LabTemplateID(uuid.New())

	if _, err := db.Exec(ctx,
		"INSERT INTO courses (id, external_id, name, ki_domain_id) VALUES ($1, $2, $3, $4)",
		uuid.UUID(courseID), "course-"+uuid.NewString(), "Course "+uuid.NewString(), "domain-"+uuid.NewString(),
	); err != nil {
		t.Fatalf("insert course: %v", err)
	}
	if _, err := db.Exec(ctx,
		"INSERT INTO lab_templates (id, slug, name, description, topology) VALUES ($1, $2, $3, $4, '{}'::jsonb)",
		uuid.UUID(templateID), "template-"+uuid.NewString(), "Template "+uuid.NewString(), "test template",
	); err != nil {
		t.Fatalf("insert lab template: %v", err)
	}
	return labRefs{StudentID: studentID, CourseID: courseID, TemplateID: templateID}
}

func insertTestUser(t *testing.T, db *pgxpool.Pool, role identity.Role) shared.UserID {
	t.Helper()
	id := shared.UserID(uuid.New())
	if _, err := db.Exec(context.Background(),
		"INSERT INTO users (id, display_name, role) VALUES ($1, $2, $3)",
		uuid.UUID(id), "User "+uuid.NewString(), string(role),
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func readyLab(t *testing.T, refs labRefs, now, cleanupAt time.Time) *labdomain.LabInstance {
	t.Helper()
	inst := labdomain.New(shared.NewLabInstanceID(), refs.StudentID, refs.CourseID, refs.TemplateID, now)
	if err := inst.Transition(labdomain.StatePendingProject, "quota_ok", now.Add(time.Second)); err != nil {
		t.Fatalf("pending project transition: %v", err)
	}
	if err := inst.Transition(labdomain.StateDeploying, "project_assigned", now.Add(2*time.Second)); err != nil {
		t.Fatalf("deploying transition: %v", err)
	}
	if err := inst.MarkReady(cleanupAt, now.Add(3*time.Second)); err != nil {
		t.Fatalf("mark ready: %v", err)
	}
	return inst
}

func frozenLab(t *testing.T, refs labRefs, now, unfreezeAt time.Time) *labdomain.LabInstance {
	t.Helper()
	inst := readyLab(t, refs, now, now.Add(time.Hour))
	if err := inst.Freeze(refs.StudentID, "test freeze", unfreezeAt, now.Add(4*time.Second)); err != nil {
		t.Fatalf("freeze lab: %v", err)
	}
	return inst
}
