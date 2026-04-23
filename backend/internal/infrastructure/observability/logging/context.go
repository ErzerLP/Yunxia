package logging

import (
	"context"
	"log/slog"
)

type loggerContextKey struct{}

// WithLogger 把日志器写入 context。
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

// FromContext 读取 context 中的日志器。
func FromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if ctx != nil {
		if logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger); ok && logger != nil {
			return logger
		}
	}
	if fallback != nil {
		return fallback
	}
	return slog.Default()
}
