package auth

import (
	"sync"
	"time"
)

// FailureLimiter is an in-memory fixed-window limiter for failed attempts.
// After max failures within window, RetryAfter reports the time left until
// the window expires. State is per-process and lost on restart.
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

// RetryAfter returns how long key is blocked for, or 0 if it is not blocked.
func (l *FailureLimiter) RetryAfter(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return 0
	}
	left := e.start.Add(l.window).Sub(time.Now())
	if left <= 0 {
		delete(l.entries, key)
		return 0
	}
	if e.count < l.max {
		return 0
	}
	return left
}

// RecordFailure counts one failed attempt for key.
func (l *FailureLimiter) RecordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if e, ok := l.entries[key]; ok && now.Sub(e.start) < l.window {
		e.count++
		return
	}
	if _, ok := l.entries[key]; !ok && len(l.entries) >= l.maxKeys {
		l.evict(now)
	}
	l.entries[key] = &failureEntry{count: 1, start: now}
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
