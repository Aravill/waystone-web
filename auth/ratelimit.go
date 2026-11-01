package auth

import (
	"sync"
	"time"
)

// FailureLimiter is an in-memory fixed-window limiter for password attempts.
// Each attempt counts up front (Attempt); successes are undone via Reset or
// Forgive, so the count effectively tracks failures. After max within window,
// Attempt reports the time left until the window expires. State is per-process and lost on restart.
type FailureLimiter struct {
	mu      sync.Mutex
	max     int
	window  time.Duration
	maxKeys int
	entries map[string]*failureEntry
}

type failureEntry struct {
	count int
	start time.Time
}

func NewFailureLimiter(max int, window time.Duration, maxKeys int) *FailureLimiter {
	return &FailureLimiter{max: max, window: window, maxKeys: maxKeys, entries: map[string]*failureEntry{}}
}

// Attempt atomically checks and counts one attempt for key. It returns how
// long key is blocked for, or 0 if the attempt is allowed (and counted).
// Counting before the expensive check stops concurrent requests from all
// slipping through while bcrypt runs.
func (l *FailureLimiter) Attempt(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if e, ok := l.entries[key]; ok {
		if left := e.start.Add(l.window).Sub(now); left > 0 {
			if e.count >= l.max {
				return left
			}
			e.count++
			return 0
		}
		delete(l.entries, key)
	}
	if len(l.entries) >= l.maxKeys {
		l.evict(now)
	}
	l.entries[key] = &failureEntry{count: 1, start: now}
	return 0
}

// Forgive un-counts one attempt for key, e.g. after it turned out to succeed.
func (l *FailureLimiter) Forgive(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e, ok := l.entries[key]; ok && e.count > 0 {
		e.count--
	}
}

// Reset clears any failures recorded for key.
func (l *FailureLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// evict drops expired entries, then the oldest one if still full, so memory
// stays bounded under e.g. username spraying. Caller holds l.mu.
func (l *FailureLimiter) evict(now time.Time) {
	var oldestKey string
	var oldest time.Time
	for k, e := range l.entries {
		if now.Sub(e.start) >= l.window {
			delete(l.entries, k)
			continue
		}
		if oldestKey == "" || e.start.Before(oldest) {
			oldestKey, oldest = k, e.start
		}
	}
	if len(l.entries) >= l.maxKeys && oldestKey != "" {
		delete(l.entries, oldestKey)
	}
}
