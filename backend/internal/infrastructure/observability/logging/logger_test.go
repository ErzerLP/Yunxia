package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestNewRootLoggerWritesJSONToInfoStream(t *testing.T) {
	var info bytes.Buffer
	var errBuf bytes.Buffer

	logger := NewRootLogger(Options{
		Level:     "info",
		Format:    "json",
		AddSource: false,
	}, AppMeta{
		Service: "yunxia-backend",
		Env:     "test",
		Version: "dev",
	}, &info, &errBuf)

	logger.Info("server started", slog.String("event", "app.start"))

	line := strings.TrimSpace(info.String())
	if !strings.Contains(line, `"service":"yunxia-backend"`) {
		t.Fatalf("expected service field, got %s", line)
	}
	if !strings.Contains(line, `"event":"app.start"`) {
		t.Fatalf("expected event field, got %s", line)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("expected empty error stream, got %q", errBuf.String())
	}
}

func TestContextRoundTrip(t *testing.T) {
	base := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctx := WithLogger(context.Background(), base)
	got := FromContext(ctx, slog.Default())
	if got != base {
		t.Fatalf("expected stored logger instance")
	}
}
