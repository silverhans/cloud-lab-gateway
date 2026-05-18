// Command worker runs async tasks from the Redis-backed asynq queue.
// Task handlers (deploy, cleanup, run_check, refresh_quota, unfreeze_lab)
// are registered here from internal/app/* in downstream PRs.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/cloud-lab-gateway/gateway/pkg/config"
	"github.com/cloud-lab-gateway/gateway/pkg/logger"
)

func main() {
	root := &cobra.Command{Use: "worker", Short: "Cloud Lab Gateway async worker"}
	root.AddCommand(newRunCmd(), newVersionCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Consume tasks from the queue",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runWorker()
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version info",
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Println("cloud-lab-gateway/worker dev")
		},
	}
}

func runWorker() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logger.MustNew(cfg.LogLevel)
	defer func() { _ = log.Sync() }()
	log.Info("worker starting")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := openPostgres(ctx, cfg.PG.DSN)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()

	rdb, err := openRedis(ctx, cfg.Redis.Addr)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	defer func() { _ = rdb.Close() }()

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.Redis.Addr},
		asynq.Config{
			Concurrency: 8,
			Queues: map[string]int{
				"deploy":  4,
				"cleanup": 2,
				"checks":  2,
				"default": 1,
			},
			Logger: asynqZapLogger{l: log},
			ErrorHandler: asynq.ErrorHandlerFunc(func(_ context.Context, t *asynq.Task, err error) {
				log.Error("task failed", zap.String("type", t.Type()), zap.Error(err))
			}),
		},
	)

	mux := asynq.NewServeMux()
	// Task handlers are registered here from internal/app/* in downstream PRs:
	//   mux.HandleFunc(string(ports.TaskDeployLab), deploy.NewHandler(deps).Handle)
	//   mux.HandleFunc(string(ports.TaskCleanupLab), cleanup.NewHandler(deps).Handle)
	//   ...

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(mux)
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
		srv.Shutdown()
	case err := <-errCh:
		if err != nil {
			return err
		}
	}
	log.Info("worker stopped")
	return nil
}

func openPostgres(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func openRedis(ctx context.Context, addr string) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}
	return rdb, nil
}

// asynqZapLogger adapts zap to asynq's logger interface.
type asynqZapLogger struct{ l *zap.Logger }

func (a asynqZapLogger) Debug(args ...interface{}) { a.l.Sugar().Debug(args...) }
func (a asynqZapLogger) Info(args ...interface{})  { a.l.Sugar().Info(args...) }
func (a asynqZapLogger) Warn(args ...interface{})  { a.l.Sugar().Warn(args...) }
func (a asynqZapLogger) Error(args ...interface{}) { a.l.Sugar().Error(args...) }
func (a asynqZapLogger) Fatal(args ...interface{}) { a.l.Sugar().Fatal(args...) }
