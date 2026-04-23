package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	appaudit "yunxia/internal/application/audit"
	"yunxia/internal/infrastructure/observability/logging"
	"yunxia/internal/infrastructure/security"
)

// AccessLog 记录请求级 access log，并注入 request logger。
func AccessLog(base *slog.Logger, webdavPrefix string, skipPaths map[string]struct{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, skipped := skipPaths[c.Request.URL.Path]; skipped {
			c.Next()
			return
		}

		startedAt := time.Now()
		entryPoint := detectEntryPoint(c.Request.URL.Path, webdavPrefix)
		requestLogger := logging.Component(base, "http.request").With(
			slog.String("request_id", c.GetString("request_id")),
			slog.String("entrypoint", entryPoint),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.String("client_ip", c.ClientIP()),
			slog.String("user_agent", c.Request.UserAgent()),
		)
		requestContext := appaudit.WithRequestContext(c.Request.Context(), appaudit.RequestContext{
			RequestID:  c.GetString("request_id"),
			EntryPoint: appaudit.EntryPoint(entryPoint),
			ClientIP:   c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
			Method:     c.Request.Method,
			Path:       c.Request.URL.Path,
		})
		c.Request = c.Request.WithContext(logging.WithLogger(requestContext, requestLogger))

		c.Next()

		auth, _ := security.RequestAuthFromContext(c.Request.Context())
		logger := logging.FromContext(c.Request.Context(), requestLogger)
		level := statusToLevel(c.Writer.Status())
		attrs := []slog.Attr{
			slog.String("event", "http.request.completed"),
			slog.String("entrypoint", entryPoint),
			slog.String("route", c.FullPath()),
			slog.Int("status", c.Writer.Status()),
			slog.Int64("latency_ms", time.Since(startedAt).Milliseconds()),
			slog.Int("response_bytes", c.Writer.Size()),
			slog.String("error_code", c.GetString("response_code")),
		}
		if auth.UserID > 0 {
			attrs = append(attrs, slog.Uint64("actor_user_id", uint64(auth.UserID)))
		}
		if auth.RoleKey != "" {
			attrs = append(attrs, slog.String("actor_role_key", auth.RoleKey))
		}
		logger.LogAttrs(c.Request.Context(), level, "request completed", attrs...)
	}
}

func detectEntryPoint(requestPath string, webdavPrefix string) string {
	switch {
	case strings.HasPrefix(requestPath, "/api/v2"):
		return string(appaudit.EntryPointRESTV2)
	case webdavPrefix != "" && strings.HasPrefix(requestPath, strings.TrimRight(webdavPrefix, "/")):
		return string(appaudit.EntryPointWebDAV)
	default:
		return string(appaudit.EntryPointRESTV1)
	}
}

func statusToLevel(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
