package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"yunxia/internal/domain/permission"
	"yunxia/internal/infrastructure/security"
	httpresp "yunxia/internal/interfaces/http/response"
)

// RequireCapability 要求当前请求具备指定 capability。
func RequireCapability(capability string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth, ok := security.RequestAuthFromContext(c.Request.Context())
		if !ok {
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "missing auth context", nil)
			c.Abort()
			return
		}
		if !permission.HasCapability(auth.Capabilities, capability) {
			httpresp.Error(c, http.StatusForbidden, "CAPABILITY_DENIED", "capability denied", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}
