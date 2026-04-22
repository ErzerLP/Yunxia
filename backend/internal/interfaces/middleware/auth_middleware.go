package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"yunxia/internal/domain/repository"
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
		if user.IsLocked {
			httpresp.Error(c, http.StatusForbidden, "AUTH_ACCOUNT_LOCKED", "account locked", nil)
			c.Abort()
			return
		}

		c.Request = c.Request.WithContext(security.WithRequestAuth(c.Request.Context(), security.RequestAuth{
			UserID: user.ID,
			Role:   user.Role,
		}))
		c.Set("user_id", user.ID)
		c.Set("user_role", user.Role)
		c.Next()
	}
}

// RequireAdmin 要求当前用户为管理员。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("user_role")
		if role != "admin" {
			httpresp.Error(c, http.StatusForbidden, "ROLE_FORBIDDEN", "admin role required", nil)
			c.Abort()
			return
		}

		c.Next()
	}
}
