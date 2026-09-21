package adminauth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"vps-node/internal/db"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func TestHashPasswordAndCheck(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "correct-horse-battery" {
		t.Fatal("hash must not equal plaintext")
	}
	if !CheckPassword(hash, "correct-horse-battery") {
		t.Fatal("expected password to verify")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Fatal("expected wrong password to fail")
	}
	if CheckPassword("not-a-bcrypt-hash", "anything") {
		t.Fatal("expected malformed hash to fail")
	}
}

func TestNewTokenAndHash(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	b, _ := NewToken()
	if a == b {
		t.Fatal("expected unique tokens")
	}
	if len(a) != 64 {
		t.Fatalf("expected 64-char hex token, got %d", len(a))
	}
	if HashToken(a) == a || len(HashToken(a)) != 64 {
		t.Fatal("token hash must be a hex sha256 digest")
	}
	if HashToken(a) != HashToken(a) {
		t.Fatal("token hash must be deterministic")
	}
}

func TestNewURLToken(t *testing.T) {
	a, err := NewURLToken(16)
	if err != nil {
		t.Fatalf("new url token: %v", err)
	}
	b, _ := NewURLToken(16)
	if a == b {
		t.Fatal("expected unique tokens")
	}
	if len(a) != 22 {
		t.Fatalf("expected 22-char base64url token, got %d (%q)", len(a), a)
	}
	for _, r := range a {
		if !('A' <= r && r <= 'Z' || 'a' <= r && r <= 'z' || '0' <= r && r <= '9' || r == '-' || r == '_') {
			t.Fatalf("token contains non-URL-safe character %q", r)
		}
	}
	if _, err := NewURLToken(15); err == nil {
		t.Fatal("expected error below 16 bytes of entropy")
	}
	long, err := NewURLToken(32)
	if err != nil || len(long) != 43 {
		t.Fatalf("32-byte token: len=%d err=%v", len(long), err)
	}
}

func TestSessionsLifecycle(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	if _, err := d.ExecContext(ctx,
		`INSERT INTO admins (username, password_hash, created_at, updated_at) VALUES ('root', 'hash', 1, 1)`); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	var adminID int64
	if err := d.QueryRowContext(ctx, `SELECT id FROM admins WHERE username = 'root'`).Scan(&adminID); err != nil {
		t.Fatalf("load admin: %v", err)
	}

	s := NewSessions(d.DB, time.Hour, false)
	token, err := s.Create(ctx, adminID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	gotID, err := s.Resolve(ctx, token)
	if err != nil {
		t.Fatalf("resolve session: %v", err)
	}
	if gotID != adminID {
		t.Fatalf("expected admin %d, got %d", adminID, gotID)
	}

	if _, err := s.Resolve(ctx, "bogus-token"); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expected invalid session for bogus token, got %v", err)
	}
	if _, err := s.Resolve(ctx, ""); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expected invalid session for empty token, got %v", err)
	}

	if err := s.Revoke(ctx, token); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := s.Resolve(ctx, token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expected revoked session to be invalid, got %v", err)
	}
}

func TestSessionsExpire(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	if _, err := d.ExecContext(ctx,
		`INSERT INTO admins (username, password_hash, created_at, updated_at) VALUES ('root', 'hash', 1, 1)`); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	var adminID int64
	_ = d.QueryRowContext(ctx, `SELECT id FROM admins WHERE username = 'root'`).Scan(&adminID)

	s := NewSessions(d.DB, -time.Second, false)
	token, err := s.Create(ctx, adminID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := s.Resolve(ctx, token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expected expired session, got %v", err)
	}
}

func TestSessionsSlidingRefresh(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	if _, err := d.ExecContext(ctx,
		`INSERT INTO admins (username, password_hash, created_at, updated_at) VALUES ('root', 'hash', 1, 1)`); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	var adminID int64
	_ = d.QueryRowContext(ctx, `SELECT id FROM admins WHERE username = 'root'`).Scan(&adminID)

	s := NewSessions(d.DB, 2*time.Hour, false)
	token, _ := s.Create(ctx, adminID)
	hash := HashToken(token)

	var firstExpiry int64
	if err := d.QueryRowContext(ctx, `SELECT expires_at FROM admin_sessions WHERE token_hash = ?`, hash).Scan(&firstExpiry); err != nil {
		t.Fatalf("read expiry: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)
	if _, err := s.Resolve(ctx, token); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	var secondExpiry int64
	if err := d.QueryRowContext(ctx, `SELECT expires_at FROM admin_sessions WHERE token_hash = ?`, hash).Scan(&secondExpiry); err != nil {
		t.Fatalf("read expiry: %v", err)
	}
	if secondExpiry <= firstExpiry {
		t.Fatalf("expected sliding expiry to advance, %d <= %d", secondExpiry, firstExpiry)
	}
}

func TestLimiterLockout(t *testing.T) {
	l := NewLimiter(3, time.Minute, time.Minute)
	now := time.Now()
	key := "1.2.3.4|root"

	l.Fail(key, now)
	l.Fail(key, now.Add(time.Second))
	if l.Blocked(key, now.Add(2*time.Second)) {
		t.Fatal("expected not blocked below max failures")
	}

	l.Fail(key, now.Add(2*time.Second))
	if !l.Blocked(key, now.Add(3*time.Second)) {
		t.Fatal("expected blocked at max failures")
	}
	if !l.Blocked(key, now.Add(61*time.Second)) {
		t.Fatal("expected still blocked within lockout window")
	}
	if l.Blocked(key, now.Add(63*time.Second)) {
		t.Fatal("expected lockout to expire")
	}
}

func TestLimiterWindowReset(t *testing.T) {
	l := NewLimiter(3, time.Minute, time.Minute)
	now := time.Now()
	key := "1.2.3.4|root"

	l.Fail(key, now)
	l.Fail(key, now.Add(10*time.Second))
	l.Fail(key, now.Add(2*time.Minute))
	if l.Blocked(key, now.Add(2*time.Minute)) {
		t.Fatal("expected failure window to reset, old failures must not count")
	}
}

func TestLimiterReset(t *testing.T) {
	l := NewLimiter(1, time.Minute, time.Minute)
	now := time.Now()
	key := "k"
	l.Fail(key, now)
	if !l.Blocked(key, now) {
		t.Fatal("expected blocked")
	}
	l.Reset(key)
	if l.Blocked(key, now) {
		t.Fatal("expected reset to clear block")
	}
}

func TestLimiterKeysIndependent(t *testing.T) {
	l := NewLimiter(2, time.Minute, time.Minute)
	now := time.Now()
	l.Fail("a|root", now)
	l.Fail("b|root", now)
	l.Fail("a|root", now.Add(time.Second))
	if l.Blocked("b|root", now.Add(2*time.Second)) {
		t.Fatal("keys must be tracked independently")
	}
	if !l.Blocked("a|root", now.Add(2*time.Second)) {
		t.Fatal("expected first key blocked")
	}
}
