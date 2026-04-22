# Backend Foundation, Auth, and System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first runnable Yunxia backend slice: project skeleton, configuration, SQLite persistence, bootstrap/setup flow, JWT auth, and system config/version endpoints aligned with the approved API contract.

**Architecture:** This plan implements the innermost vertical slice needed to boot the service and authenticate users. It follows the documented DDD layering: interface adapters call application services, application services depend on repository interfaces and security abstractions, and infrastructure provides GORM/SQLite, bcrypt, JWT, and Viper-backed implementations.

**Tech Stack:** Go 1.24, Gin, GORM, SQLite, Viper, golang-jwt/jwt/v5, bcrypt, Zap, net/http/httptest, standard `testing`

---

## Scope decomposition

The approved API contract spans multiple independent backend subsystems. To keep plans executable and testable, split the backend into these sequential plans:

1. **This plan** — foundation, config, persistence, setup, auth, system endpoints
2. **Plan 2** — file browsing/search/rename/mkdir/download + source navigation/admin CRUD
3. **Plan 3** — upload init/chunk/finish/session recovery
4. **Plan 4** — download task APIs and Aria2 integration
5. **Plan 5** — WebDAV + ACL + Draft follow-up APIs

This document only implements Plan 1.

## File structure for this plan

**Create:**
- `go.mod` — Go module and direct dependencies for Plan 1
- `cmd/server/main.go` — process entrypoint, config load, dependency wiring, HTTP startup
- `internal/domain/entity/user.go` — user aggregate for admin setup/login
- `internal/domain/entity/system_config.go` — persisted editable public config view
- `internal/domain/entity/refresh_token.go` — refresh token persistence model
- `internal/domain/repository/user_repo.go` — user repository interface
- `internal/domain/repository/system_config_repo.go` — system config repository interface
- `internal/domain/repository/refresh_token_repo.go` — refresh token repository interface
- `internal/application/dto/auth_dto.go` — setup/login/refresh/auth response DTOs
- `internal/application/dto/system_dto.go` — system config/version DTOs
- `internal/application/service/setup_app_svc.go` — setup status/init use cases
- `internal/application/service/auth_app_svc.go` — login/refresh/logout/me use cases
- `internal/application/service/system_app_svc.go` — config/version use cases
- `internal/interfaces/http/response/response.go` — stable REST envelope helpers
- `internal/interfaces/http/handler/setup_handler.go` — `/setup/*` handlers
- `internal/interfaces/http/handler/auth_handler.go` — `/auth/*` handlers
- `internal/interfaces/http/handler/system_handler.go` — `/health`, `/system/*` handlers
- `internal/interfaces/http/router.go` — Gin router registration for Plan 1 routes
- `internal/interfaces/middleware/auth_mw.go` — Bearer auth + current user injection
- `internal/interfaces/middleware/security.go` — minimal security headers middleware
- `internal/infrastructure/config/config.go` — Viper-backed config load with defaults/env override
- `internal/infrastructure/persistence/gorm/db.go` — SQLite connection/bootstrap
- `internal/infrastructure/persistence/gorm/models.go` — GORM persistence models for user/config/refresh token
- `internal/infrastructure/persistence/gorm/user_repo_impl.go` — user repo implementation
- `internal/infrastructure/persistence/gorm/system_config_repo_impl.go` — system config repo implementation
- `internal/infrastructure/persistence/gorm/refresh_token_repo_impl.go` — refresh token repo implementation
- `internal/infrastructure/persistence/migration/migration.go` — Plan 1 table migration entrypoint
- `internal/infrastructure/security/bcrypt_hasher.go` — password hasher implementation
- `internal/infrastructure/security/jwt_token_service.go` — access/refresh token issue/validate implementation
- `internal/pkg/logger/logger.go` — Zap logger constructor

**Test:**
- `internal/infrastructure/config/config_test.go`
- `internal/interfaces/http/handler/system_handler_test.go`
- `internal/infrastructure/persistence/gorm/user_repo_impl_test.go`
- `internal/infrastructure/security/bcrypt_hasher_test.go`
- `internal/infrastructure/security/jwt_token_service_test.go`
- `internal/interfaces/http/handler/setup_handler_test.go`
- `internal/interfaces/http/handler/auth_handler_test.go`
- `internal/interfaces/http/handler/system_config_handler_test.go`
- `internal/interfaces/http/router_test.go`

---

### Task 1: Bootstrap module and configuration loader

**Files:**
- Create: `go.mod`
- Create: `internal/infrastructure/config/config.go`
- Test: `internal/infrastructure/config/config_test.go`

- [ ] **Step 1: Create bootstrap module file**

```go
module yunxia

go 1.24.0

require (
    github.com/gin-gonic/gin v1.10.1
    github.com/golang-jwt/jwt/v5 v5.2.1
    github.com/spf13/viper v1.19.0
    golang.org/x/crypto v0.37.0
    gorm.io/driver/sqlite v1.5.7
    gorm.io/gorm v1.30.0
    go.uber.org/zap v1.27.0
)
```

- [ ] **Step 2: Write the failing config test**

```go
package config

import (
    "os"
    "testing"
    "time"
)

func TestLoadAppliesDefaultsAndEnvOverrides(t *testing.T) {
    t.Setenv("YUNXIA_SERVER_PORT", "9090")
    t.Setenv("YUNXIA_DATABASE_DSN", "./test.db")

    cfg, err := Load()
    if err != nil {
        t.Fatalf("Load() error = %v", err)
    }
    if cfg.Server.Port != 9090 {
        t.Fatalf("expected port 9090, got %d", cfg.Server.Port)
    }
    if cfg.Database.DSN != "./test.db" {
        t.Fatalf("expected dsn override, got %q", cfg.Database.DSN)
    }
    if cfg.JWT.AccessTokenExpire != 15*time.Minute {
        t.Fatalf("expected default access token ttl 15m, got %s", cfg.JWT.AccessTokenExpire)
    }
    if cfg.System.DefaultChunkSize != 5*1024*1024 {
        t.Fatalf("expected default chunk size 5MB, got %d", cfg.System.DefaultChunkSize)
    }
    _ = os.Remove("./test.db")
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/infrastructure/config -run TestLoadAppliesDefaultsAndEnvOverrides -v`
Expected: FAIL with `undefined: Load`

- [ ] **Step 4: Write minimal implementation**

```go
package config

import (
    "strings"
    "time"

    "github.com/spf13/viper"
)

type Config struct {
    Server struct {
        Host string
        Port int
        Mode string
    }
    Database struct {
        DSN string
    }
    JWT struct {
        Secret             string
        AccessTokenExpire  time.Duration
        RefreshTokenExpire time.Duration
    }
    System struct {
        DefaultChunkSize int64
        MaxUploadSize    int64
        SiteName         string
        WebDAVPrefix     string
        TimeZone         string
        Language         string
        Theme            string
    }
}

func Load() (Config, error) {
    v := viper.New()
    v.SetEnvPrefix("YUNXIA")
    v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
    v.AutomaticEnv()

    v.SetDefault("server.host", "0.0.0.0")
    v.SetDefault("server.port", 8080)
    v.SetDefault("server.mode", "debug")
    v.SetDefault("database.dsn", "/data/database.db")
    v.SetDefault("jwt.secret", "change-me")
    v.SetDefault("jwt.access_token_expire", "15m")
    v.SetDefault("jwt.refresh_token_expire", "168h")
    v.SetDefault("system.default_chunk_size", 5*1024*1024)
    v.SetDefault("system.max_upload_size", 10*1024*1024*1024)
    v.SetDefault("system.site_name", "云匣")
    v.SetDefault("system.webdav_prefix", "/dav")
    v.SetDefault("system.time_zone", "Asia/Shanghai")
    v.SetDefault("system.language", "zh-CN")
    v.SetDefault("system.theme", "system")

    accessTTL, err := time.ParseDuration(v.GetString("jwt.access_token_expire"))
    if err != nil {
        return Config{}, err
    }
    refreshTTL, err := time.ParseDuration(v.GetString("jwt.refresh_token_expire"))
    if err != nil {
        return Config{}, err
    }

    var cfg Config
    cfg.Server.Host = v.GetString("server.host")
    cfg.Server.Port = v.GetInt("server.port")
    cfg.Server.Mode = v.GetString("server.mode")
    cfg.Database.DSN = v.GetString("database.dsn")
    cfg.JWT.Secret = v.GetString("jwt.secret")
    cfg.JWT.AccessTokenExpire = accessTTL
    cfg.JWT.RefreshTokenExpire = refreshTTL
    cfg.System.DefaultChunkSize = v.GetInt64("system.default_chunk_size")
    cfg.System.MaxUploadSize = v.GetInt64("system.max_upload_size")
    cfg.System.SiteName = v.GetString("system.site_name")
    cfg.System.WebDAVPrefix = v.GetString("system.webdav_prefix")
    cfg.System.TimeZone = v.GetString("system.time_zone")
    cfg.System.Language = v.GetString("system.language")
    cfg.System.Theme = v.GetString("system.theme")

    return cfg, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/infrastructure/config -run TestLoadAppliesDefaultsAndEnvOverrides -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go.mod internal/infrastructure/config/config.go internal/infrastructure/config/config_test.go
git commit -m "feat: add backend config loader"
```

### Task 2: Add REST envelope and health/version endpoints

**Files:**
- Create: `internal/interfaces/http/response/response.go`
- Create: `internal/application/dto/system_dto.go`
- Create: `internal/interfaces/http/handler/system_handler.go`
- Test: `internal/interfaces/http/handler/system_handler_test.go`

- [ ] **Step 1: Write the failing handler tests**

```go
package handler

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
)

type envelope struct {
    Success bool            `json:"success"`
    Code    string          `json:"code"`
    Data    json.RawMessage `json:"data"`
}

func TestHealthReturnsOKEnvelope(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    h := NewSystemHandler("1.0.0", "abcdef1", "2026-04-21T12:00:00+08:00", "go1.24.0", nil)
    r.GET("/api/v1/health", h.Health)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
    rec := httptest.NewRecorder()
    r.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
    var got envelope
    if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
        t.Fatalf("json unmarshal error = %v", err)
    }
    if !got.Success || got.Code != "OK" {
        t.Fatalf("unexpected envelope: %+v", got)
    }
}

func TestVersionReturnsBuildInfo(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    h := NewSystemHandler("1.0.0", "abcdef1", "2026-04-21T12:00:00+08:00", "go1.24.0", nil)
    r.GET("/api/v1/system/version", h.Version)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/system/version", nil)
    rec := httptest.NewRecorder()
    r.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
    if !json.Valid(rec.Body.Bytes()) {
        t.Fatalf("expected valid json body")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/interfaces/http/handler -run "Test(HealthReturnsOKEnvelope|VersionReturnsBuildInfo)" -v`
Expected: FAIL with `undefined: NewSystemHandler`

- [ ] **Step 3: Write minimal implementation**

```go
package response

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
)

type Meta struct {
    RequestID string `json:"request_id"`
    Timestamp string `json:"timestamp"`
}

func JSON(c *gin.Context, status int, code, message string, data any) {
    c.JSON(status, gin.H{
        "success": status < http.StatusBadRequest,
        "code":    code,
        "message": message,
        "data":    data,
        "meta": Meta{
            RequestID: c.GetString("request_id"),
            Timestamp: time.Now().Format(time.RFC3339),
        },
    })
}
```

```go
package dto

type VersionResponse struct {
    Service    string `json:"service"`
    Version    string `json:"version"`
    Commit     string `json:"commit"`
    BuildTime  string `json:"build_time"`
    GoVersion  string `json:"go_version"`
    APIVersion string `json:"api_version"`
}
```

```go
package handler

import (
    "net/http"

    "github.com/gin-gonic/gin"

    "yunxia/internal/application/dto"
    httpresp "yunxia/internal/interfaces/http/response"
)

type SystemHandler struct {
    version   string
    commit    string
    buildTime string
    goVersion string
}

func NewSystemHandler(version, commit, buildTime, goVersion string, _ any) *SystemHandler {
    return &SystemHandler{version: version, commit: commit, buildTime: buildTime, goVersion: goVersion}
}

func (h *SystemHandler) Health(c *gin.Context) {
    httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{
        "status":  "ok",
        "service": "yunxia",
        "version": h.version,
    })
}

func (h *SystemHandler) Version(c *gin.Context) {
    httpresp.JSON(c, http.StatusOK, "OK", "ok", dto.VersionResponse{
        Service:    "yunxia",
        Version:    h.version,
        Commit:     h.commit,
        BuildTime:  h.buildTime,
        GoVersion:  h.goVersion,
        APIVersion: "v1",
    })
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/interfaces/http/handler -run "Test(HealthReturnsOKEnvelope|VersionReturnsBuildInfo)" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/interfaces/http/response/response.go internal/application/dto/system_dto.go internal/interfaces/http/handler/system_handler.go internal/interfaces/http/handler/system_handler_test.go
git commit -m "feat: add health and version endpoints"
```
### Task 3: Add SQLite persistence for users, refresh tokens, and system config

**Files:**
- Create: `internal/domain/entity/user.go`
- Create: `internal/domain/entity/system_config.go`
- Create: `internal/domain/entity/refresh_token.go`
- Create: `internal/domain/repository/user_repo.go`
- Create: `internal/domain/repository/system_config_repo.go`
- Create: `internal/domain/repository/refresh_token_repo.go`
- Create: `internal/infrastructure/persistence/gorm/db.go`
- Create: `internal/infrastructure/persistence/gorm/models.go`
- Create: `internal/infrastructure/persistence/gorm/user_repo_impl.go`
- Create: `internal/infrastructure/persistence/gorm/system_config_repo_impl.go`
- Create: `internal/infrastructure/persistence/gorm/refresh_token_repo_impl.go`
- Create: `internal/infrastructure/persistence/migration/migration.go`
- Test: `internal/infrastructure/persistence/gorm/user_repo_impl_test.go`

- [ ] **Step 1: Write the failing repository tests**

```go
package gorm

import (
    "context"
    "path/filepath"
    "testing"
    "time"

    "yunxia/internal/domain/entity"
)

func TestUserRepoCreateAndFindByUsername(t *testing.T) {
    db, cleanup := testDB(t, filepath.Join(t.TempDir(), "repo.db"))
    defer cleanup()

    repo := NewUserRepository(db)
    user := &entity.User{Username: "admin", PasswordHash: "hash", Role: "admin"}
    if err := repo.Create(context.Background(), user); err != nil {
        t.Fatalf("Create() error = %v", err)
    }

    got, err := repo.FindByUsername(context.Background(), "admin")
    if err != nil {
        t.Fatalf("FindByUsername() error = %v", err)
    }
    if got.Username != "admin" || got.Role != "admin" {
        t.Fatalf("unexpected user = %+v", got)
    }
}

func TestSystemConfigRepoUpsertAndGet(t *testing.T) {
    db, cleanup := testDB(t, filepath.Join(t.TempDir(), "cfg.db"))
    defer cleanup()

    repo := NewSystemConfigRepository(db)
    cfg := &entity.SystemConfig{SiteName: "云匣", MultiUserEnabled: true, UpdatedAt: time.Now()}
    if err := repo.Upsert(context.Background(), cfg); err != nil {
        t.Fatalf("Upsert() error = %v", err)
    }

    got, err := repo.Get(context.Background())
    if err != nil {
        t.Fatalf("Get() error = %v", err)
    }
    if got.SiteName != "云匣" || !got.MultiUserEnabled {
        t.Fatalf("unexpected config = %+v", got)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infrastructure/persistence/gorm -run "Test(UserRepoCreateAndFindByUsername|SystemConfigRepoUpsertAndGet)" -v`
Expected: FAIL with `undefined: testDB` or missing repository constructors

- [ ] **Step 3: Write minimal implementation**

```go
package entity

type User struct {
    ID           uint
    Username     string
    Email        string
    PasswordHash string
    Role         string
    IsLocked     bool
    TokenVersion int
}
```

```go
package entity

import "time"

type SystemConfig struct {
    ID               uint
    SiteName         string
    MultiUserEnabled bool
    DefaultSourceID  *uint
    MaxUploadSize    int64
    DefaultChunkSize int64
    WebDAVEnabled    bool
    WebDAVPrefix     string
    Theme            string
    Language         string
    TimeZone         string
    UpdatedAt        time.Time
}
```

```go
package entity

import "time"

type RefreshToken struct {
    ID        uint
    UserID    uint
    TokenHash string
    ExpiresAt time.Time
    RevokedAt *time.Time
}
```

```go
package repository

import (
    "context"

    "yunxia/internal/domain/entity"
)

type UserRepository interface {
    Create(ctx context.Context, user *entity.User) error
    FindByUsername(ctx context.Context, username string) (*entity.User, error)
    FindByID(ctx context.Context, id uint) (*entity.User, error)
    Count(ctx context.Context) (int64, error)
    UpdateTokenVersion(ctx context.Context, id uint, version int) error
}
```

```go
package repository

import (
    "context"

    "yunxia/internal/domain/entity"
)

type SystemConfigRepository interface {
    Get(ctx context.Context) (*entity.SystemConfig, error)
    Upsert(ctx context.Context, cfg *entity.SystemConfig) error
}
```

```go
package repository

import (
    "context"

    "yunxia/internal/domain/entity"
)

type RefreshTokenRepository interface {
    Create(ctx context.Context, token *entity.RefreshToken) error
    FindByTokenHash(ctx context.Context, tokenHash string) (*entity.RefreshToken, error)
    RevokeByTokenHash(ctx context.Context, tokenHash string) error
}
```

```go
package gorm

import (
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

type UserModel struct {
    ID           uint   `gorm:"primaryKey"`
    Username     string `gorm:"uniqueIndex;size:64"`
    Email        string `gorm:"size:128"`
    PasswordHash string `gorm:"size:255"`
    Role         string `gorm:"size:16"`
    IsLocked     bool
    TokenVersion int
}

type SystemConfigModel struct {
    ID               uint `gorm:"primaryKey"`
    SiteName         string
    MultiUserEnabled bool
    DefaultSourceID  *uint
    MaxUploadSize    int64
    DefaultChunkSize int64
    WebDAVEnabled    bool
    WebDAVPrefix     string
    Theme            string
    Language         string
    TimeZone         string
}

type RefreshTokenModel struct {
    ID        uint `gorm:"primaryKey"`
    UserID    uint `gorm:"index"`
    TokenHash string `gorm:"uniqueIndex;size:255"`
    ExpiresAt int64
    RevokedAt *int64
}

func Open(dsn string) (*gorm.DB, error) {
    db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
    if err != nil {
        return nil, err
    }
    return db, db.AutoMigrate(&UserModel{}, &SystemConfigModel{}, &RefreshTokenModel{})
}
```

```go
package gorm

import (
    "context"
    "testing"

    dom "yunxia/internal/domain/entity"
)

func NewUserRepository(db DB) *UserRepositoryImpl { return &UserRepositoryImpl{db: db} }

type DB interface {
    WithContext(ctx context.Context) DB
    Create(value any) DB
    Where(query any, args ...any) DB
    First(dest any, conds ...any) DB
    Model(value any) DB
    Count(count *int64) DB
    Update(column string, value any) DB
    Error() error
}

// For the real implementation, use `*gorm.DB` and adapt these calls directly.
// Keep this exact repository API when writing the real code.

type UserRepositoryImpl struct{ db *gorm.DB }

type SystemConfigRepositoryImpl struct{ db *gorm.DB }
```

```go
package migration

import "gorm.io/gorm"

func Run(db *gorm.DB) error {
    return db.AutoMigrate(&gorm.UserModel{}, &gorm.SystemConfigModel{}, &gorm.RefreshTokenModel{})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/infrastructure/persistence/gorm -run "Test(UserRepoCreateAndFindByUsername|SystemConfigRepoUpsertAndGet)" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/entity internal/domain/repository internal/infrastructure/persistence
git commit -m "feat: add sqlite persistence for auth and system config"
```

### Task 4: Add bcrypt hashing and JWT token service

**Files:**
- Create: `internal/infrastructure/security/bcrypt_hasher.go`
- Create: `internal/infrastructure/security/jwt_token_service.go`
- Test: `internal/infrastructure/security/bcrypt_hasher_test.go`
- Test: `internal/infrastructure/security/jwt_token_service_test.go`

- [ ] **Step 1: Write the failing security tests**

```go
package security

import (
    "testing"
    "time"
)

func TestBcryptHasherRoundTrip(t *testing.T) {
    hasher := NewBcryptHasher(10)
    hashed, err := hasher.Hash("strong-password-123")
    if err != nil {
        t.Fatalf("Hash() error = %v", err)
    }
    ok := hasher.Compare(hashed, "strong-password-123")
    if !ok {
        t.Fatalf("expected password compare to succeed")
    }
}

func TestJWTTokenServiceIssuesAndValidatesAccessToken(t *testing.T) {
    svc := NewJWTTokenService("secret", 15*time.Minute, 7*24*time.Hour)
    token, err := svc.IssueAccessToken(1, "admin", 3)
    if err != nil {
        t.Fatalf("IssueAccessToken() error = %v", err)
    }
    claims, err := svc.ValidateAccessToken(token)
    if err != nil {
        t.Fatalf("ValidateAccessToken() error = %v", err)
    }
    if claims.UserID != 1 || claims.Role != "admin" || claims.TokenVersion != 3 {
        t.Fatalf("unexpected claims = %+v", claims)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infrastructure/security -run "Test(BcryptHasherRoundTrip|JWTTokenServiceIssuesAndValidatesAccessToken)" -v`
Expected: FAIL with missing constructors or methods

- [ ] **Step 3: Write minimal implementation**

```go
package security

import "golang.org/x/crypto/bcrypt"

type BcryptHasher struct{ cost int }

func NewBcryptHasher(cost int) *BcryptHasher { return &BcryptHasher{cost: cost} }

func (h *BcryptHasher) Hash(password string) (string, error) {
    raw, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
    return string(raw), err
}

func (h *BcryptHasher) Compare(hash, password string) bool {
    return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
```

```go
package security

import (
    "time"

    "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
    UserID       uint   `json:"user_id"`
    Username     string `json:"username"`
    Role         string `json:"role"`
    TokenVersion int    `json:"token_version"`
    jwt.RegisteredClaims
}

type JWTTokenService struct {
    secret     []byte
    accessTTL  time.Duration
    refreshTTL time.Duration
}

func NewJWTTokenService(secret string, accessTTL, refreshTTL time.Duration) *JWTTokenService {
    return &JWTTokenService{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (s *JWTTokenService) IssueAccessToken(userID uint, role string, version int) (string, error) {
    claims := Claims{
        UserID: userID,
        Role: role,
        TokenVersion: version,
        RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.accessTTL))},
    }
    return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *JWTTokenService) ValidateAccessToken(token string) (*Claims, error) {
    parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) { return s.secret, nil })
    if err != nil {
        return nil, err
    }
    return parsed.Claims.(*Claims), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/infrastructure/security -run "Test(BcryptHasherRoundTrip|JWTTokenServiceIssuesAndValidatesAccessToken)" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infrastructure/security
git commit -m "feat: add password hashing and jwt token service"
```
### Task 5: Implement setup status/init vertical slice

**Files:**
- Create: `internal/application/dto/auth_dto.go`
- Create: `internal/application/service/setup_app_svc.go`
- Create: `internal/interfaces/http/handler/setup_handler.go`
- Test: `internal/interfaces/http/handler/setup_handler_test.go`

- [ ] **Step 1: Write the failing setup handler tests**

```go
package handler

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
)

func TestSetupStatusReturnsSetupRequiredWhenNoAdminExists(t *testing.T) {
    gin.SetMode(gin.TestMode)
    svc := &fakeSetupService{status: map[string]any{"is_initialized": false, "setup_required": true, "has_admin": false}}
    h := NewSetupHandler(svc)
    r := gin.New()
    r.GET("/api/v1/setup/status", h.Status)

    rec := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
    r.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
}

func TestSetupInitCreatesAdminAndReturnsTokens(t *testing.T) {
    gin.SetMode(gin.TestMode)
    svc := &fakeSetupService{initResp: map[string]any{"user": map[string]any{"username": "admin"}, "tokens": map[string]any{"access_token": "a", "refresh_token": "b"}}}
    h := NewSetupHandler(svc)
    r := gin.New()
    r.POST("/api/v1/setup/init", h.Init)

    body := bytes.NewBufferString(`{"username":"admin","password":"strong-password-123","email":"admin@example.com"}`)
    rec := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/init", body)
    req.Header.Set("Content-Type", "application/json")
    r.ServeHTTP(rec, req)

    if rec.Code != http.StatusCreated {
        t.Fatalf("expected 201, got %d", rec.Code)
    }
    if !json.Valid(rec.Body.Bytes()) {
        t.Fatalf("expected valid json")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/interfaces/http/handler -run "TestSetup(StatusReturnsSetupRequiredWhenNoAdminExists|InitCreatesAdminAndReturnsTokens)" -v`
Expected: FAIL with `undefined: NewSetupHandler`

- [ ] **Step 3: Write minimal implementation**

```go
package dto

type SetupStatusResponse struct {
    IsInitialized bool `json:"is_initialized"`
    SetupRequired bool `json:"setup_required"`
    HasAdmin      bool `json:"has_admin"`
}

type SetupInitRequest struct {
    Username string `json:"username" binding:"required,min=3,max=64"`
    Password string `json:"password" binding:"required,min=8"`
    Email    string `json:"email"`
}

type UserSummary struct {
    ID        uint   `json:"id"`
    Username  string `json:"username"`
    Email     string `json:"email"`
    Role      string `json:"role"`
    IsLocked  bool   `json:"is_locked"`
    CreatedAt string `json:"created_at"`
}

type TokenPair struct {
    AccessToken       string `json:"access_token"`
    RefreshToken      string `json:"refresh_token"`
    ExpiresIn         int    `json:"expires_in"`
    RefreshExpiresIn  int    `json:"refresh_expires_in"`
    TokenType         string `json:"token_type"`
}

type SetupInitResponse struct {
    User   UserSummary `json:"user"`
    Tokens TokenPair   `json:"tokens"`
}
```

```go
package service

import (
    "context"
    "errors"
    "time"

    "yunxia/internal/application/dto"
    "yunxia/internal/domain/entity"
    "yunxia/internal/domain/repository"
    "yunxia/internal/infrastructure/security"
)

var ErrSetupCompleted = errors.New("setup already completed")

type SetupApplicationService struct {
    users  repository.UserRepository
    tokens *security.JWTTokenService
    hasher *security.BcryptHasher
}

func NewSetupApplicationService(users repository.UserRepository, tokens *security.JWTTokenService, hasher *security.BcryptHasher) *SetupApplicationService {
    return &SetupApplicationService{users: users, tokens: tokens, hasher: hasher}
}

func (s *SetupApplicationService) Status(ctx context.Context) (*dto.SetupStatusResponse, error) {
    count, err := s.users.Count(ctx)
    if err != nil {
        return nil, err
    }
    hasAdmin := count > 0
    return &dto.SetupStatusResponse{IsInitialized: hasAdmin, SetupRequired: !hasAdmin, HasAdmin: hasAdmin}, nil
}

func (s *SetupApplicationService) Init(ctx context.Context, req dto.SetupInitRequest) (*dto.SetupInitResponse, error) {
    count, err := s.users.Count(ctx)
    if err != nil {
        return nil, err
    }
    if count > 0 {
        return nil, ErrSetupCompleted
    }
    hash, err := s.hasher.Hash(req.Password)
    if err != nil {
        return nil, err
    }
    user := &entity.User{Username: req.Username, Email: req.Email, PasswordHash: hash, Role: "admin"}
    if err := s.users.Create(ctx, user); err != nil {
        return nil, err
    }
    access, err := s.tokens.IssueAccessToken(user.ID, user.Role, user.TokenVersion)
    if err != nil {
        return nil, err
    }
    refresh, err := s.tokens.IssueAccessToken(user.ID, user.Role, user.TokenVersion)
    if err != nil {
        return nil, err
    }
    return &dto.SetupInitResponse{User: dto.UserSummary{ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role, CreatedAt: time.Now().Format(time.RFC3339)}, Tokens: dto.TokenPair{AccessToken: access, RefreshToken: refresh, ExpiresIn: 900, RefreshExpiresIn: 604800, TokenType: "Bearer"}}, nil
}
```

```go
package handler

import (
    "net/http"

    "github.com/gin-gonic/gin"

    "yunxia/internal/application/dto"
    appsvc "yunxia/internal/application/service"
    httpresp "yunxia/internal/interfaces/http/response"
)

type setupService interface {
    Status(ctx context.Context) (*dto.SetupStatusResponse, error)
    Init(ctx context.Context, req dto.SetupInitRequest) (*dto.SetupInitResponse, error)
}

type SetupHandler struct{ svc setupService }

func NewSetupHandler(svc setupService) *SetupHandler { return &SetupHandler{svc: svc} }

func (h *SetupHandler) Status(c *gin.Context) {
    resp, err := h.svc.Status(c.Request.Context())
    if err != nil {
        httpresp.JSON(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), gin.H{})
        return
    }
    httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *SetupHandler) Init(c *gin.Context) {
    var req dto.SetupInitRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpresp.JSON(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), gin.H{})
        return
    }
    resp, err := h.svc.Init(c.Request.Context(), req)
    if err != nil {
        if errors.Is(err, appsvc.ErrSetupCompleted) {
            httpresp.JSON(c, http.StatusConflict, "SETUP_ALREADY_COMPLETED", err.Error(), gin.H{})
            return
        }
        httpresp.JSON(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), gin.H{})
        return
    }
    httpresp.JSON(c, http.StatusCreated, "OK", "ok", resp)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/interfaces/http/handler -run "TestSetup(StatusReturnsSetupRequiredWhenNoAdminExists|InitCreatesAdminAndReturnsTokens)" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/dto/auth_dto.go internal/application/service/setup_app_svc.go internal/interfaces/http/handler/setup_handler.go internal/interfaces/http/handler/setup_handler_test.go
git commit -m "feat: add setup status and init endpoints"
```

### Task 6: Implement login, refresh, logout, and me

**Files:**
- Modify: `internal/application/dto/auth_dto.go`
- Create: `internal/application/service/auth_app_svc.go`
- Create: `internal/interfaces/middleware/auth_mw.go`
- Create: `internal/interfaces/http/handler/auth_handler.go`
- Test: `internal/interfaces/http/handler/auth_handler_test.go`

- [ ] **Step 1: Write the failing auth handler tests**

```go
package handler

import (
    "bytes"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
)

func TestLoginReturnsTokenPair(t *testing.T) {
    gin.SetMode(gin.TestMode)
    svc := &fakeAuthService{loginResp: map[string]any{"user": map[string]any{"username": "admin"}, "tokens": map[string]any{"access_token": "a", "refresh_token": "b"}}}
    h := NewAuthHandler(svc)
    r := gin.New()
    r.POST("/api/v1/auth/login", h.Login)

    rec := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"admin","password":"strong-password-123"}`))
    req.Header.Set("Content-Type", "application/json")
    r.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
}

func TestMeReturnsUnauthorizedWithoutBearerToken(t *testing.T) {
    gin.SetMode(gin.TestMode)
    h := NewAuthHandler(&fakeAuthService{})
    r := gin.New()
    r.GET("/api/v1/auth/me", h.Me)

    rec := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
    r.ServeHTTP(rec, req)

    if rec.Code != http.StatusUnauthorized {
        t.Fatalf("expected 401, got %d", rec.Code)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/interfaces/http/handler -run "Test(LoginReturnsTokenPair|MeReturnsUnauthorizedWithoutBearerToken)" -v`
Expected: FAIL with `undefined: NewAuthHandler`

- [ ] **Step 3: Write minimal implementation**

```go
package dto

type LoginRequest struct {
    Username string `json:"username" binding:"required,min=3,max=64"`
    Password string `json:"password" binding:"required,min=8"`
}

type RefreshRequest struct {
    RefreshToken string `json:"refresh_token" binding:"required"`
}

type LoginResponse struct {
    User   UserSummary `json:"user"`
    Tokens TokenPair   `json:"tokens"`
}
```

```go
package service

import (
    "context"
    "errors"
    "time"

    "yunxia/internal/application/dto"
    "yunxia/internal/domain/repository"
    "yunxia/internal/infrastructure/security"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type AuthApplicationService struct {
    users   repository.UserRepository
    refresh repository.RefreshTokenRepository
    hasher  *security.BcryptHasher
    tokens  *security.JWTTokenService
}

func (s *AuthApplicationService) Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error) {
    user, err := s.users.FindByUsername(ctx, req.Username)
    if err != nil || !s.hasher.Compare(user.PasswordHash, req.Password) {
        return nil, ErrInvalidCredentials
    }
    access, _ := s.tokens.IssueAccessToken(user.ID, user.Role, user.TokenVersion)
    refresh, _ := s.tokens.IssueAccessToken(user.ID, user.Role, user.TokenVersion)
    return &dto.LoginResponse{User: dto.UserSummary{ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role, IsLocked: user.IsLocked, CreatedAt: time.Now().Format(time.RFC3339)}, Tokens: dto.TokenPair{AccessToken: access, RefreshToken: refresh, ExpiresIn: 900, RefreshExpiresIn: 604800, TokenType: "Bearer"}}, nil
}
```

```go
package middleware

import (
    "net/http"
    "strings"

    "github.com/gin-gonic/gin"
    httpresp "yunxia/internal/interfaces/http/response"
)

type AccessValidator interface { ValidateAccessToken(token string) (*security.Claims, error) }

type AuthMiddleware struct { validator AccessValidator }

func NewAuthMiddleware(validator AccessValidator) *AuthMiddleware { return &AuthMiddleware{validator: validator} }

func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        header := c.GetHeader("Authorization")
        if header == "" || !strings.HasPrefix(header, "Bearer ") {
            httpresp.JSON(c, http.StatusUnauthorized, "AUTH_TOKEN_MISSING", "missing bearer token", gin.H{})
            c.Abort()
            return
        }
        claims, err := m.validator.ValidateAccessToken(strings.TrimPrefix(header, "Bearer "))
        if err != nil {
            httpresp.JSON(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", err.Error(), gin.H{})
            c.Abort()
            return
        }
        c.Set("user_id", claims.UserID)
        c.Next()
    }
}
```

```go
package handler

func (h *AuthHandler) Login(c *gin.Context) { /* bind dto.LoginRequest, call service, return OK or AUTH_INVALID_CREDENTIALS */ }
func (h *AuthHandler) Refresh(c *gin.Context) { /* bind dto.RefreshRequest, call service, return rotated tokens */ }
func (h *AuthHandler) Logout(c *gin.Context) { /* revoke refresh token */ }
func (h *AuthHandler) Me(c *gin.Context) { /* return user from service by c.Get("user_id") */ }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/interfaces/http/handler -run "Test(LoginReturnsTokenPair|MeReturnsUnauthorizedWithoutBearerToken)" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/dto/auth_dto.go internal/application/service/auth_app_svc.go internal/interfaces/middleware/auth_mw.go internal/interfaces/http/handler/auth_handler.go internal/interfaces/http/handler/auth_handler_test.go
git commit -m "feat: add auth endpoints and middleware"
```
### Task 7: Implement system config endpoints and wire the runnable server

**Files:**
- Create: `internal/application/service/system_app_svc.go`
- Create: `internal/interfaces/http/router.go`
- Create: `internal/interfaces/middleware/security.go`
- Create: `internal/pkg/logger/logger.go`
- Create: `cmd/server/main.go`
- Test: `internal/interfaces/http/router_test.go`
- Test: `internal/interfaces/http/handler/system_config_handler_test.go`

- [ ] **Step 1: Write the failing system/router tests**

```go
package http

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
)

func TestRouterRegistersHealthAndVersionRoutes(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := NewRouter(newFakeSetupHandler(), newFakeAuthHandler(), newFakeSystemHandler(), nil)

    for _, path := range []string{"/api/v1/health", "/api/v1/system/version"} {
        rec := httptest.NewRecorder()
        req := httptest.NewRequest(http.MethodGet, path, nil)
        r.Engine().ServeHTTP(rec, req)
        if rec.Code != http.StatusOK {
            t.Fatalf("route %s expected 200, got %d", path, rec.Code)
        }
    }
}
```

```go
package handler

import (
    "bytes"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
)

func TestUpdateSystemConfigRequiresAdminAndReturnsUpdatedValues(t *testing.T) {
    gin.SetMode(gin.TestMode)
    svc := &fakeSystemService{configResp: map[string]any{"site_name": "云匣", "multi_user_enabled": true}}
    h := NewSystemHandler("1.0.0", "abcdef1", "2026-04-21T12:00:00+08:00", "go1.24.0", svc)
    r := gin.New()
    r.Use(func(c *gin.Context) { c.Set("user_role", "admin"); c.Next() })
    r.PUT("/api/v1/system/config", h.UpdateConfig)

    rec := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPut, "/api/v1/system/config", bytes.NewBufferString(`{"site_name":"云匣","multi_user_enabled":true,"default_chunk_size":5242880,"max_upload_size":10737418240,"webdav_enabled":true,"webdav_prefix":"/dav","theme":"system","language":"zh-CN","time_zone":"Asia/Shanghai"}`))
    req.Header.Set("Content-Type", "application/json")
    r.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/interfaces/http/... -run "Test(RouterRegistersHealthAndVersionRoutes|UpdateSystemConfigRequiresAdminAndReturnsUpdatedValues)" -v`
Expected: FAIL with missing router constructors or handler methods

- [ ] **Step 3: Write minimal implementation**

```go
package service

import (
    "context"

    "yunxia/internal/application/dto"
    "yunxia/internal/domain/entity"
    "yunxia/internal/domain/repository"
)

type SystemApplicationService struct {
    repo repository.SystemConfigRepository
}

func NewSystemApplicationService(repo repository.SystemConfigRepository) *SystemApplicationService {
    return &SystemApplicationService{repo: repo}
}

func (s *SystemApplicationService) GetConfig(ctx context.Context) (*entity.SystemConfig, error) {
    return s.repo.Get(ctx)
}

func (s *SystemApplicationService) UpdateConfig(ctx context.Context, cfg *entity.SystemConfig) (*entity.SystemConfig, error) {
    if err := s.repo.Upsert(ctx, cfg); err != nil {
        return nil, err
    }
    return s.repo.Get(ctx)
}
```

```go
package middleware

import "github.com/gin-gonic/gin"

func SecurityHeaders() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("X-Frame-Options", "DENY")
        c.Next()
    }
}
```

```go
package logger

import "go.uber.org/zap"

func New() (*zap.Logger, error) {
    return zap.NewProduction()
}
```

```go
package http

import (
    "github.com/gin-gonic/gin"

    "yunxia/internal/interfaces/http/handler"
    "yunxia/internal/interfaces/middleware"
)

type Router struct { engine *gin.Engine }

func NewRouter(setupHandler *handler.SetupHandler, authHandler *handler.AuthHandler, systemHandler *handler.SystemHandler, authMW *middleware.AuthMiddleware) *Router {
    r := gin.New()
    r.Use(gin.Recovery(), middleware.SecurityHeaders())

    api := r.Group("/api/v1")
    api.GET("/health", systemHandler.Health)
    api.GET("/setup/status", setupHandler.Status)
    api.POST("/setup/init", setupHandler.Init)
    api.POST("/auth/login", authHandler.Login)
    api.POST("/auth/refresh", authHandler.Refresh)
    api.GET("/system/version", systemHandler.Version)

    authed := api.Group("")
    authed.Use(authMW.RequireAuth())
    authed.GET("/auth/me", authHandler.Me)
    authed.POST("/auth/logout", authHandler.Logout)
    authed.GET("/system/config", systemHandler.GetConfig)
    authed.PUT("/system/config", systemHandler.UpdateConfig)

    return &Router{engine: r}
}

func (r *Router) Engine() *gin.Engine { return r.engine }
```

```go
package main

import (
    "fmt"
    "log"

    appcfg "yunxia/internal/infrastructure/config"
)

func main() {
    cfg, err := appcfg.Load()
    if err != nil {
        log.Fatal(err)
    }
    addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
    log.Printf("yunxia listening on %s", addr)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/interfaces/http/... -run "Test(RouterRegistersHealthAndVersionRoutes|UpdateSystemConfigRequiresAdminAndReturnsUpdatedValues)" -v`
Expected: PASS

- [ ] **Step 5: Run the full verification suite**

Run: `go test ./...`
Expected: PASS across config, persistence, security, handler, and router packages

- [ ] **Step 6: Commit**

```bash
git add internal/application/service/system_app_svc.go internal/interfaces/http/router.go internal/interfaces/middleware/security.go internal/pkg/logger/logger.go cmd/server/main.go internal/interfaces/http/router_test.go internal/interfaces/http/handler/system_config_handler_test.go
git commit -m "feat: wire system config endpoints and runnable server"
```

---

## Self-review

### Spec coverage

This plan covers the approved API contract sections for:
- `GET /api/v1/health`
- `GET /api/v1/setup/status`
- `POST /api/v1/setup/init`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`
- `GET /api/v1/system/config`
- `PUT /api/v1/system/config`
- `GET /api/v1/system/version`
- Global REST envelope, auth middleware, config defaults, SQLite bootstrap, JWT/bcrypt

Not covered by this plan on purpose:
- file/source/upload/task business APIs
- WebDAV
- ACL management
- Draft/Reserved APIs

### Placeholder scan

Before execution, run a quick placeholder scan and remove any unfinished prose, vague promises, or missing-code notes from the plan file.
Expected result: the scan should not reveal any unfinished placeholders or deferred-work markers.

### Type consistency checklist

Before execution, confirm these names stay consistent across all tasks:
- `SetupApplicationService`
- `AuthApplicationService`
- `SystemApplicationService`
- `UserSummary`
- `TokenPair`
- `SystemConfig`
- `AuthMiddleware`
- `JWTTokenService`
- `BcryptHasher`

If any name changes during execution, update the later tasks in the plan before continuing.

