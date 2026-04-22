package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/golang-jwt/jwt/v5"
)

// TokenType 表示令牌类型。
type TokenType string

const (
	// TokenTypeAccess 表示访问令牌。
	TokenTypeAccess TokenType = "access"
	// TokenTypeRefresh 表示刷新令牌。
	TokenTypeRefresh TokenType = "refresh"
)

var errUnexpectedTokenType = errors.New("unexpected token type")

// Claims 表示 Yunxia JWT Claims。
type Claims struct {
	UserID       uint      `json:"user_id"`
	Role         string    `json:"role"`
	TokenVersion int       `json:"token_version"`
	TokenType    TokenType `json:"token_type"`
	jwt.RegisteredClaims
}

// JWTTokenService 负责签发和校验 JWT。
type JWTTokenService struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

// NewJWTTokenService 创建 JWT 服务。
func NewJWTTokenService(secret string, accessTTL, refreshTTL time.Duration) *JWTTokenService {
	return &JWTTokenService{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		now:        time.Now,
	}
}

// IssueAccessToken 签发访问令牌。
func (s *JWTTokenService) IssueAccessToken(userID uint, role string, tokenVersion int) (string, error) {
	return s.issueToken(userID, role, tokenVersion, TokenTypeAccess, s.accessTTL)
}

// IssueRefreshToken 签发刷新令牌。
func (s *JWTTokenService) IssueRefreshToken(userID uint, role string, tokenVersion int) (string, error) {
	return s.issueToken(userID, role, tokenVersion, TokenTypeRefresh, s.refreshTTL)
}

// ValidateAccessToken 校验访问令牌。
func (s *JWTTokenService) ValidateAccessToken(token string) (*Claims, error) {
	return s.validateToken(token, TokenTypeAccess)
}

// ValidateRefreshToken 校验刷新令牌。
func (s *JWTTokenService) ValidateRefreshToken(token string) (*Claims, error) {
	return s.validateToken(token, TokenTypeRefresh)
}

// AccessTokenTTL 返回访问令牌 TTL。
func (s *JWTTokenService) AccessTokenTTL() time.Duration {
	return s.accessTTL
}

// RefreshTokenTTL 返回刷新令牌 TTL。
func (s *JWTTokenService) RefreshTokenTTL() time.Duration {
	return s.refreshTTL
}

func (s *JWTTokenService) issueToken(userID uint, role string, tokenVersion int, tokenType TokenType, ttl time.Duration) (string, error) {
	now := s.now()
	claims := Claims{
		UserID:       userID,
		Role:         role,
		TokenVersion: tokenVersion,
		TokenType:    tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Subject:   fmt.Sprintf("%d", userID),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *JWTTokenService) validateToken(raw string, expected TokenType) (*Claims, error) {
	token, err := jwt.ParseWithClaims(raw, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("invalid claims")
	}
	if claims.TokenType != expected {
		return nil, errUnexpectedTokenType
	}

	return claims, nil
}
