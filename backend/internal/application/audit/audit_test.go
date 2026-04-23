package audit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"yunxia/internal/domain/entity"
)

func TestProjectStorageSourceMasksSecrets(t *testing.T) {
	before := ProjectStorageSource(map[string]any{
		"name": "archive",
		"config": map[string]any{
			"endpoint":   "https://s3.example.com",
			"bucket":     "demo",
			"access_key": "AKIA-RAW",
			"secret_key": "super-secret",
		},
	})

	if strings.Contains(before, "super-secret") || strings.Contains(before, "AKIA-RAW") {
		t.Fatalf("expected secrets masked, got %s", before)
	}
}

func TestRecordBestEffortLogsAuditWriteFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	recorder := NewRecorder(failingRepository{err: context.DeadlineExceeded}, logger)

	RecordBestEffort(context.Background(), recorder, logger, Event{
		ResourceType: "user",
		Action:       "create",
		Result:       ResultSuccess,
	})

	if !strings.Contains(buf.String(), "audit.write.failed") {
		t.Fatalf("expected audit.write.failed log, got %s", buf.String())
	}
}

type failingRepository struct {
	err error
}

func (r failingRepository) Create(context.Context, *entity.AuditLog) error {
	if r.err != nil {
		return r.err
	}
	return errors.New("unexpected call")
}
