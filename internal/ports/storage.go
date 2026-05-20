package ports

import (
	"context"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/audit"
	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	"github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/pool"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/domain/verify"
)

// UnitOfWork wraps a database transaction. Use it to enforce transactional
// outbox: aggregate save + audit event append happen in the same tx.
type UnitOfWork interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

// Tx is a marker interface; concrete adapters expose typed accessors.
// Domain code should never type-assert this — pass it to the repo methods.
type Tx interface {
	Private()
}

// PoolRepo manages persistence and atomic allocation of Project aggregates.
type PoolRepo interface {
	// AllocateOneFree atomically reserves one FREE project in the given КИ
	// domain. MUST use SELECT ... FOR UPDATE SKIP LOCKED to avoid double-rent.
	// Returns shared.ErrPoolEmpty when no FREE project is available.
	AllocateOneFree(ctx context.Context, tx Tx, kiDomainID string, labID shared.LabInstanceID) (*pool.Project, error)

	// Save persists state changes and flushes domain events to outbox.
	Save(ctx context.Context, tx Tx, p *pool.Project) error

	GetByID(ctx context.Context, id shared.ProjectID) (*pool.Project, error)
	ListByDomain(ctx context.Context, kiDomainID string, state *pool.State) ([]pool.Project, error)

	// SeedInsert is used by `gateway seed` to bulk-insert pre-created КИ projects.
	SeedInsert(ctx context.Context, projects []pool.Project) error
}

// LabRepo manages LabInstance aggregates.
type LabRepo interface {
	Create(ctx context.Context, tx Tx, l *lab.LabInstance) error
	Save(ctx context.Context, tx Tx, l *lab.LabInstance) error
	GetByID(ctx context.Context, id shared.LabInstanceID) (*lab.LabInstance, error)

	// FindActiveByStudentAndCourse returns the single active lab (state not in
	// {done, rejected, failed}) for the (student, course) pair, if any.
	FindActiveByStudentAndCourse(ctx context.Context, studentID shared.UserID, courseID shared.CourseID) (*lab.LabInstance, error)

	// ListPendingCleanup returns ready labs whose cleanup_at <= before.
	ListPendingCleanup(ctx context.Context, before time.Time, limit int) ([]lab.LabInstance, error)

	// ListPendingUnfreeze returns frozen labs whose unfreeze_at <= before.
	ListPendingUnfreeze(ctx context.Context, before time.Time, limit int) ([]lab.LabInstance, error)
}

// DeployStepRepo persists the saga state of the deploy workflow.
//
// Methods take no Tx: deploy-step records are written between cloud calls
// (BootServer, WaitForActive) that may take tens of seconds, so they can
// never share one database transaction. Each call is its own short write.
type DeployStepRepo interface {
	// GetOrInit returns the existing record for (labID, step), or a fresh
	// pending record if none exists yet.
	GetOrInit(ctx context.Context, labID shared.LabInstanceID, step string) (DeployStep, error)
	Save(ctx context.Context, s DeployStep) error
	ListByLab(ctx context.Context, labID shared.LabInstanceID) ([]DeployStep, error)
}

// DeployStep is the persistent form of one saga step.
type DeployStep struct {
	LabID      shared.LabInstanceID
	StepName   string
	Status     string
	Attempt    int
	LastError  string
	Result     []byte // JSON
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// CheckRunRepo persists CheckRun aggregates.
type CheckRunRepo interface {
	Create(ctx context.Context, tx Tx, r *verify.CheckRun) error
	Save(ctx context.Context, tx Tx, r *verify.CheckRun) error
	GetByID(ctx context.Context, id shared.CheckRunID) (*verify.CheckRun, error)
	ListByLab(ctx context.Context, labID shared.LabInstanceID, limit int) ([]verify.CheckRun, error)
}

// UserRepo persists Users + LTIIdentities.
type UserRepo interface {
	GetByID(ctx context.Context, id shared.UserID) (*identity.User, error)
	GetByLTI(ctx context.Context, iss, sub string) (*identity.User, error)
	UpsertFromLaunch(ctx context.Context, tx Tx, iss, sub, displayName, email string, role identity.Role) (*identity.User, error)

	// GetCourseRoles loads roles in courses for a user (for policy decisions).
	GetCourseRoles(ctx context.Context, userID shared.UserID) (map[shared.CourseID]identity.CourseRole, error)
}

// CourseRepo persists Courses and Enrollments.
type CourseRepo interface {
	GetByID(ctx context.Context, id shared.CourseID) (*identity.Course, error)
	GetByExternalID(ctx context.Context, externalID string) (*identity.Course, error)
	Upsert(ctx context.Context, tx Tx, c *identity.Course) error
	Enroll(ctx context.Context, tx Tx, e identity.Enrollment) error
}

// SettingsRepo loads / stores Settings with scope hierarchy.
type SettingsRepo interface {
	// Resolve returns the most-specific value for the given key, taking into
	// account scope hierarchy (lab_template > course > global).
	Resolve(ctx context.Context, key string, courseID *shared.CourseID, labTemplateID *shared.LabTemplateID) ([]byte, error)
	List(ctx context.Context, scope string, scopeID *string) ([]Setting, error)
	Put(ctx context.Context, tx Tx, s Setting) error
}

type Setting struct {
	Key           string
	Value         []byte // JSON
	Scope         string // "global" | "per_course" | "per_lab_template"
	ScopeID       *string
	UpdatedByUser shared.UserID
	UpdatedAt     time.Time
}

// AuditRepo persists append-only audit events.
type AuditRepo interface {
	Append(ctx context.Context, ev audit.AuditEvent) error
	AppendInTx(ctx context.Context, tx Tx, ev audit.AuditEvent) error
	Query(ctx context.Context, f AuditFilter) ([]audit.AuditEvent, error)
}

type AuditFilter struct {
	Kind        *string
	ActorUserID *shared.UserID
	Since       *time.Time
	Limit       int
}
