// Package logger constructs the application zap logger. All processes use
// the same factory so log lines are uniform (JSON, stdout, RFC3339 timestamps).
package logger

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New builds a zap.Logger from a level string ("debug" | "info" | "warn" | "error").
// Unknown levels fall back to info with a warning.
func New(level string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.LevelKey = "lvl"
	cfg.EncoderConfig.MessageKey = "msg"
	cfg.EncoderConfig.CallerKey = "caller"
	cfg.EncoderConfig.StacktraceKey = "stack"
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	cfg.EncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(strings.ToLower(level))); err != nil {
		lvl = zapcore.InfoLevel
	}
	cfg.Level = zap.NewAtomicLevelAt(lvl)

	l, err := cfg.Build(zap.AddCallerSkip(0))
	if err != nil {
		return nil, fmt.Errorf("build zap logger: %w", err)
	}
	return l, nil
}

// MustNew is a panic-on-error helper for cmd/* bootstrap where logger failure
// is fatal anyway.
func MustNew(level string) *zap.Logger {
	l, err := New(level)
	if err != nil {
		panic(err)
	}
	return l
}
