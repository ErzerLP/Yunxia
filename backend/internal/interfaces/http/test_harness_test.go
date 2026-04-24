package http

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"yunxia/internal/domain/entity"
	gormrepo "yunxia/internal/infrastructure/persistence/gorm"
)

type testRouterHarness struct {
	AuditRepo *gormrepo.AuditLogRepository
	InfoBuf   *bytes.Buffer
	ErrBuf    *bytes.Buffer
}

var testRouterHarnessRegistry sync.Map

func registerTestRouterHarness(engine *gin.Engine, harness *testRouterHarness) {
	if engine == nil || harness == nil {
		return
	}
	testRouterHarnessRegistry.Store(engine, harness)
}

func lookupTestRouterHarness(t *testing.T, engine *gin.Engine) *testRouterHarness {
	t.Helper()

	value, ok := testRouterHarnessRegistry.Load(engine)
	if !ok {
		t.Fatalf("test router harness not found for engine %p", engine)
	}
	harness, ok := value.(*testRouterHarness)
	if !ok || harness == nil {
		t.Fatalf("invalid test router harness for engine %p", engine)
	}
	return harness
}

func bootstrapOperator(t *testing.T, engine *gin.Engine) string {
	t.Helper()

	rec := performRequest(t, engine, "POST", "/api/v1/auth/login", map[string]any{
		"username": "admin",
		"password": "strong-password-123",
	}, "")
	if rec.Code != 200 {
		t.Fatalf("admin login expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	adminToken := decodeEnvelope[tokenData](t, rec.Body.Bytes()).Tokens.AccessToken
	return createUserWithRoleAndLoginForTest(
		t,
		engine,
		adminToken,
		fmt.Sprintf("operator-%d", time.Now().UnixNano()),
		"strong-password-123",
		"operator",
	)
}

func assertAuditLogExists(t *testing.T, engine *gin.Engine, expected map[string]any) {
	t.Helper()

	harness := lookupTestRouterHarness(t, engine)
	items, _, err := harness.AuditRepo.List(context.Background(), entity.AuditLogFilter{
		Page:     1,
		PageSize: 200,
	})
	if err != nil {
		t.Fatalf("auditRepo.List() error = %v", err)
	}
	for _, item := range items {
		if auditLogMatchesExpected(item, expected) {
			return
		}
	}

	summaries := make([]string, 0, len(items))
	for _, item := range items {
		summaries = append(summaries, fmt.Sprintf("%s/%s/%s/%s", item.ResourceType, item.Action, item.Result, item.ErrorCode))
	}
	t.Fatalf("expected audit log %+v, got %d logs: %s", expected, len(items), strings.Join(summaries, ", "))
}

func auditLogMatchesExpected(log *entity.AuditLog, expected map[string]any) bool {
	if log == nil {
		return false
	}
	for key, expectedValue := range expected {
		switch key {
		case "entrypoint":
			if log.EntryPoint != fmt.Sprint(expectedValue) {
				return false
			}
		case "request_id":
			if log.RequestID != fmt.Sprint(expectedValue) {
				return false
			}
		case "actor_user_id":
			if !matchUintPointer(log.ActorUserID, expectedValue) {
				return false
			}
		case "actor_username":
			if log.ActorUsername != fmt.Sprint(expectedValue) {
				return false
			}
		case "actor_role_key":
			if log.ActorRoleKey != fmt.Sprint(expectedValue) {
				return false
			}
		case "method":
			if log.Method != fmt.Sprint(expectedValue) {
				return false
			}
		case "path":
			if log.Path != fmt.Sprint(expectedValue) {
				return false
			}
		case "resource_type":
			if log.ResourceType != fmt.Sprint(expectedValue) {
				return false
			}
		case "action":
			if log.Action != fmt.Sprint(expectedValue) {
				return false
			}
		case "result":
			if log.Result != fmt.Sprint(expectedValue) {
				return false
			}
		case "error_code":
			if log.ErrorCode != fmt.Sprint(expectedValue) {
				return false
			}
		case "resource_id":
			if log.ResourceID != fmt.Sprint(expectedValue) {
				return false
			}
		case "source_id":
			if !matchUintPointer(log.SourceID, expectedValue) {
				return false
			}
		case "virtual_path":
			if log.VirtualPath != fmt.Sprint(expectedValue) {
				return false
			}
		case "resolved_source_id":
			if !matchUintPointer(log.ResolvedSourceID, expectedValue) {
				return false
			}
		case "resolved_path":
			if log.ResolvedPath != fmt.Sprint(expectedValue) {
				return false
			}
		case "before_contains":
			if !strings.Contains(log.BeforeJSON, fmt.Sprint(expectedValue)) {
				return false
			}
		case "after_contains":
			if !strings.Contains(log.AfterJSON, fmt.Sprint(expectedValue)) {
				return false
			}
		case "detail_contains":
			if !strings.Contains(log.DetailJSON, fmt.Sprint(expectedValue)) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func matchUintPointer(actual *uint, expected any) bool {
	expectedPtr, ok := uintPointerFromAny(expected)
	if !ok {
		return false
	}
	if actual == nil || expectedPtr == nil {
		return actual == nil && expectedPtr == nil
	}
	return *actual == *expectedPtr
}

func uintPointerFromAny(value any) (*uint, bool) {
	switch v := value.(type) {
	case nil:
		return nil, true
	case uint:
		return &v, true
	case uint8:
		value := uint(v)
		return &value, true
	case uint16:
		value := uint(v)
		return &value, true
	case uint32:
		value := uint(v)
		return &value, true
	case uint64:
		value := uint(v)
		return &value, true
	case int:
		if v < 0 {
			return nil, false
		}
		value := uint(v)
		return &value, true
	case int8:
		if v < 0 {
			return nil, false
		}
		value := uint(v)
		return &value, true
	case int16:
		if v < 0 {
			return nil, false
		}
		value := uint(v)
		return &value, true
	case int32:
		if v < 0 {
			return nil, false
		}
		value := uint(v)
		return &value, true
	case int64:
		if v < 0 {
			return nil, false
		}
		value := uint(v)
		return &value, true
	case float64:
		if v < 0 {
			return nil, false
		}
		value := uint(v)
		return &value, true
	default:
		return nil, false
	}
}

func seedAuditLogForTest(t *testing.T, engine *gin.Engine, fields map[string]any) *entity.AuditLog {
	t.Helper()

	harness := lookupTestRouterHarness(t, engine)
	now := time.Now().UTC()
	log := &entity.AuditLog{
		OccurredAt:   now,
		RequestID:    fmt.Sprintf("seed-%d", now.UnixNano()),
		EntryPoint:   "rest_v1",
		Method:       "POST",
		Path:         "/seed/audit",
		ResourceType: "seed",
		Action:       "seed",
		Result:       "success",
		CreatedAt:    now,
	}
	applyAuditFieldOverrides(t, log, fields)
	if err := harness.AuditRepo.Create(context.Background(), log); err != nil {
		t.Fatalf("auditRepo.Create() error = %v", err)
	}
	return log
}

func applyAuditFieldOverrides(t *testing.T, log *entity.AuditLog, fields map[string]any) {
	t.Helper()

	for key, value := range fields {
		switch key {
		case "entrypoint":
			log.EntryPoint = fmt.Sprint(value)
		case "request_id":
			log.RequestID = fmt.Sprint(value)
		case "actor_user_id":
			converted, ok := uintPointerFromAny(value)
			if !ok {
				t.Fatalf("invalid actor_user_id override: %#v", value)
			}
			log.ActorUserID = converted
		case "actor_username":
			log.ActorUsername = fmt.Sprint(value)
		case "actor_role_key":
			log.ActorRoleKey = fmt.Sprint(value)
		case "method":
			log.Method = fmt.Sprint(value)
		case "path":
			log.Path = fmt.Sprint(value)
		case "resource_type":
			log.ResourceType = fmt.Sprint(value)
		case "action":
			log.Action = fmt.Sprint(value)
		case "result":
			log.Result = fmt.Sprint(value)
		case "error_code":
			log.ErrorCode = fmt.Sprint(value)
		case "resource_id":
			log.ResourceID = fmt.Sprint(value)
		case "source_id":
			converted, ok := uintPointerFromAny(value)
			if !ok {
				t.Fatalf("invalid source_id override: %#v", value)
			}
			log.SourceID = converted
		case "virtual_path":
			log.VirtualPath = fmt.Sprint(value)
		case "resolved_source_id":
			converted, ok := uintPointerFromAny(value)
			if !ok {
				t.Fatalf("invalid resolved_source_id override: %#v", value)
			}
			log.ResolvedSourceID = converted
		case "resolved_path":
			log.ResolvedPath = fmt.Sprint(value)
		case "before_json":
			log.BeforeJSON = fmt.Sprint(value)
		case "after_json":
			log.AfterJSON = fmt.Sprint(value)
		case "detail_json":
			log.DetailJSON = fmt.Sprint(value)
		default:
			t.Fatalf("unsupported audit log override key %q", key)
		}
	}
}

func assertRuntimeLogContains(t *testing.T, engine *gin.Engine, expected string) {
	t.Helper()

	harness := lookupTestRouterHarness(t, engine)
	combined := harness.InfoBuf.String() + "\n" + harness.ErrBuf.String()
	if !strings.Contains(combined, expected) {
		t.Fatalf("expected runtime log containing %q, got %s", expected, combined)
	}
}

func resetRuntimeLogBuffers(t *testing.T, engine *gin.Engine) {
	t.Helper()

	harness := lookupTestRouterHarness(t, engine)
	harness.InfoBuf.Reset()
	harness.ErrBuf.Reset()
}
