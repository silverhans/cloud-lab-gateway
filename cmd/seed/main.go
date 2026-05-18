// Command seed populates the project pool from a CSV file. Idempotent: rows
// already present (matched on ki_project_id) are skipped.
//
// CSV format (header optional):
//
//	ki_project_id,ki_domain_id,name
//	openstack-proj-001,course-linux-101,Linux Lab Slot 1
//	openstack-proj-002,course-linux-101,Linux Lab Slot 2
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var (
		csvPath string
		dsn     string
	)
	flag.StringVar(&csvPath, "csv", "", "path to CSV file (ki_project_id,ki_domain_id,name)")
	flag.StringVar(&dsn, "dsn", os.Getenv("PG_DSN"), "Postgres DSN (defaults to PG_DSN env)")
	flag.Parse()

	if csvPath == "" || dsn == "" {
		fmt.Fprintln(os.Stderr, "usage: seed --csv=projects.csv [--dsn=postgres://...]")
		os.Exit(2)
	}

	if err := run(csvPath, dsn); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(csvPath, dsn string) error {
	rows, err := readCSV(csvPath)
	if err != nil {
		return fmt.Errorf("read csv: %w", err)
	}
	if len(rows) == 0 {
		return errors.New("csv has no data rows")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	const stmt = `
		INSERT INTO projects (ki_project_id, ki_domain_id, name, state)
		VALUES ($1, $2, $3, 'free')
		ON CONFLICT (ki_project_id) DO NOTHING
	`

	var inserted, skipped int
	for _, r := range rows {
		tag, err := pool.Exec(ctx, stmt, r.kiProjectID, r.kiDomainID, r.name)
		if err != nil {
			return fmt.Errorf("insert %s: %w", r.kiProjectID, err)
		}
		if tag.RowsAffected() == 1 {
			inserted++
		} else {
			skipped++
		}
	}
	fmt.Printf("seed complete: inserted=%d skipped=%d total=%d\n", inserted, skipped, len(rows))
	return nil
}

type projectRow struct {
	kiProjectID string
	kiDomainID  string
	name        string
}

func readCSV(path string) ([]projectRow, error) {
	f, err := os.Open(path) //nolint:gosec // CLI tool, path comes from operator
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = 3

	var out []projectRow
	for i := 0; ; i++ {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i+1, err)
		}
		if i == 0 && strings.EqualFold(row[0], "ki_project_id") {
			continue // header
		}
		if row[0] == "" || row[1] == "" || row[2] == "" {
			return nil, fmt.Errorf("row %d: empty field", i+1)
		}
		out = append(out, projectRow{
			kiProjectID: row[0],
			kiDomainID:  row[1],
			name:        row[2],
		})
	}
	return out, nil
}
