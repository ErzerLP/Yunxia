package security

import (
    "testing"
    "time"
)

func TestJWTTokenServiceIssuesAndValidatesTokens(t *testing.T) {
    svc := NewJWTTokenService("test-secret", 15*time.Minute, 7*24*time.Hour)

    accessToken, err := svc.IssueAccessToken(7, "admin", 3)
    if err != nil {
        t.Fatalf("IssueAccessToken() error = %v", err)
    }
    accessClaims, err := svc.ValidateAccessToken(accessToken)
    if err != nil {
        t.Fatalf("ValidateAccessToken() error = %v", err)
    }
    if accessClaims.UserID != 7 || accessClaims.Role != "admin" || accessClaims.TokenVersion != 3 {
        t.Fatalf("unexpected access claims = %+v", accessClaims)
    }

    refreshToken, err := svc.IssueRefreshToken(7, "admin", 3)
    if err != nil {
        t.Fatalf("IssueRefreshToken() error = %v", err)
    }
    refreshClaims, err := svc.ValidateRefreshToken(refreshToken)
    if err != nil {
        t.Fatalf("ValidateRefreshToken() error = %v", err)
    }
    if refreshClaims.TokenType != TokenTypeRefresh {
        t.Fatalf("expected refresh token type, got %q", refreshClaims.TokenType)
    }
}
