package http

import (
	"net/http"
	"testing"
)

func TestCapabilityMiddlewareWritesDeniedAudit(t *testing.T) {
	engine := newStorageTestRouter(t)
	_, _ = bootstrapAdmin(t, engine)
	userToken := bootstrapOperator(t, engine)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/users", map[string]any{
		"username": "blocked",
		"password": "strong-password-123",
		"email":    "blocked@example.com",
		"role_key": "user",
	}, userToken)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "CAPABILITY_DENIED")
	assertAuditLogExists(t, engine, map[string]any{
		"resource_type": "user",
		"action":        "create",
		"result":        "denied",
	})
}

func TestAuditLogsAPIRequiresCapability(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, _ := bootstrapAdmin(t, engine)
	operatorToken := bootstrapOperator(t, engine)

	adminRec := performRequest(t, engine, http.MethodGet, "/api/v1/audit/logs?page=1&page_size=20", nil, adminToken)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin audit list expected 200, got %d body=%s", adminRec.Code, adminRec.Body.String())
	}

	blockedRec := performRequest(t, engine, http.MethodGet, "/api/v1/audit/logs?page=1&page_size=20", nil, operatorToken)
	if blockedRec.Code != http.StatusForbidden {
		t.Fatalf("operator audit list expected 403, got %d body=%s", blockedRec.Code, blockedRec.Body.String())
	}
}

func TestAuditLogsAPIFiltersByResourceTypeAndResult(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, _ := bootstrapAdmin(t, engine)

	seedAuditLogForTest(t, engine, map[string]any{"resource_type": "user", "action": "create", "result": "success"})
	seedAuditLogForTest(t, engine, map[string]any{"resource_type": "file", "action": "rename", "result": "failed"})

	rec := performRequest(t, engine, http.MethodGet, "/api/v1/audit/logs?resource_type=user&result=success&page=1&page_size=20", nil, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("audit list expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	payload := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	items := payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 audit item, got %+v", payload)
	}
	first := items[0].(map[string]any)
	if first["resource_type"] != "user" || first["result"] != "success" {
		t.Fatalf("unexpected audit item %+v", first)
	}
}
