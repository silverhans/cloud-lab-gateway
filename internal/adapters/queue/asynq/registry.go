package asynq

import (
	"context"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// Registry implements ports.TaskRegistry: it binds task handlers to task types
// and runs the asynq worker server.
type Registry struct {
	srv *asynq.Server
	mux *asynq.ServeMux
}

var _ ports.TaskRegistry = (*Registry)(nil)

// NewRegistry builds an asynq worker server. concurrency caps the number of
// tasks processed in parallel.
func NewRegistry(redisAddr string, concurrency int, log *zap.Logger) *Registry {
	if log == nil {
		log = zap.NewNop()
	}
	if concurrency <= 0 {
		concurrency = 8
	}
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: concurrency,
			Queues:      queuePriorities,
			Logger:      zapAsynqLogger{l: log},
			ErrorHandler: asynq.ErrorHandlerFunc(func(_ context.Context, t *asynq.Task, err error) {
				log.Error("async task failed",
					zap.String("type", t.Type()),
					zap.Error(err),
				)
			}),
		},
	)
	return &Registry{srv: srv, mux: asynq.NewServeMux()}
}

// Subscribe binds a handler to a task type. Call before Run.
func (r *Registry) Subscribe(typ ports.TaskType, handler ports.TaskHandler) error {
	r.mux.HandleFunc(string(typ), func(ctx context.Context, t *asynq.Task) error {
		return handler(ctx, fromAsynqTask(t))
	})
	return nil
}

// Run starts the worker and blocks until ctx is cancelled, then shuts down
// gracefully (in-flight tasks are allowed to finish).
func (r *Registry) Run(ctx context.Context) error {
	if err := r.srv.Start(r.mux); err != nil {
		return err
	}
	<-ctx.Done()
	r.srv.Shutdown()
	return ctx.Err()
}

// zapAsynqLogger adapts a zap logger to asynq's logging interface.
type zapAsynqLogger struct{ l *zap.Logger }

func (z zapAsynqLogger) Debug(args ...interface{}) { z.l.Sugar().Debug(args...) }
func (z zapAsynqLogger) Info(args ...interface{})  { z.l.Sugar().Info(args...) }
func (z zapAsynqLogger) Warn(args ...interface{})  { z.l.Sugar().Warn(args...) }
func (z zapAsynqLogger) Error(args ...interface{}) { z.l.Sugar().Error(args...) }
func (z zapAsynqLogger) Fatal(args ...interface{}) { z.l.Sugar().Fatal(args...) }
