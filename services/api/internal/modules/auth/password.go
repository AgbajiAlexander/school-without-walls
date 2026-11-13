package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2id configuration parameters per security plan
type Argon2Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

var DefaultArgon2Params = Argon2Params{
	Memory:      64 * 1024, // 64 MB
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

var (
	ErrPasswordTooShort    = errors.New("password must be at least 12 characters long")
	ErrPasswordTooLong     = errors.New("password must not exceed 128 characters")
	ErrPasswordCommon      = errors.New("password is too common or easily guessable")
	ErrPasswordInvalidUTF8 = errors.New("password must be valid UTF-8")
	ErrInvalidHash         = errors.New("the encoded password hash is not in the correct format")
	ErrIncompatibleArgon2  = errors.New("incompatible version of argon2")
)

// Common passwords blocklist to reject trivial credentials
var commonPasswords = map[string]bool{
	"password":      true,
	"password123":   true,
	"password123!":  true,
	"admin12345!":   true,
	"welcome123!":   true,
	"letmein123!":   true,
	"school12345!":  true,
	"nigeria1234!":  true,
	"qwerty12345!":  true,
	"123456789012":  true,
	"qwertyuiop123": true,
}

// ValidatePasswordPolicy allows passphrases while blocking short and common credentials.
func ValidatePasswordPolicy(password string) error {
	if !utf8.ValidString(password) {
		return ErrPasswordInvalidUTF8
	}
	characters := utf8.RuneCountInString(password)
	if characters < 12 {
		return ErrPasswordTooShort
	}
	if characters > 128 {
		return ErrPasswordTooLong
	}

	if commonPasswords[strings.ToLower(strings.TrimSpace(password))] {
		return ErrPasswordCommon
	}

	return nil
}

// HashPassword hashes a plain text password using Argon2id and returns the PHC string.
func HashPassword(password string) (string, error) {
	params := DefaultArgon2Params

	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate random salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		params.Memory,
		params.Iterations,
		params.Parallelism,
		b64Salt,
		b64Hash,
	)

	return encoded, nil
}

// VerifyPassword checks if a plain password matches an encoded Argon2id PHC string.
func VerifyPassword(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, ErrInvalidHash
	}

	if parts[1] != "argon2id" {
		return false, ErrInvalidHash
	}

	if parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return false, ErrIncompatibleArgon2
	}

	var memory, iterations uint32
	var parallelism uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil || parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", memory, iterations, parallelism) {
		return false, ErrInvalidHash
	}
	if memory < 8*1024 || memory > 256*1024 || iterations == 0 || iterations > 10 || parallelism == 0 || parallelism > 8 {
		return false, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false, ErrInvalidHash
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expectedHash) < 16 || len(expectedHash) > 64 {
		return false, ErrInvalidHash
	}

	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		iterations,
		memory,
		parallelism,
		uint32(len(expectedHash)),
	)

	// Constant-time comparison
	if subtle.ConstantTimeCompare(expectedHash, computedHash) == 1 {
		return true, nil
	}

	return false, nil
}
