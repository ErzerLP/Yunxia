package security

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// FileAccessClaims 表示短时文件访问令牌 claims。
type FileAccessClaims struct {
	SourceID    uint   `json:"source_id"`
	Path        string `json:"path"`
	Purpose     string `json:"purpose"`
	Disposition string `json:"disposition"`
	TokenType   string `json:"token_type"`
	jwt.RegisteredClaims
}

// FileAccessTokenService 负责签发和校验短时文件访问令牌。
type FileAccessTokenService struct {
	secret []byte
	now    func() time.Time
}

// NewFileAccessTokenService 创建文件访问令牌服务。
func NewFileAccessTokenService(secret string) *FileAccessTokenService {
	return &FileAccessTokenService{
		secret: []byte(secret),
		now:    time.Now,
	}
}

// Issue 签发短时文件访问令牌。
func (s *FileAccessTokenService) Issue(sourceID uint, path, purpose, disposition string, ttl time.Duration) (string, time.Time, error) {
	now := s.now()
	expiresAt := now.Add(ttl)
	claims := FileAccessClaims{
		SourceID:    sourceID,
		Path:        path,
		Purpose:     purpose,
		Disposition: disposition,
		TokenType:   "file_access",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

// Validate 校验短时文件访问令牌。
func (s *FileAccessTokenService) Validate(raw string) (*FileAccessClaims, error) {
	token, err := jwt.ParseWithClaims(raw, &FileAccessClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*FileAccessClaims)
	if !ok || claims.TokenType != "file_access" {
		return nil, errors.New("invalid file access token")
	}
	return claims, nil
}
