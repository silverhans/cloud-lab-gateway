// Command worker runs async tasks from the Redis-backed asynq queue.
// Task handlers (deploy, cleanup, run_check, refresh_quota, unfreeze_lab)
// are registered here from internal/app/* in downstream PRs.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/checker/ansible"
	"github.com/cloud-lab-gateway/gateway/internal/adapters/cloud/inmem"
	"github.com/cloud-lab-gateway/gateway/internal/adapters/cloud/openstack"
	queueasynq "github.com/cloud-lab-gateway/gateway/internal/adapters/queue/asynq"
	"github.com/cloud-lab-gateway/gateway/internal/adapters/secrets/envkek"
	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/pgxrepo"
	"github.com/cloud-lab-gateway/gateway/internal/app/cleanup"
	"github.com/cloud-lab-gateway/gateway/internal/app/deploy"
	"github.com/cloud-lab-gateway/gateway/internal/app/quotarefresh"
	appverify "github.com/cloud-lab-gateway/gateway/internal/app/verify"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/cloud-lab-gateway/gateway/pkg/clock"
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

	cloudProvider, providerName, err := buildCloudProvider(cfg)
	if err != nil {
		return err
	}
	log.Info("cloud provider selected", zap.String("provider", providerName))

	clk := clock.System{}
	uow := pgxrepo.NewUoW(pool)
	auditRepo := pgxrepo.NewAuditRepo(pool)
	labRepo := pgxrepo.NewLabRepo(pool)
	poolRepo := pgxrepo.NewPoolRepo(pool)
	quotaCache := pgxrepo.NewQuotaCacheRepo(pool, clk)
	taskQueue := queueasynq.NewClient(cfg.Redis.Addr)
	defer func() { _ = taskQueue.Close() }()

	keyProvider, err := envkek.NewKeyProvider()
	if err != nil {
		return err
	}
	secretStore := envkek.NewStore(pool, keyProvider, auditRepo, clk)

	registry := queueasynq.NewRegistry(cfg.Redis.Addr, 8, log)
	if err := registry.Subscribe(ports.TaskDeployLab, deploy.Deps{
		Cloud:        cloudProvider,
		Lab:          labRepo,
		Steps:        pgxrepo.NewDeployStepRepo(pool),
		Secrets:      secretStore,
		Queue:        taskQueue,
		UoW:          uow,
		Clock:        clk,
		Logger:       log,
		CleanupAfter: cfg.Lifecycle.DefaultCleanup,
	}.HandleTask); err != nil {
		return err
	}
	if err := registry.Subscribe(ports.TaskCleanupLab, cleanup.Deps{
		Cloud:   cloudProvider,
		Lab:     labRepo,
		Pool:    poolRepo,
		Secrets: secretStore,
		UoW:     uow,
		Clock:   clk,
		Logger:  log,
	}.HandleTask); err != nil {
		return err
	}
	if err := registry.Subscribe(ports.TaskRefreshQuota, quotarefresh.Deps{
		Cloud:      cloudProvider,
		QuotaCache: quotaCache,
		Logger:     log,
	}.HandleTask); err != nil {
		return err
	}
	if err := registry.Subscribe(ports.TaskRunCheck, appverify.Deps{
		UoW:       uow,
		Labs:      labRepo,
		Runs:      pgxrepo.NewCheckRunRepo(pool),
		Templates: pgxrepo.NewCheckTemplateRepo(pool),
		Secrets:   secretStore,
		Runner:    ansible.New(),
		Clock:     clk,
		Logger:    log,
	}.HandleTask); err != nil {
		return err
	}

	if err := registry.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	log.Info("worker stopped")
	return nil
}

func buildCloudProvider(cfg config.Config) (ports.CloudProvider, string, error) {
	switch cfg.CloudProvider {
	case "", "inmem":
		return inmem.New(inmem.DefaultCapacity(), inmem.Faults{}), "inmem", nil
	case "openstack":
		provider, err := openstack.New(cfg.OpenStack)
		if err != nil {
			return nil, "", fmt.Errorf("openstack provider: %w", err)
		}
		return provider, "openstack", nil
	default:
		return nil, "", fmt.Errorf("unsupported CLG_CLOUD_PROVIDER %q", cfg.CloudProvider)
	}
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
