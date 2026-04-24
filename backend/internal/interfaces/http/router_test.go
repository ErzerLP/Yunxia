package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	appaudit "yunxia/internal/application/audit"
	appsvc "yunxia/internal/application/service"
	"yunxia/internal/domain/entity"
	appLog "yunxia/internal/infrastructure/observability/logging"
	gormrepo "yunxia/internal/infrastructure/persistence/gorm"
	"yunxia/internal/infrastructure/security"
	infraStorage "yunxia/internal/infrastructure/storage"
	httphandler "yunxia/internal/interfaces/http/handler"
	mw "yunxia/internal/interfaces/middleware"
)

type envelope struct {
	Success bool            `json:"success"`
	Code    string          `json:"code"`
	Data    json.RawMessage `json:"data"`
}

type setupInitData struct {
	User struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
	Tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	} `json:"tokens"`
}

type tokenData struct {
	Tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	} `json:"tokens"`
}

type testAuditCreateRepository interface {
	Create(ctx context.Context, log *entity.AuditLog) error
}

type testRouterConfig struct {
	auditCreateRepo testAuditCreateRepository
}

type failingAuditCreateRepo struct {
	err error
}

func (r failingAuditCreateRepo) Create(context.Context, *entity.AuditLog) error {
	if r.err != nil {
		return r.err
	}
	return fmt.Errorf("forced audit create failure")
}

func TestSetupAndAuthLifecycle(t *testing.T) {
	engine := newTestRouter(t)

	rec := performRequest(t, engine, http.MethodGet, "/api/v1/setup/status", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status expected 200, got %d", rec.Code)
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/setup/init", map[string]any{
		"username": "admin",
		"password": "strong-password-123",
		"email":    "admin@example.com",
	}, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup init expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	initPayload := decodeEnvelope[setupInitData](t, rec.Body.Bytes())
	if initPayload.User.Username != "admin" {
		t.Fatalf("unexpected init payload = %+v", initPayload)
	}

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/auth/me", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("auth me without token expected 401, got %d", rec.Code)
	}

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/auth/me", nil, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("auth me with token expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	mePayload := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	meUser := mePayload["user"].(map[string]any)
	if meUser["role_key"] != "super_admin" || meUser["status"] != "active" {
		t.Fatalf("unexpected me payload = %+v", mePayload)
	}
	caps := mePayload["capabilities"].([]any)
	if len(caps) == 0 {
		t.Fatalf("expected capabilities in me payload, got %+v", mePayload)
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/auth/refresh", map[string]any{
		"refresh_token": initPayload.Tokens.RefreshToken,
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	refreshPayload := decodeEnvelope[tokenData](t, rec.Body.Bytes())
	if refreshPayload.Tokens.RefreshToken == initPayload.Tokens.RefreshToken {
		t.Fatalf("expected rotated refresh token")
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/auth/logout", map[string]any{
		"refresh_token": refreshPayload.Tokens.RefreshToken,
	}, refreshPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/auth/refresh", map[string]any{
		"refresh_token": refreshPayload.Tokens.RefreshToken,
	}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSystemEndpointsRequireAuthAndPersistConfig(t *testing.T) {
	engine := newTestRouter(t)

	rec := performRequest(t, engine, http.MethodGet, "/api/v1/health", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("health expected 200, got %d", rec.Code)
	}

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/system/version", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("version without auth expected 401, got %d", rec.Code)
	}

	initRec := performRequest(t, engine, http.MethodPost, "/api/v1/setup/init", map[string]any{
		"username": "admin",
		"password": "strong-password-123",
		"email":    "admin@example.com",
	}, "")
	initPayload := decodeEnvelope[setupInitData](t, initRec.Body.Bytes())

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/system/version", nil, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("version with auth expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPut, "/api/v1/system/config", map[string]any{
		"site_name":          "云匣 Pro",
		"multi_user_enabled": true,
		"default_source_id":  nil,
		"max_upload_size":    int64(21474836480),
		"default_chunk_size": int64(5242880),
		"webdav_enabled":     true,
		"webdav_prefix":      "/dav",
		"theme":              "system",
		"language":           "zh-CN",
		"time_zone":          "Asia/Shanghai",
	}, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("update config expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/system/config", nil, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("get config expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	cfgPayload := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if cfgPayload["site_name"] != "云匣 Pro" {
		t.Fatalf("expected updated site_name, got %+v", cfgPayload)
	}
}

func TestSystemStatsRequireCapabilityAndReturnAggregates(t *testing.T) {
	engine := newTestRouter(t)

	rec := performRequest(t, engine, http.MethodGet, "/api/v1/system/stats", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("stats without auth expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}

	initRec := performRequest(t, engine, http.MethodPost, "/api/v1/setup/init", map[string]any{
		"username": "admin",
		"password": "strong-password-123",
		"email":    "admin@example.com",
	}, "")
	if initRec.Code != http.StatusCreated {
		t.Fatalf("setup init expected 201, got %d body=%s", initRec.Code, initRec.Body.String())
	}
	initPayload := decodeEnvelope[setupInitData](t, initRec.Body.Bytes())

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/system/stats", nil, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats with admin expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	stats := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	requiredKeys := []string{
		"sources_total",
		"files_total",
		"downloads_running",
		"downloads_completed",
		"users_total",
		"storage_used_bytes",
	}
	for _, key := range requiredKeys {
		if _, exists := stats[key]; !exists {
			t.Fatalf("expected stats key %s, got %+v", key, stats)
		}
	}
}

func TestUserManagementRequireCapabilityAndLifecycle(t *testing.T) {
	engine := newTestRouter(t)

	rec := performRequest(t, engine, http.MethodGet, "/api/v1/users?page=1&page_size=20", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("users without auth expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}

	initRec := performRequest(t, engine, http.MethodPost, "/api/v1/setup/init", map[string]any{
		"username": "admin",
		"password": "strong-password-123",
		"email":    "admin@example.com",
	}, "")
	if initRec.Code != http.StatusCreated {
		t.Fatalf("setup init expected 201, got %d body=%s", initRec.Code, initRec.Body.String())
	}
	initPayload := decodeEnvelope[setupInitData](t, initRec.Body.Bytes())

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/users", map[string]any{
		"username": "alice",
		"password": "strong-password-456",
		"email":    "alice@example.com",
		"role_key": "user",
	}, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	user := created["user"].(map[string]any)
	userID := int(user["id"].(float64))
	if user["username"] != "alice" || user["role_key"] != "user" || user["status"] != "active" {
		t.Fatalf("unexpected created user payload = %+v", created)
	}

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/users?page=1&page_size=20&keyword=ali&status=active", nil, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("list users expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	items := listed["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 listed user, got %+v", listed)
	}

	rec = performRequest(t, engine, http.MethodPut, fmt.Sprintf("/api/v1/users/%d", userID), map[string]any{
		"email":    "alice+updated@example.com",
		"role_key": "admin",
		"status":   "locked",
	}, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("update user expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	updated := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	updatedUser := updated["user"].(map[string]any)
	if updatedUser["email"] != "alice+updated@example.com" || updatedUser["role_key"] != "admin" || updatedUser["status"] != "locked" {
		t.Fatalf("unexpected updated user payload = %+v", updated)
	}

	rec = performRequest(t, engine, http.MethodPost, fmt.Sprintf("/api/v1/users/%d/reset-password", userID), map[string]any{
		"new_password": "new-strong-password-789",
	}, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset password expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, fmt.Sprintf("/api/v1/users/%d/revoke-tokens", userID), nil, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke tokens expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	revoked := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if revoked["id"].(float64) != float64(userID) || revoked["revoked"] != true {
		t.Fatalf("unexpected revoke payload = %+v", revoked)
	}
}

func TestACLManagementRequireCapabilityAndLifecycle(t *testing.T) {
	engine := newTestRouter(t)

	rec := performRequest(t, engine, http.MethodGet, "/api/v1/acl/rules?source_id=1&path=/projects", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("acl rules without auth expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}

	initRec := performRequest(t, engine, http.MethodPost, "/api/v1/setup/init", map[string]any{
		"username": "admin",
		"password": "strong-password-123",
		"email":    "admin@example.com",
	}, "")
	if initRec.Code != http.StatusCreated {
		t.Fatalf("setup init expected 201, got %d body=%s", initRec.Code, initRec.Body.String())
	}
	initPayload := decodeEnvelope[setupInitData](t, initRec.Body.Bytes())

	createUserRec := performRequest(t, engine, http.MethodPost, "/api/v1/users", map[string]any{
		"username": "alice",
		"password": "strong-password-456",
		"email":    "alice@example.com",
		"role_key": "user",
	}, initPayload.Tokens.AccessToken)
	if createUserRec.Code != http.StatusCreated {
		t.Fatalf("create user expected 201, got %d body=%s", createUserRec.Code, createUserRec.Body.String())
	}
	createdUser := decodeEnvelope[map[string]any](t, createUserRec.Body.Bytes())
	userID := int(createdUser["user"].(map[string]any)["id"].(float64))

	navRec := performRequest(t, engine, http.MethodGet, "/api/v1/sources?view=navigation", nil, initPayload.Tokens.AccessToken)
	if navRec.Code != http.StatusOK {
		t.Fatalf("navigation sources expected 200, got %d body=%s", navRec.Code, navRec.Body.String())
	}
	nav := decodeEnvelope[sourceListData](t, navRec.Body.Bytes())
	sourceID := int(nav.Items[0]["id"].(float64))

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/acl/rules", map[string]any{
		"source_id":    sourceID,
		"path":         "/projects",
		"subject_type": "user",
		"subject_id":   userID,
		"effect":       "allow",
		"priority":     100,
		"permissions": map[string]any{
			"read":   true,
			"write":  true,
			"delete": false,
			"share":  false,
		},
		"inherit_to_children": true,
	}, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create acl rule expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	rule := created["rule"].(map[string]any)
	ruleID := int(rule["id"].(float64))
	if rule["path"] != "/projects" || rule["virtual_path"] != "/local/projects" || rule["subject_type"] != "user" || rule["effect"] != "allow" {
		t.Fatalf("unexpected created acl rule payload = %+v", created)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/acl/rules?source_id=%d&path=/projects", sourceID), nil, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("list acl rules expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	items := listed["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 acl rule, got %+v", listed)
	}
	if items[0].(map[string]any)["virtual_path"] != "/local/projects" {
		t.Fatalf("expected listed acl virtual_path=/local/projects, got %+v", listed)
	}

	rec = performRequest(t, engine, http.MethodPut, fmt.Sprintf("/api/v1/acl/rules/%d", ruleID), map[string]any{
		"path":         "/projects",
		"subject_type": "user",
		"subject_id":   userID,
		"effect":       "deny",
		"priority":     200,
		"permissions": map[string]any{
			"read":   true,
			"write":  false,
			"delete": false,
			"share":  false,
		},
		"inherit_to_children": false,
	}, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("update acl rule expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	updated := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	updatedRule := updated["rule"].(map[string]any)
	if updatedRule["effect"] != "deny" || updatedRule["priority"].(float64) != 200 || updatedRule["inherit_to_children"] != false || updatedRule["virtual_path"] != "/local/projects" {
		t.Fatalf("unexpected updated acl rule payload = %+v", updated)
	}

	rec = performRequest(t, engine, http.MethodDelete, fmt.Sprintf("/api/v1/acl/rules/%d", ruleID), nil, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete acl rule expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/acl/rules?source_id=%d&path=/projects", sourceID), nil, initPayload.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("list acl rules after delete expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed = decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	items = listed["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("expected empty acl rules after delete, got %+v", listed)
	}
}

func TestGovernanceRoutesUseCapabilities(t *testing.T) {
	engine := newStorageTestRouter(t)
	superToken, _ := bootstrapAdmin(t, engine)
	enableMultiUserForTest(t, engine, superToken)

	adminToken := createUserWithRoleAndLoginForTest(t, engine, superToken, "alice", "strong-password-123", "admin")
	operatorToken := createUserWithRoleAndLoginForTest(t, engine, superToken, "ops", "strong-password-123", "operator")
	userToken := createUserWithRoleAndLoginForTest(t, engine, superToken, "bob", "strong-password-123", "user")

	rec := performRequest(t, engine, http.MethodGet, "/api/v1/system/config", nil, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin get system config expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/system/stats", nil, operatorToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("operator get stats expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/system/config", nil, operatorToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("operator get config expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "CAPABILITY_DENIED")

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/system/stats", nil, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user get stats expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "CAPABILITY_DENIED")

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/sources?view=navigation", nil, operatorToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("operator list sources expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/users?page=1&page_size=20", nil, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user list users expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "CAPABILITY_DENIED")
}

func TestAdminCannotCreateAnotherAdmin(t *testing.T) {
	engine := newStorageTestRouter(t)
	superToken, _ := bootstrapAdmin(t, engine)
	enableMultiUserForTest(t, engine, superToken)
	aliceAdminToken := createUserWithRoleAndLoginForTest(t, engine, superToken, "alice", "strong-password-123", "admin")

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/users", map[string]any{
		"username": "mallory",
		"password": "strong-password-123",
		"email":    "mallory@example.com",
		"role_key": "admin",
	}, aliceAdminToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "ROLE_ASSIGNMENT_FORBIDDEN")
}

func newTestRouter(t *testing.T) *gin.Engine {
	return newTestRouterWithOptions(t, testRouterConfig{})
}

func newStorageTestRouterWithFailingAuditRepo(t *testing.T) *gin.Engine {
	t.Helper()
	return newTestRouterWithOptions(t, testRouterConfig{
		auditCreateRepo: failingAuditCreateRepo{err: fmt.Errorf("forced audit create failure")},
	})
}

func newTestRouterWithOptions(t *testing.T, config testRouterConfig) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gormrepo.OpenSQLite(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	infoBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	rootLogger := appLog.NewRootLogger(
		appLog.Options{Level: "debug", Format: "json"},
		appLog.AppMeta{Service: "yunxia-backend", Env: "test", Version: "test"},
		infoBuf,
		errBuf,
	)
	previousDefaultLogger := slog.Default()
	slog.SetDefault(rootLogger)
	t.Cleanup(func() {
		slog.SetDefault(previousDefaultLogger)
	})

	userRepo := gormrepo.NewUserRepository(db)
	refreshRepo := gormrepo.NewRefreshTokenRepository(db)
	configRepo := gormrepo.NewSystemConfigRepository(db)
	sourceRepo := gormrepo.NewSourceRepository(db)
	uploadRepo := gormrepo.NewUploadSessionRepository(db)
	trashRepo := gormrepo.NewTrashItemRepository(db)
	aclRepo := gormrepo.NewACLRuleRepository(db)
	shareRepo := gormrepo.NewShareRepository(db)
	auditRepo := gormrepo.NewAuditLogRepository(db)
	hasher := security.NewBcryptHasher(4)
	tokenSvc := security.NewJWTTokenService("router-secret", 15*time.Minute, 7*24*time.Hour)
	fileAccessSvc := security.NewFileAccessTokenService("router-secret")
	auditCreateRepo := config.auditCreateRepo
	if auditCreateRepo == nil {
		auditCreateRepo = auditRepo
	}
	auditRecorder := appaudit.NewRecorder(auditCreateRepo, appLog.Component(rootLogger, "audit.recorder"))
	auditQuerySvc := appaudit.NewQueryService(auditRepo)
	options := appsvc.DefaultSystemOptions()
	root := t.TempDir()
	options.StorageDataDir = filepath.Join(root, "storage")
	options.TempDir = filepath.Join(root, "temp")

	fakeS3 := newFakeS3Driver()
	setupSvc := appsvc.NewSetupService(
		userRepo,
		refreshRepo,
		configRepo,
		sourceRepo,
		hasher,
		tokenSvc,
		options,
		appsvc.WithSetupAuditRecorder(auditRecorder),
	)
	authSvc := appsvc.NewAuthService(userRepo, refreshRepo, hasher, tokenSvc)
	systemSvc := appsvc.NewSystemService(
		configRepo,
		options,
		appsvc.WithSystemAuditRecorder(auditRecorder),
		appsvc.WithSystemStatsDependencies(userRepo, sourceRepo, gormrepo.NewTaskRepository(db)),
		appsvc.WithSystemStatsFileDriver("s3", fakeS3),
	)
	aclAuthorizer := appsvc.NewACLAuthorizer(configRepo, aclRepo, sourceRepo)
	sourceSvc := appsvc.NewSourceService(
		sourceRepo,
		configRepo,
		appsvc.WithSourceAuditRecorder(auditRecorder),
		appsvc.WithSourceACLAuthorizer(aclAuthorizer),
		appsvc.WithSourceDriverProbe("s3", fakeS3),
	)
	userSvc := appsvc.NewUserService(userRepo, hasher, appsvc.WithUserAuditRecorder(auditRecorder))
	aclSvc := appsvc.NewACLService(sourceRepo, userRepo, aclRepo, appsvc.WithACLAuditRecorder(auditRecorder))
	fileSvc := appsvc.NewFileService(
		sourceRepo,
		fileAccessSvc,
		tokenSvc,
		userRepo,
		appsvc.WithFileAuditRecorder(auditRecorder),
		appsvc.WithFileACLAuthorizer(aclAuthorizer),
		appsvc.WithFileDriver("s3", fakeS3),
		appsvc.WithTrashItemRepository(trashRepo),
	)
	trashSvc := appsvc.NewTrashService(
		sourceRepo,
		trashRepo,
		appsvc.WithTrashAuditRecorder(auditRecorder),
		appsvc.WithTrashACLAuthorizer(aclAuthorizer),
		appsvc.WithTrashFileDriver("s3", fakeS3),
	)
	vfsSvc := appsvc.NewVFSService(
		sourceRepo,
		appsvc.WithVFSFileDriver("s3", fakeS3),
		appsvc.WithVFSFileOperator(fileSvc),
	)
	uploadSvc := appsvc.NewUploadService(
		sourceRepo,
		uploadRepo,
		options,
		appsvc.WithUploadAuditRecorder(auditRecorder),
		appsvc.WithUploadACLAuthorizer(aclAuthorizer),
		appsvc.WithUploadDriver("s3", fakeS3),
		appsvc.WithUploadVFSResolver(vfsSvc),
	)
	taskSvc := appsvc.NewTaskService(
		gormrepo.NewTaskRepository(db),
		sourceRepo,
		newFakeDownloader(),
		appsvc.WithTaskAuditRecorder(auditRecorder),
		appsvc.WithTaskACLAuthorizer(aclAuthorizer),
	)
	shareSvc := appsvc.NewShareService(
		shareRepo,
		sourceRepo,
		hasher,
		fileAccessSvc,
		appsvc.WithShareAuditRecorder(auditRecorder),
		appsvc.WithShareACLAuthorizer(aclAuthorizer),
		appsvc.WithShareFileDriver("s3", fakeS3),
	)

	setupHandler := httphandler.NewSetupHandler(setupSvc)
	authHandler := httphandler.NewAuthHandler(authSvc)
	systemHandler := httphandler.NewSystemHandler(systemSvc, "1.0.0", "abcdef1", "2026-04-21T13:00:00+08:00", "go1.25.5")
	auditHandler := httphandler.NewAuditHandler(auditQuerySvc)
	sourceHandler := httphandler.NewSourceHandler(sourceSvc)
	userHandler := httphandler.NewUserHandler(userSvc)
	aclHandler := httphandler.NewACLHandler(aclSvc)
	fileHandler := httphandler.NewFileHandler(fileSvc)
	trashHandler := httphandler.NewTrashHandler(trashSvc)
	uploadHandler := httphandler.NewUploadHandler(uploadSvc)
	taskHandler := httphandler.NewTaskHandler(taskSvc)
	shareHandler := httphandler.NewShareHandler(shareSvc)
	vfsHandler := httphandler.NewVFSHandler(vfsSvc, fileSvc)
	webdavHandler := httphandler.NewWebDAVHandler(
		"/dav",
		sourceRepo,
		configRepo,
		userRepo,
		aclAuthorizer,
		hasher,
		auditRecorder,
		appLog.Component(rootLogger, "http.webdav"),
	)
	authMW := mw.NewAuthMiddleware(userRepo, tokenSvc)

	engine := NewRouter(setupHandler, authHandler, systemHandler, authMW, rootLogger, "/dav", true)
	RegisterStorageRoutes(engine, sourceHandler, fileHandler, trashHandler, uploadHandler, authMW, auditRecorder, rootLogger)
	RegisterUserRoutes(engine, userHandler, authMW, auditRecorder, rootLogger)
	RegisterACLRoutes(engine, aclHandler, authMW, auditRecorder, rootLogger)
	RegisterAuditRoutes(engine, auditHandler, authMW, auditRecorder, rootLogger)
	RegisterTaskRoutes(engine, taskHandler, authMW)
	RegisterShareRoutes(engine, shareHandler, authMW)
	RegisterVFSRoutes(engine, vfsHandler, authMW)
	RegisterWebDAVRoutes(engine, "/dav", webdavHandler)
	registerTestRouterHarness(engine, &testRouterHarness{
		AuditRepo: auditRepo,
		InfoBuf:   infoBuf,
		ErrBuf:    errBuf,
	})
	return engine
}

type fakeDownloader struct {
	mu     sync.Mutex
	nextID int
	tasks  map[string]*appsvc.DownloadStatus
}

func newFakeDownloader() *fakeDownloader {
	return &fakeDownloader{
		tasks: make(map[string]*appsvc.DownloadStatus),
	}
}

func (d *fakeDownloader) AddURI(_ context.Context, _ string, _ string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.nextID++
	id := fmt.Sprintf("gid-%d", d.nextID)
	total := int64(100)
	eta := int64(1)
	d.tasks[id] = &appsvc.DownloadStatus{
		Status:         "running",
		CompletedBytes: 25,
		TotalBytes:     &total,
		DownloadSpeed:  1024,
		ETASeconds:     &eta,
	}
	return id, nil
}

func (d *fakeDownloader) TellStatus(_ context.Context, externalID string) (*appsvc.DownloadStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	status, ok := d.tasks[externalID]
	if !ok {
		return &appsvc.DownloadStatus{Status: "canceled"}, nil
	}
	copied := *status
	return &copied, nil
}

func (d *fakeDownloader) Remove(_ context.Context, externalID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.tasks, externalID)
	return nil
}

func (d *fakeDownloader) Pause(_ context.Context, externalID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if task, ok := d.tasks[externalID]; ok {
		task.Status = "paused"
		task.DownloadSpeed = 0
		task.ETASeconds = nil
	}
	return nil
}

func (d *fakeDownloader) Resume(_ context.Context, externalID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if task, ok := d.tasks[externalID]; ok {
		eta := int64(1)
		task.Status = "running"
		task.DownloadSpeed = 1024
		task.ETASeconds = &eta
	}
	return nil
}

type fakeS3Driver struct {
	files          map[string]appsvc.StorageEntry
	nextUploadID   int
	pendingUploads map[string]fakeMultipartUpload
}

func newFakeS3Driver() *fakeS3Driver {
	now := time.Date(2026, 4, 21, 15, 0, 0, 0, time.FixedZone("CST", 8*3600))
	return &fakeS3Driver{
		pendingUploads: make(map[string]fakeMultipartUpload),
		files: map[string]appsvc.StorageEntry{
			"/movies/demo.mp4": {
				Name:       "demo.mp4",
				Path:       "/movies/demo.mp4",
				IsDir:      false,
				Size:       128 * 1024 * 1024,
				ETag:       "etag-demo",
				ModifiedAt: now,
			},
			"/movies/trailer.mp4": {
				Name:       "trailer.mp4",
				Path:       "/movies/trailer.mp4",
				IsDir:      false,
				Size:       64 * 1024 * 1024,
				ETag:       "etag-trailer",
				ModifiedAt: now.Add(-time.Hour),
			},
			"/covers/poster.jpg": {
				Name:       "poster.jpg",
				Path:       "/covers/poster.jpg",
				IsDir:      false,
				Size:       512 * 1024,
				ETag:       "etag-poster",
				ModifiedAt: now.Add(-2 * time.Hour),
			},
		},
	}
}

type fakeMultipartUpload struct {
	virtualPath string
	size        int64
}

func (d *fakeS3Driver) Test(_ context.Context, source *entity.StorageSource) error {
	_, err := infraStorage.ParseS3ConfigJSON(source.ConfigJSON)
	return err
}

func (d *fakeS3Driver) List(_ context.Context, _ *entity.StorageSource, virtualPath string) ([]appsvc.StorageEntry, error) {
	virtualPath = normalizeFakePath(virtualPath)
	if virtualPath == "" {
		return nil, os.ErrNotExist
	}

	fileItems := make([]appsvc.StorageEntry, 0)
	dirSet := map[string]struct{}{}
	for filePath, entry := range d.files {
		if !strings.HasPrefix(filePath, virtualPath) {
			if virtualPath != "/" || !strings.HasPrefix(filePath, "/") {
				continue
			}
		}

		if virtualPath == "/" {
			trimmed := strings.TrimPrefix(filePath, "/")
			parts := strings.Split(trimmed, "/")
			if len(parts) == 1 {
				fileItems = append(fileItems, entry)
				continue
			}
			dirSet["/"+parts[0]] = struct{}{}
			continue
		}

		prefix := strings.TrimSuffix(virtualPath, "/") + "/"
		if !strings.HasPrefix(filePath, prefix) {
			continue
		}
		trimmed := strings.TrimPrefix(filePath, prefix)
		parts := strings.Split(trimmed, "/")
		if len(parts) == 1 {
			fileItems = append(fileItems, entry)
			continue
		}
		dirSet[path.Join(virtualPath, parts[0])] = struct{}{}
	}

	items := make([]appsvc.StorageEntry, 0, len(dirSet)+len(fileItems))
	for dirPath := range dirSet {
		items = append(items, appsvc.StorageEntry{
			Name:       path.Base(dirPath),
			Path:       dirPath,
			IsDir:      true,
			ModifiedAt: time.Date(2026, 4, 21, 15, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		})
	}
	items = append(items, fileItems...)
	return items, nil
}

func (d *fakeS3Driver) SearchByName(_ context.Context, _ *entity.StorageSource, pathPrefix, keyword string) ([]appsvc.StorageEntry, error) {
	pathPrefix = normalizeFakePath(pathPrefix)
	if pathPrefix == "" {
		return nil, os.ErrNotExist
	}
	lowerKeyword := strings.ToLower(keyword)
	items := make([]appsvc.StorageEntry, 0)
	for filePath, entry := range d.files {
		if entry.IsDir {
			continue
		}
		if pathPrefix != "/" && !strings.HasPrefix(filePath, strings.TrimSuffix(pathPrefix, "/")+"/") && filePath != pathPrefix {
			continue
		}
		if strings.Contains(strings.ToLower(entry.Name), lowerKeyword) {
			items = append(items, entry)
		}
	}
	return items, nil
}

func (d *fakeS3Driver) Stat(_ context.Context, _ *entity.StorageSource, virtualPath string) (*appsvc.StorageEntry, error) {
	virtualPath = normalizeFakePath(virtualPath)
	entry, exists := d.fakePathEntry(virtualPath)
	if !exists {
		return nil, os.ErrNotExist
	}
	return &entry, nil
}

func (d *fakeS3Driver) Mkdir(_ context.Context, _ *entity.StorageSource, parentPath, name string) (*appsvc.StorageEntry, error) {
	parentPath = normalizeFakePath(parentPath)
	if !d.fakeDirExists(parentPath) {
		return nil, os.ErrInvalid
	}
	newPath := path.Join(parentPath, name)
	if parentPath == "/" {
		newPath = "/" + name
	}
	if _, exists := d.files[newPath]; exists {
		return nil, os.ErrExist
	}
	entry := appsvc.StorageEntry{
		Name:       name,
		Path:       newPath,
		IsDir:      true,
		ModifiedAt: time.Now(),
	}
	d.files[newPath] = entry
	return &entry, nil
}

func (d *fakeS3Driver) Rename(_ context.Context, _ *entity.StorageSource, virtualPath, newName string) (*appsvc.StorageEntry, error) {
	virtualPath = normalizeFakePath(virtualPath)
	entry, exists := d.fakePathEntry(virtualPath)
	if !exists {
		return nil, os.ErrNotExist
	}
	parentPath := path.Dir(virtualPath)
	if parentPath == "." {
		parentPath = "/"
	}
	newPath := path.Join(parentPath, newName)
	if parentPath == "/" {
		newPath = "/" + newName
	}
	if d.fakePathExists(newPath) {
		return nil, os.ErrExist
	}
	if entry.IsDir {
		d.rewriteDirectoryTree(virtualPath, newPath, false)
	} else {
		delete(d.files, virtualPath)
		entry.Path = newPath
		entry.Name = path.Base(newPath)
		d.files[newPath] = entry
	}
	entry.Name = newName
	entry.Path = newPath
	return &entry, nil
}

func (d *fakeS3Driver) Move(_ context.Context, _ *entity.StorageSource, virtualPath, targetPath string) error {
	virtualPath = normalizeFakePath(virtualPath)
	targetPath = normalizeFakePath(targetPath)
	entry, exists := d.fakePathEntry(virtualPath)
	if !exists {
		return os.ErrNotExist
	}
	if !d.fakeDirExists(targetPath) {
		return os.ErrInvalid
	}
	newPath := path.Join(targetPath, path.Base(virtualPath))
	if targetPath == "/" {
		newPath = "/" + path.Base(virtualPath)
	}
	if d.fakePathExists(newPath) {
		return os.ErrExist
	}
	if entry.IsDir {
		d.rewriteDirectoryTree(virtualPath, newPath, false)
		return nil
	}
	delete(d.files, virtualPath)
	entry.Path = newPath
	entry.Name = path.Base(newPath)
	d.files[newPath] = entry
	return nil
}

func (d *fakeS3Driver) Copy(_ context.Context, _ *entity.StorageSource, virtualPath, targetPath string) error {
	virtualPath = normalizeFakePath(virtualPath)
	targetPath = normalizeFakePath(targetPath)
	entry, exists := d.fakePathEntry(virtualPath)
	if !exists {
		return os.ErrNotExist
	}
	if !d.fakeDirExists(targetPath) {
		return os.ErrInvalid
	}
	newPath := path.Join(targetPath, path.Base(virtualPath))
	if targetPath == "/" {
		newPath = "/" + path.Base(virtualPath)
	}
	if d.fakePathExists(newPath) {
		return os.ErrExist
	}
	if entry.IsDir {
		d.rewriteDirectoryTree(virtualPath, newPath, true)
		return nil
	}
	copied := entry
	copied.Path = newPath
	copied.Name = path.Base(newPath)
	d.files[newPath] = copied
	return nil
}

func (d *fakeS3Driver) Delete(_ context.Context, _ *entity.StorageSource, virtualPath string) error {
	virtualPath = normalizeFakePath(virtualPath)
	entry, exists := d.fakePathEntry(virtualPath)
	if !exists {
		return os.ErrNotExist
	}
	if entry.IsDir {
		d.deleteDirectoryTree(virtualPath)
		return nil
	}
	delete(d.files, virtualPath)
	return nil
}

func (d *fakeS3Driver) PresignDownload(_ context.Context, _ *entity.StorageSource, virtualPath, disposition string, ttl time.Duration) (string, time.Time, error) {
	virtualPath = normalizeFakePath(virtualPath)
	entry, exists := d.files[virtualPath]
	if !exists {
		return "", time.Time{}, os.ErrNotExist
	}
	expiresAt := time.Now().Add(ttl)
	params := url.Values{}
	params.Set("disposition", disposition)
	params.Set("path", entry.Path)
	return "https://fake-s3.local/download?" + params.Encode(), expiresAt, nil
}

func (d *fakeS3Driver) InitMultipartUpload(_ context.Context, _ *entity.StorageSource, req appsvc.MultipartUploadRequest) (*appsvc.MultipartUploadPlan, error) {
	d.nextUploadID++
	remoteUploadID := fmt.Sprintf("remote-%d", d.nextUploadID)
	finalPath := path.Join(req.VirtualPath, req.Filename)
	if req.VirtualPath == "/" {
		finalPath = "/" + req.Filename
	}
	d.pendingUploads[remoteUploadID] = fakeMultipartUpload{
		virtualPath: finalPath,
		size:        req.FileSize,
	}

	instructions := make([]appsvc.MultipartUploadPartInstruction, 0, req.TotalParts)
	for index := 0; index < req.TotalParts; index++ {
		start := int64(index) * req.PartSize
		end := start + req.PartSize - 1
		if end >= req.FileSize {
			end = req.FileSize - 1
		}
		instructions = append(instructions, appsvc.MultipartUploadPartInstruction{
			Index:     index,
			Method:    "PUT",
			URL:       fmt.Sprintf("https://fake-s3.local/upload/%s/%d", remoteUploadID, index),
			Headers:   map[string]string{},
			ByteStart: start,
			ByteEnd:   end,
			ExpiresAt: time.Now().Add(req.ExpiresIn),
		})
	}

	return &appsvc.MultipartUploadPlan{
		State: appsvc.MultipartUploadState{
			RemoteUploadID: remoteUploadID,
			ObjectKey:      strings.TrimPrefix(finalPath, "/"),
			VirtualPath:    finalPath,
		},
		PartInstructions: instructions,
	}, nil
}

func (d *fakeS3Driver) CompleteMultipartUpload(_ context.Context, _ *entity.StorageSource, state appsvc.MultipartUploadState, parts []appsvc.CompletedUploadPart) (*appsvc.StorageEntry, error) {
	upload, exists := d.pendingUploads[state.RemoteUploadID]
	if !exists || len(parts) == 0 {
		return nil, os.ErrNotExist
	}
	entry := appsvc.StorageEntry{
		Name:       path.Base(upload.virtualPath),
		Path:       upload.virtualPath,
		IsDir:      false,
		Size:       upload.size,
		ETag:       strings.Trim(parts[0].ETag, `"`),
		ModifiedAt: time.Now(),
	}
	d.files[upload.virtualPath] = entry
	delete(d.pendingUploads, state.RemoteUploadID)
	return &entry, nil
}

func normalizeFakePath(value string) string {
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		return ""
	}
	cleaned := path.Clean(value)
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	if strings.Contains(cleaned, "..") {
		return ""
	}
	return cleaned
}

func (d *fakeS3Driver) fakeDirExists(targetPath string) bool {
	if targetPath == "/" {
		return true
	}
	if entry, exists := d.files[targetPath]; exists && entry.IsDir {
		return true
	}
	prefix := strings.TrimSuffix(targetPath, "/") + "/"
	for filePath := range d.files {
		if strings.HasPrefix(filePath, prefix) {
			return true
		}
	}
	return false
}

func (d *fakeS3Driver) fakePathEntry(targetPath string) (appsvc.StorageEntry, bool) {
	if entry, exists := d.files[targetPath]; exists {
		return entry, true
	}
	if d.fakeDirExists(targetPath) {
		return appsvc.StorageEntry{
			Name:       path.Base(targetPath),
			Path:       targetPath,
			IsDir:      true,
			ModifiedAt: time.Now(),
		}, true
	}
	return appsvc.StorageEntry{}, false
}

func (d *fakeS3Driver) fakePathExists(targetPath string) bool {
	_, exists := d.fakePathEntry(targetPath)
	return exists
}

func (d *fakeS3Driver) rewriteDirectoryTree(sourcePath string, targetPath string, copyOnly bool) {
	sourcePrefix := strings.TrimSuffix(sourcePath, "/") + "/"
	updates := make(map[string]appsvc.StorageEntry)
	removals := make([]string, 0)

	for currentPath, entry := range d.files {
		if currentPath != sourcePath && !strings.HasPrefix(currentPath, sourcePrefix) {
			continue
		}
		relative := strings.TrimPrefix(currentPath, sourcePath)
		newPath := targetPath + relative
		updated := entry
		updated.Path = newPath
		updated.Name = path.Base(newPath)
		updates[newPath] = updated
		removals = append(removals, currentPath)
	}

	if _, exists := updates[targetPath]; !exists {
		updates[targetPath] = appsvc.StorageEntry{
			Name:       path.Base(targetPath),
			Path:       targetPath,
			IsDir:      true,
			ModifiedAt: time.Now(),
		}
	}

	if !copyOnly {
		for _, currentPath := range removals {
			delete(d.files, currentPath)
		}
	}
	for newPath, entry := range updates {
		d.files[newPath] = entry
	}
}

func (d *fakeS3Driver) deleteDirectoryTree(targetPath string) {
	targetPrefix := strings.TrimSuffix(targetPath, "/") + "/"
	for currentPath := range d.files {
		if currentPath == targetPath || strings.HasPrefix(currentPath, targetPrefix) {
			delete(d.files, currentPath)
		}
	}
}

func performRequest(t *testing.T, engine *gin.Engine, method, path string, body any, accessToken string) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func decodeEnvelope[T any](t *testing.T, body []byte) T {
	t.Helper()

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("json.Unmarshal(envelope) error = %v body=%s", err, string(body))
	}
	if !env.Success {
		t.Fatalf("expected success envelope, got body=%s", string(body))
	}

	var data T
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("json.Unmarshal(data) error = %v body=%s", err, string(body))
	}
	return data
}
