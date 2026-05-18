// Command migrate is a thin wrapper around goose for use inside containers.
// It exists so the docker-compose `migrate` service has a single, well-defined
// entrypoint without shelling out to a `goose` binary.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/spf13/cobra"
)

func main() {
	dir := "/migrations"
	if v := os.Getenv("MIGRATIONS_DIR"); v != "" {
		dir = v
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}

	dsn := os.Getenv("PG_DSN")

	var direction string
	root := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		RunE: func(_ *cobra.Command, args []string) error {
			direction = "up"
			if len(args) > 0 {
				direction = args[0]
			}
			return runMigrate(dsn, dir, direction)
		},
	}
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runMigrate(dsn, dir, direction string) error {
	if dsn == "" {
		return fmt.Errorf("PG_DSN is required")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}
	db := stdlib.OpenDB(*cfg)
	defer func() { _ = db.Close() }()

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	ctx := context.Background()
	switch direction {
	case "up":
		return goose.UpContext(ctx, db, dir)
	case "down":
		return goose.DownContext(ctx, db, dir)
	case "status":
		return goose.StatusContext(ctx, db, dir)
	case "redo":
		return goose.RedoContext(ctx, db, dir)
	default:
		return fmt.Errorf("unknown direction %q (use up|down|status|redo)", direction)
	}
}
