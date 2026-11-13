package auth

import (
	"testing"
)

func TestGenerateSecureToken(t *testing.T) {
	token1, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	token2, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	if token1 == token2 {
		t.Error("expected randomly generated tokens to be distinct")
	}

	if len(token1) != 64 { // 32 bytes hex encoded = 64 hex chars
		t.Errorf("expected token length 64, got %d", len(token1))
	}
}

func TestHashTokenDeterministic(t *testing.T) {
	raw := "random-test-token-value-12345"
	hash1 := HashToken(raw)
	hash2 := HashToken(raw)

	if hash1 != hash2 {
		t.Errorf("expected deterministic hash output, got %s and %s", hash1, hash2)
	}

	if len(hash1) != 64 {
		t.Errorf("expected SHA-256 hex length 64, got %d", len(hash1))
	}
}
