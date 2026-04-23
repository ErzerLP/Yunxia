package security

import (
	"testing"
	"time"
)

func TestJWTTokenServiceRoundTripRoleKey(t *testing.T) {
	svc := NewJWTTokenService("test-secret", time.Minute, time.Hour)

	token, err := svc.IssueAccessToken(7, "super_admin", 3)
	if err != nil {
		t.Fatalf("IssueAccessToken() error = %v", err)
	}

	claims, err := svc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken() error = %v", err)
	}
	if claims.UserID != 7 || claims.RoleKey != "super_admin" || claims.TokenVersion != 3 {
		t.Fatalf("unexpected claims = %+v", claims)
	}
}
