package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options 表示日志输出配置。
type Options struct {
	Level     string
	Format    string
	AddSource bool
}

// AppMeta 表示应用固定日志字段。
type AppMeta struct {
	Service string
	Env     string
	Version string
	Commit  string
}

// NewRootLogger 创建根日志器。
func NewRootLogger(opts Options, meta AppMeta, infoWriter io.Writer, errWriter io.Writer) *slog.Logger {
	if infoWriter == nil {
		infoWriter = os.Stdout
	}
	if errWriter == nil {
		errWriter = os.Stderr
	}

	level := parseLevel(opts.Level)
	handlerOptions := &slog.HandlerOptions{
		Level:     level,
		AddSource: opts.AddSource,
	}

	infoHandler := newHandler(infoWriter, opts.Format, handlerOptions)
	errHandler := newHandler(errWriter, opts.Format, handlerOptions)

	return slog.New(newSplitHandler(infoHandler, errHandler)).With(
		slog.String("service", meta.Service),
		slog.String("env", meta.Env),
		slog.String("version", meta.Version),
		slog.String("commit", meta.Commit),
	)
}

// Component 返回带组件字段的子日志器。
func Component(base *slog.Logger, component string) *slog.Logger {
	if base == nil {
		base = slog.Default()
	}
	if strings.TrimSpace(component) == "" {
		return base
	}
	return base.With(slog.String("component", component))
}

type splitHandler struct {
	info slog.Handler
	err  slog.Handler
}

func newSplitHandler(info slog.Handler, err slog.Handler) *splitHandler {
	return &splitHandler{
		info: info,
		err:  err,
	}
}

func (h *splitHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.info.Enabled(ctx, level) || h.err.Enabled(ctx, level)
}

func (h *splitHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.Level >= slog.LevelWarn {
		return h.err.Handle(ctx, record)
	}
	return h.info.Handle(ctx, record)
}

func (h *splitHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &splitHandler{
		info: h.info.WithAttrs(attrs),
		err:  h.err.WithAttrs(attrs),
	}
}

func (h *splitHandler) WithGroup(name string) slog.Handler {
	return &splitHandler{
		info: h.info.WithGroup(name),
		err:  h.err.WithGroup(name),
	}
}

func newHandler(writer io.Writer, format string, opts *slog.HandlerOptions) slog.Handler {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		return slog.NewJSONHandler(writer, opts)
	default:
		return slog.NewTextHandler(writer, opts)
	}
}

func parseLevel(level string) slog.Leveler {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
