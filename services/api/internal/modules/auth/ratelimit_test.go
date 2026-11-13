package auth

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(3, 100*time.Millisecond)
	key := "test-ip-127.0.0.1"

	// 1st attempt: allowed
	allowed, _ := limiter.Allow(key)
	if !allowed {
		t.Error("expected 1st attempt to be allowed")
	}

	// 2nd attempt: allowed
	allowed, _ = limiter.Allow(key)
	if !allowed {
		t.Error("expected 2nd attempt to be allowed")
	}

	// 3rd attempt: allowed
	allowed, _ = limiter.Allow(key)
	if !allowed {
		t.Error("expected 3rd attempt to be allowed")
	}

	// 4th attempt: blocked
	allowed, retryAfter := limiter.Allow(key)
	if allowed {
		t.Error("expected 4th attempt to be rate limited")
	}
	if retryAfter <= 0 {
		t.Errorf("expected positive retry duration, got %v", retryAfter)
	}

	// Wait for window to expire
	time.Sleep(120 * time.Millisecond)

	// 5th attempt after window expiration: allowed
	allowed, _ = limiter.Allow(key)
	if !allowed {
		t.Error("expected attempt after window expiration to be allowed")
	}
}

func TestRateLimiterReset(t *testing.T) {
	limiter := NewRateLimiter(2, 1*time.Minute)
	key := "user-reset-key"

	limiter.Allow(key)
	limiter.Allow(key)

	// Blocked
	allowed, _ := limiter.Allow(key)
	if allowed {
		t.Error("expected key to be blocked")
	}

	// Reset
	limiter.Reset(key)

	// Allowed again
	allowed, _ = limiter.Allow(key)
	if !allowed {
		t.Error("expected key to be allowed after explicit reset")
	}
}
