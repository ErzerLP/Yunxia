package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	appLog "yunxia/internal/infrastructure/observability/logging"
)

func TestAccessLogWritesCompletedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var info bytes.Buffer
	logger := appLog.NewRootLogger(appLog.Options{Level: "info", Format: "json"}, appLog.AppMeta{Service: "yunxia-backend"}, &info, &info)

	r := gin.New()
	r.Use(RequestID(), AccessLog(logger, "/dav", map[string]struct{}{"/api/v1/health": {}}))
	r.GET("/api/v1/system/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/version", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !bytes.Contains(info.Bytes(), []byte(`"event":"http.request.completed"`)) {
		t.Fatalf("expected completed access log, got %s", info.String())
	}
}

func TestRecoveryWithLoggerLogsRecoveredPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := appLog.NewRootLogger(appLog.Options{Level: "info", Format: "json"}, appLog.AppMeta{Service: "yunxia-backend"}, &buf, &buf)

	r := gin.New()
	r.Use(RequestID(), AccessLog(logger, "/dav", nil), RecoveryWithLogger(logger))
	r.GET("/boom", func(c *gin.Context) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"event":"http.request.recovered"`)) {
		t.Fatalf("expected recovered log, got %s", buf.String())
	}
}
