package security

import "testing"

func TestBcryptHasherHashAndCompare(t *testing.T) {
    hasher := NewBcryptHasher(12)

    hash, err := hasher.Hash("strong-password-123")
    if err != nil {
        t.Fatalf("Hash() error = %v", err)
    }
    if hash == "strong-password-123" {
        t.Fatalf("expected hashed password, got plain text")
    }
    if !hasher.Compare(hash, "strong-password-123") {
        t.Fatalf("expected Compare() to succeed")
    }
    if hasher.Compare(hash, "wrong-password") {
        t.Fatalf("expected Compare() to fail for wrong password")
    }
}
