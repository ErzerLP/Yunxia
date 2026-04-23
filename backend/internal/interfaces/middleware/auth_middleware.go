package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"yunxia/internal/domain/permission"
	"yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/observability/logging"
	"yunxia/internal/infrastructure/security"
	httpresp "yunxia/internal/interfaces/http/response"
)

// AuthMiddleware 负责 Bearer Token 鉴权。
type AuthMiddleware struct {
	userRepo repository.UserRepository
	tokens   interface {
		ValidateAccessToken(token string) (*security.Claims, error)
	}
}

// NewAuthMiddleware 创建鉴权中间件。
func NewAuthMiddleware(
	userRepo repository.UserRepository,
	tokens interface {
		ValidateAccessToken(token string) (*security.Claims, error)
	},
) *AuthMiddleware {
	return &AuthMiddleware{
		userRepo: userRepo,
		tokens:   tokens,
	}
}

// RequireAuth 要求请求带有效访问令牌。
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		rawAuth := c.GetHeader("Authorization")
		if rawAuth == "" || !strings.HasPrefix(rawAuth, "Bearer ") {
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_MISSING", "missing bearer token", nil)
			c.Abort()
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(rawAuth, "Bearer "))
		claims, err := m.tokens.ValidateAccessToken(token)
		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_EXPIRED", "access token expired", nil)
			} else {
				httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "invalid access token", nil)
			}
			c.Abort()
			return
		}

		user, err := m.userRepo.FindByID(c.Request.Context(), claims.UserID)
		if err != nil {
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "invalid access token", nil)
			c.Abort()
			return
		}
		if user.TokenVersion != claims.TokenVersion {
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "token version mismatch", nil)
			c.Abort()
			return
		}
		if user.Status == permission.StatusLocked {
			httpresp.Error(c, http.StatusForbidden, "AUTH_ACCOUNT_LOCKED", "account locked", nil)
			c.Abort()
			return
		}
		capabilities, err := permission.ResolveCapabilities(user.RoleKey)
		if err != nil {
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "invalid role_key", nil)
			c.Abort()
			return
		}
		auth := security.RequestAuth{
			UserID:       user.ID,
			Username:     user.Username,
			RoleKey:      user.RoleKey,
			Status:       user.Status,
			Capabilities: capabilities,
		}
		requestContext := security.WithRequestAuth(c.Request.Context(), auth)
		requestLogger := logging.FromContext(requestContext, nil)
		requestLogger = requestLogger.With(
			slog.Uint64("actor_user_id", uint64(user.ID)),
			slog.String("actor_username", user.Username),
			slog.String("actor_role_key", user.RoleKey),
		)
		c.Request = c.Request.WithContext(logging.WithLogger(requestContext, requestLogger))
		c.Set("user_id", user.ID)
		c.Set("request_auth", auth)
		c.Next()
	}
}
