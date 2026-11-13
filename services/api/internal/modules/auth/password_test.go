package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashingAndVerification(t *testing.T) {
	rawPassword := "ValidP@ssw0rd2026!"

	hash, err := HashPassword(rawPassword)
	if err != nil {
		t.Fatalf("expected no error hashing password, got %v", err)
	}

	if len(hash) == 0 {
		t.Fatal("expected non-empty hash string")
	}

	// Correct password
	match, err := VerifyPassword(rawPassword, hash)
	if err != nil {
		t.Fatalf("unexpected verification error: %v", err)
	}
	if !match {
		t.Errorf("expected password to match hash")
	}

	// Incorrect password
	matchWrong, err := VerifyPassword("WrongP@ssw0rd2026!", hash)
	if err != nil {
		t.Fatalf("unexpected verification error: %v", err)
	}
	if matchWrong {
		t.Errorf("expected wrong password not to match hash")
	}
}

func TestValidatePasswordPolicy(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{"valid passphrase", "correct horse battery staple", nil},
		{"valid unicode passphrase", "école sans frontières", nil},
		{"too short", "shortphrase", ErrPasswordTooShort},
		{"too long", strings.Repeat("a", 129), ErrPasswordTooLong},
		{"common blocked password", "Password123!", ErrPasswordCommon},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePasswordPolicy(tt.password)
			if tt.wantErr == nil && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
			if tt.wantErr != nil && err != tt.wantErr {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestVerifyPasswordInvalidHash(t *testing.T) {
	_, err := VerifyPassword("AnyPassword1!", "invalid-hash-string")
	if err == nil {
		t.Error("expected error for malformed hash, got nil")
	}
}
