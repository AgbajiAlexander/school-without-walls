package auth

import (
	"sync"
	"time"
)

type rateLimitEntry struct {
	timestamps []time.Time
}

// RateLimiter implements an in-memory sliding window rate limiter.
type RateLimiter struct {
	mu           sync.Mutex
	maxAttempts  int
	window       time.Duration
	records      map[string]*rateLimitEntry
	cleanupTimer *time.Ticker
}

func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		maxAttempts: maxAttempts,
		window:      window,
		records:     make(map[string]*rateLimitEntry),
	}
	return rl
}

// Allow checks if an action for key is permitted under the sliding window.
// If allowed, it records the attempt and returns (true, 0).
// If rejected, it returns (false, retryAfter).
func (rl *RateLimiter) Allow(key string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	entry, exists := rl.records[key]
	if !exists {
		rl.records[key] = &rateLimitEntry{
			timestamps: []time.Time{now},
		}
		return true, 0
	}

	// Filter out expired timestamps
	var valid []time.Time
	for _, t := range entry.timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	entry.timestamps = valid

	if len(entry.timestamps) >= rl.maxAttempts {
		oldest := entry.timestamps[0]
		retryAfter := oldest.Add(rl.window).Sub(now)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return false, retryAfter
	}

	entry.timestamps = append(entry.timestamps, now)
	return true, 0
}

// Reset clears the rate limit history for a specific key (e.g. upon successful login).
func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.records, key)
}
