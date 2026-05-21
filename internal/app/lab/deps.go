// Package lab holds the Lab Lifecycle application use cases. The domain
// aggregate of the same name is imported as `labdomain` to avoid a clash
// with this package name.
package lab

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// QuotaCache is the use case's view of the cached КИ utilization snapshot.
// The concrete implementation (Postgres or Redis backed) lives in adapters.
type QuotaCache interface {
	// Read returns the snapshot and how old it is. A missing snapshot must
	// return a very large age so the caller treats it as stale.
	Read(ctx context.Context) (snap shared.QuotaSnapshot, age time.Duration, err error)
	Write(ctx context.Context, snap shared.QuotaSnapshot) error
}

// LabReader is the list read-model used by lifecycle endpoints.
type LabReader interface {
	List(ctx context.Context, f LabListFilter) ([]labdomain.LabInstance, error)
}

type LabListFilter struct {
	StudentUserID *shared.UserID
	CourseID      *shared.CourseID
	CourseIDs     []shared.CourseID
	States        []labdomain.State
	Limit         int
}

// Actor is the authenticated user asking to perform a lab operation.
type Actor struct {
	UserID      shared.UserID
	Role        identity.Role
	CourseRoles map[shared.CourseID]identity.CourseRole
}

type LabDetail struct {
	Lab         *labdomain.LabInstance
	DeploySteps []ports.DeployStep
}

// Deps bundles every collaborator the lab use cases need. It is constructed
// once at process startup and shared (all fields are safe for concurrent use).
type Deps struct {
	UoW        ports.UnitOfWork
	Pool       ports.PoolRepo
	Lab        ports.LabRepo
	LabReader  LabReader
	Steps      ports.DeployStepRepo
	Courses    ports.CourseRepo
	Audit      ports.AuditRepo
	QuotaCache QuotaCache
	Queue      ports.TaskQueue
	Secrets    ports.SecretStore
	Clock      ports.Clock
	Logger     *zap.Logger

	// QuotaThresholdPct is the cluster-utilization ceiling (default 90).
	// Sourced from CLG_QUOTA_THRESHOLD_PCT; 0 falls back to the domain default.
	QuotaThresholdPct float64

	// QuotaMaxAge marks the cached snapshot stale. Older than this and
	// CreateLab fails closed (returns 503-mapped error) rather than deciding
	// on data that may no longer reflect the cluster. 0 → 60s.
	QuotaMaxAge time.Duration

	DefaultCleanupAfter time.Duration
	DefaultFreezeFor    time.Duration
}

func (d Deps) log() *zap.Logger {
	if d.Logger == nil {
		return zap.NewNop()
	}
	return d.Logger
}
