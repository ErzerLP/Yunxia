package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"yunxia/internal/infrastructure/observability/logging"
)

// RecoveryWithLogger 恢复 panic 并输出结构化日志。
func RecoveryWithLogger(base *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger := logging.FromContext(c.Request.Context(), base)
				logger.Error("panic recovered",
					slog.String("event", "http.request.recovered"),
					slog.String("request_id", c.GetString("request_id")),
					slog.Any("panic", recovered),
				)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}
