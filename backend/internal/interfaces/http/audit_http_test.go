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
