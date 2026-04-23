package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	appaudit "yunxia/internal/application/audit"
	"yunxia/internal/domain/permission"
	"yunxia/internal/infrastructure/security"
	httpresp "yunxia/internal/interfaces/http/response"
)

// RequireCapabilitiesForAction 要求请求具备指定能力，并在拒绝时记录审计。
func RequireCapabilitiesForAction(
	recorder *appaudit.Recorder,
	logger *slog.Logger,
	resourceType string,
	action string,
	capabilities ...string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth, ok := security.RequestAuthFromContext(c.Request.Context())
		if !ok {
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "missing auth context", nil)
			c.Abort()
			return
		}
		for _, capability := range capabilities {
			if !permission.HasCapability(auth.Capabilities, capability) {
				appaudit.RecordBestEffort(c.Request.Context(), recorder, logger, appaudit.Event{
					ResourceType: resourceType,
					Action:       action,
					Result:       appaudit.ResultDenied,
					ErrorCode:    "CAPABILITY_DENIED",
				})
				httpresp.Error(c, http.StatusForbidden, "CAPABILITY_DENIED", "capability denied", nil)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
