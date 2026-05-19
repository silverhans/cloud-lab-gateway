// Command gateway is the HTTP API + LTI + SSE process of Cloud Lab Gateway.
//
// Subcommands:
//
//	serve         — start the HTTP server (default)
//	healthcheck   — exit 0 if /healthz responds, used by Docker HEALTHCHECK
//	version       — print build info
//
// Wiring overview (executed in serve):
//
//	config.Load → logger → pgx pool → redis → asynq client → chi router → http.Server
//
// Application use-cases and adapters are wired by app-layer code (added by
// downstream PRs); this file owns process bootstrap and graceful shutdown only.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/cloud-lab-gateway/gateway/pkg/config"
	"github.com/cloud-lab-gateway/gateway/pkg/logger"
)

func main() {
	root := &cobra.Command{
		Use:   "gateway",
		Short: "Cloud Lab Gateway HTTP API",
	}
	root.AddCommand(newServeCmd(), newHealthcheckCmd(), newVersionCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the HTTP server",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServe()
		},
	}
}

func newHealthcheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "healthcheck",
		Short: "Probe /healthz on the local server (exit 0 if healthy)",
		RunE: func(_ *cobra.Command, _ []string) error {
			addr := os.Getenv("CLG_BIND_ADDR")
			if addr == "" {
				addr = "127.0.0.1:8080"
			}
			url := "http://" + addr + "/healthz"
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("healthz returned %d", resp.StatusCode)
			}
			return nil
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version info",
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Println("cloud-lab-gateway/gateway dev")
		},
	}
}

func runServe() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if vErr := cfg.ValidateForGateway(); vErr != nil {
		return fmt.Errorf("config validation: %w", vErr)
	}

	log := logger.MustNew(cfg.LogLevel)
	defer func() { _ = log.Sync() }()
	log.Info("gateway starting", zap.String("bind", cfg.BindAddr))

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

	router := chi.NewRouter()
	router.Use(
		middleware.RequestID,
		middleware.RealIP,
		middleware.Recoverer,
		zapRequestLogger(log),
	)
	mountHealth(router, pool, rdb)

	// Application routes are wired here by downstream PRs.
	// router.Mount("/api/v1/", httpapi.NewMux(deps))
	// router.Mount("/sse/", sse.NewMux(broker))
	// router.Mount("/lti/", lti13.NewMux(deps))

	srv := &http.Server{
		Addr:              cfg.BindAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("http listening", zap.String("addr", srv.Addr))
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown error", zap.Error(err))
	}
	log.Info("gateway stopped")
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

func mountHealth(r chi.Router, pool *pgxpool.Pool, rdb *redis.Client) {
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			http.Error(w, "postgres not ready", http.StatusServiceUnavailable)
			return
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			http.Error(w, "redis not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

func zapRequestLogger(l *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			l.Info("http",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", ww.Status()),
				zap.Int("bytes", ww.BytesWritten()),
				zap.Duration("dur", time.Since(start)),
				zap.String("request_id", middleware.GetReqID(r.Context())),
			)
		})
	}
}
