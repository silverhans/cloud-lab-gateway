package pgxrepo

import (
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/sqlcgen"
	"github.com/cloud-lab-gateway/gateway/internal/domain/pool"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func projectFromRow(r sqlcgen.Project) *pool.Project {
	var labID *shared.LabInstanceID
	if r.AllocatedToLabID.Valid {
		v := shared.LabInstanceID(r.AllocatedToLabID.UUID)
		labID = &v
	}

	return &pool.Project{
		ID:                shared.ProjectID(r.ID),
		KIProjectID:       r.KiProjectID,
		KIDomainID:        r.KiDomainID,
		Name:              r.Name,
		State:             pool.State(r.State),
		AllocatedToLabID:  labID,
		CleanupFailures:   int(r.CleanupFailures),
		LastStateChangeAt: timeFromTimestamptz(r.LastStateChangeAt),
		CreatedAt:         timeFromTimestamptz(r.CreatedAt),
	}
}

func projectsFromRows(rows []sqlcgen.Project) []pool.Project {
	out := make([]pool.Project, 0, len(rows))
	for _, row := range rows {
		out = append(out, *projectFromRow(row))
	}
	return out
}

func projectID(id shared.ProjectID) uuid.UUID {
	if id == (shared.ProjectID{}) {
		return uuid.New()
	}
	return uuid.UUID(id)
}

func labID(id shared.LabInstanceID) uuid.UUID {
	return uuid.UUID(id)
}

func nullableLabID(id *shared.LabInstanceID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.UUID(*id), Valid: true}
}

func timestamptz(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func timeFromTimestamptz(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}
