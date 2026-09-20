package adminauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const CookieName = "panel_session"

var ErrInvalidSession = errors.New("invalid admin session")

type Sessions struct {
	DB     *sql.DB
	TTL    time.Duration
	Secure bool
}

func NewSessions(db *sql.DB, ttl time.Duration, secure bool) *Sessions {
	return &Sessions{DB: db, TTL: ttl, Secure: secure}
}

func NewToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

var dummyHash, dummyHashErr = bcrypt.GenerateFromPassword([]byte("panel-dummy-password"), bcrypt.DefaultCost)

func CheckDummyPassword(password string) bool {
	if dummyHashErr != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword(dummyHash, []byte(password)) == nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *Sessions) Create(ctx context.Context, adminID int64) (string, error) {
	token, err := NewToken()
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	_, err = s.DB.ExecContext(ctx,
		`INSERT INTO admin_sessions (token_hash, admin_id, created_at, last_seen_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		HashToken(token), adminID, now, now, now+int64(s.TTL/time.Second))
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Sessions) Resolve(ctx context.Context, token string) (int64, error) {
	if token == "" {
		return 0, ErrInvalidSession
	}
	hash := HashToken(token)
	var adminID, expiresAt int64
	err := s.DB.QueryRowContext(ctx,
		`SELECT admin_id, expires_at FROM admin_sessions WHERE token_hash = ?`, hash).Scan(&adminID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrInvalidSession
	}
	if err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	if now >= expiresAt {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM admin_sessions WHERE token_hash = ?`, hash)
		return 0, ErrInvalidSession
	}
	_, err = s.DB.ExecContext(ctx,
		`UPDATE admin_sessions SET last_seen_at = ?, expires_at = ? WHERE token_hash = ?`,
		now, now+int64(s.TTL/time.Second), hash)
	if err != nil {
		return 0, err
	}
	return adminID, nil
}

func (s *Sessions) Revoke(ctx context.Context, token string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM admin_sessions WHERE token_hash = ?`, HashToken(token))
	return err
}

func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   -1,
	})
}
