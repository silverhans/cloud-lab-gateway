package pgxrepo

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/sqlcgen"
	"github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type kiResourcesJSON struct {
	ServerIDs   []string `json:"server_ids,omitempty"`
	NetworkID   string   `json:"network_id,omitempty"`
	FloatingIPs []string `json:"floating_ips,omitempty"`
	KeypairName string   `json:"keypair_name,omitempty"`
}

func labFromRow(r sqlcgen.LabInstance) (*lab.LabInstance, error) {
	resources, err := kiResourcesFromJSON(r.KiResources)
	if err != nil {
		return nil, err
	}

	return &lab.LabInstance{
		ID:                    shared.LabInstanceID(r.ID),
		StudentUserID:         shared.UserID(r.StudentUserID),
		CourseID:              shared.CourseID(r.CourseID),
		LabTemplateID:         shared.LabTemplateID(r.LabTemplateID),
		ProjectID:             projectIDFromNull(r.ProjectID),
		State:                 lab.State(r.State),
		StateReason:           r.StateReason,
		KIResources:           resources,
		CleanupAt:             timePtrFromTimestamptz(r.CleanupAt),
		UnfreezeAt:            timePtrFromTimestamptz(r.UnfreezeAt),
		FrozenByUserID:        userIDFromNull(r.FrozenByUserID),
		FrozenReason:          stringFromPtr(r.FrozenReason),
		StudentSSHKeySecretID: secretIDFromNull(r.StudentSshKeySecretID),
		CheckerSSHKeySecretID: secretIDFromNull(r.CheckerSshKeySecretID),
		CreatedAt:             timeFromTimestamptz(r.CreatedAt),
		UpdatedAt:             timeFromTimestamptz(r.UpdatedAt),
	}, nil
}

func labsFromRows(rows []sqlcgen.LabInstance) ([]lab.LabInstance, error) {
	out := make([]lab.LabInstance, 0, len(rows))
	for _, row := range rows {
		l, err := labFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, nil
}

func createLabParams(l *lab.LabInstance) (sqlcgen.CreateLabInstanceParams, error) {
	resources, err := kiResourcesToJSON(l.KIResources)
	if err != nil {
		return sqlcgen.CreateLabInstanceParams{}, err
	}
	return sqlcgen.CreateLabInstanceParams{
		ID:                    labID(l.ID),
		StudentUserID:         userID(l.StudentUserID),
		CourseID:              courseID(l.CourseID),
		LabTemplateID:         labTemplateID(l.LabTemplateID),
		ProjectID:             nullableProjectID(l.ProjectID),
		State:                 string(l.State),
		StateReason:           l.StateReason,
		KiResources:           resources,
		CleanupAt:             timestamptzPtr(l.CleanupAt),
		UnfreezeAt:            timestamptzPtr(l.UnfreezeAt),
		FrozenByUserID:        nullableUserID(l.FrozenByUserID),
		FrozenReason:          nullableString(l.FrozenReason),
		StudentSshKeySecretID: nullableSecretID(l.StudentSSHKeySecretID),
		CheckerSshKeySecretID: nullableSecretID(l.CheckerSSHKeySecretID),
		CreatedAt:             nullableTimeArg(l.CreatedAt),
		UpdatedAt:             nullableTimeArg(l.UpdatedAt),
	}, nil
}

func updateLabParams(l *lab.LabInstance) (sqlcgen.UpdateLabInstanceParams, error) {
	resources, err := kiResourcesToJSON(l.KIResources)
	if err != nil {
		return sqlcgen.UpdateLabInstanceParams{}, err
	}
	return sqlcgen.UpdateLabInstanceParams{
		ID:                    labID(l.ID),
		ProjectID:             nullableProjectID(l.ProjectID),
		State:                 string(l.State),
		StateReason:           l.StateReason,
		KiResources:           resources,
		CleanupAt:             timestamptzPtr(l.CleanupAt),
		UnfreezeAt:            timestamptzPtr(l.UnfreezeAt),
		FrozenByUserID:        nullableUserID(l.FrozenByUserID),
		FrozenReason:          nullableString(l.FrozenReason),
		StudentSshKeySecretID: nullableSecretID(l.StudentSSHKeySecretID),
		CheckerSshKeySecretID: nullableSecretID(l.CheckerSSHKeySecretID),
		UpdatedAt:             timestamptz(l.UpdatedAt),
	}, nil
}

func kiResourcesToJSON(resources lab.KIResources) ([]byte, error) {
	payload, err := json.Marshal(kiResourcesJSON{
		ServerIDs:   resources.ServerIDs,
		NetworkID:   resources.NetworkID,
		FloatingIPs: resources.FloatingIPs,
		KeypairName: resources.KeypairName,
	})
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: marshal lab ki resources: %w", err)
	}
	return payload, nil
}

func kiResourcesFromJSON(payload []byte) (lab.KIResources, error) {
	if len(payload) == 0 {
		return lab.KIResources{}, nil
	}
	var resources kiResourcesJSON
	if err := json.Unmarshal(payload, &resources); err != nil {
		return lab.KIResources{}, fmt.Errorf("pgxrepo: unmarshal lab ki resources: %w", err)
	}
	return lab.KIResources{
		ServerIDs:   resources.ServerIDs,
		NetworkID:   resources.NetworkID,
		FloatingIPs: resources.FloatingIPs,
		KeypairName: resources.KeypairName,
	}, nil
}

func userID(id shared.UserID) uuid.UUID {
	return uuid.UUID(id)
}

func courseID(id shared.CourseID) uuid.UUID {
	return uuid.UUID(id)
}

func labTemplateID(id shared.LabTemplateID) uuid.UUID {
	return uuid.UUID(id)
}

func nullableProjectID(id *shared.ProjectID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.UUID(*id), Valid: true}
}

func projectIDFromNull(id uuid.NullUUID) *shared.ProjectID {
	if !id.Valid {
		return nil
	}
	v := shared.ProjectID(id.UUID)
	return &v
}

func nullableUserID(id *shared.UserID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.UUID(*id), Valid: true}
}

func nullableCourseID(id *shared.CourseID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.UUID(*id), Valid: true}
}

func courseIDs(ids []shared.CourseID) []uuid.UUID {
	if len(ids) == 0 {
		return []uuid.UUID{}
	}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		out = append(out, uuid.UUID(id))
	}
	return out
}

func labStates(states []lab.State) []string {
	if len(states) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(states))
	for _, state := range states {
		out = append(out, string(state))
	}
	return out
}

func userIDFromNull(id uuid.NullUUID) *shared.UserID {
	if !id.Valid {
		return nil
	}
	v := shared.UserID(id.UUID)
	return &v
}

func nullableSecretID(id *shared.SecretID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.UUID(*id), Valid: true}
}

func secretIDFromNull(id uuid.NullUUID) *shared.SecretID {
	if !id.Valid {
		return nil
	}
	v := shared.SecretID(id.UUID)
	return &v
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func stringFromPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func timestamptzPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return timestamptz(*t)
}

func timePtrFromTimestamptz(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
