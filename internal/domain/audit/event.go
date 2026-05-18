// Package audit is the Audit context. AuditEvent is the append-only record of
// what happened. It is written transactionally with domain changes (outbox).
package audit

import (
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

// AuditEvent is a persisted record of a domain-significant event.
type AuditEvent struct {
	ID          shared.AuditEventID
	Kind        string
	ActorUserID *shared.UserID
	SubjectType string
	SubjectID   *string
	Payload     interface{} // JSONB
	RequestID   string
	OccurredAt  time.Time
}

// Common kinds. Adapters should use these constants, never raw strings.
const (
	KindLabCreated         = "lab.created"
	KindLabStateChanged    = "lab.state_changed"
	KindLabProjectAssigned = "lab.project_assigned"
	KindLabCleanupExtended = "lab.cleanup_extended"

	KindProjectAllocated   = "project.allocated"
	KindProjectReleasing   = "project.releasing"
	KindProjectFreed       = "project.freed"
	KindProjectQuarantined = "project.quarantined"

	KindQuotaBlocked = "quota.blocked"

	KindAccessDenied   = "access.denied"
	KindLoginSucceeded = "auth.login_succeeded"
	KindLoginFailed    = "auth.login_failed"

	KindSecretAccessed = "secret.accessed"
	KindSecretCreated  = "secret.created"

	KindCheckQueued    = "check.queued"
	KindCheckCompleted = "check.completed"

	KindSettingsChanged = "settings.changed"
)
