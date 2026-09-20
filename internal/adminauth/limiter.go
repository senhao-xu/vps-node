package adminauth

import (
	"sync"
	"time"
)

type limiterEntry struct {
	failures    int
	windowStart time.Time
	lockedUntil time.Time
}

type Limiter struct {
	mu      sync.Mutex
	entries map[string]*limiterEntry
	Max     int
	Window  time.Duration
	Lockout time.Duration
}

func NewLimiter(max int, window, lockout time.Duration) *Limiter {
	return &Limiter{
		entries: map[string]*limiterEntry{},
		Max:     max,
		Window:  window,
		Lockout: lockout,
	}
}

func (l *Limiter) Blocked(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	return ok && now.Before(e.lockedUntil)
}

func (l *Limiter) Fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok || now.Sub(e.windowStart) >= l.Window {
		e = &limiterEntry{windowStart: now}
		l.entries[key] = e
	}
	e.failures++
	if e.failures >= l.Max {
		e.lockedUntil = now.Add(l.Lockout)
	}
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}
