package pgxrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/sqlcgen"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckTemplateRepo resolves trusted playbook metadata from Postgres.
type CheckTemplateRepo struct {
	q *sqlcgen.Queries
}

var _ ports.CheckTemplateRepo = (*CheckTemplateRepo)(nil)

func NewCheckTemplateRepo(db *pgxpool.Pool) *CheckTemplateRepo {
	return &CheckTemplateRepo{q: sqlcgen.New(db)}
}

func (r *CheckTemplateRepo) GetByID(ctx context.Context, id shared.CheckTemplate) (ports.CheckTemplate, error) {
	row, err := r.q.GetCheckTemplateByID(ctx, checkTemplateID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.CheckTemplate{}, shared.ErrNotFound
	}
	if err != nil {
		return ports.CheckTemplate{}, fmt.Errorf("pgxrepo: get check template: %w", err)
	}
	return ports.CheckTemplate{
		ID:             shared.CheckTemplate(row.ID),
		Slug:           row.Slug,
		Name:           row.Name,
		PlaybookPath:   row.PlaybookPath,
		TimeoutSeconds: int(row.TimeoutSeconds),
	}, nil
}
