package web_test

import (
	"fmt"
	"net/http"
	"testing"

	"vps-node/internal/adminauth"
)

func TestLoginSuccessAndCookieFlags(t *testing.T) {
	e := newTestEnv(t)

	resp, body := e.do(t, "POST", "/api/admin/login",
		map[string]string{"username": testAdminUser, "password": testAdminPass}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status %d body %s", resp.StatusCode, body)
	}
	m := jsonMap(t, body)
	admin, ok := m["admin"].(map[string]any)
	if !ok || admin["username"] != testAdminUser {
		t.Fatalf("unexpected login payload: %s", body)
	}
	if id, ok := admin["id"].(float64); !ok || id < 1 {
		t.Fatalf("expected admin id, got %s", body)
	}

	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == adminauth.CookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("missing session cookie")
	}
	if !cookie.HttpOnly {
		t.Fatal("session cookie must be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie must be SameSite=Lax, got %v", cookie.SameSite)
	}
	if cookie.Secure {
		t.Fatal("session cookie must not be Secure when TLS is not configured")
	}

	resp, body = e.do(t, "GET", "/api/admin/me", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me status %d body %s", resp.StatusCode, body)
	}
	me := jsonMap(t, body)
	if got := me["admin"].(map[string]any)["username"]; got != testAdminUser {
		t.Fatalf("unexpected me payload: %s", body)
	}
}

func TestLoginSecureCookieWhenConfigured(t *testing.T) {
	e := newTestEnvWith(t, true)
	resp, _ := e.do(t, "POST", "/api/admin/login",
		map[string]string{"username": testAdminUser, "password": testAdminPass}, nil)
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == adminauth.CookieName {
			cookie = c
		}
	}
	if cookie == nil || !cookie.Secure {
		t.Fatal("session cookie must be Secure when TLS is configured")
	}
}

func TestLoginFailure(t *testing.T) {
	e := newTestEnv(t)

	resp, body := e.do(t, "POST", "/api/admin/login",
		map[string]string{"username": testAdminUser, "password": "wrong-password"}, nil)
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
		t.Fatalf("expected 401 unauthorized, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "POST", "/api/admin/login",
		map[string]string{"username": "no-such-user", "password": testAdminPass}, nil)
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
		t.Fatalf("expected 401 unauthorized for unknown user, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "POST", "/api/admin/login", map[string]string{"username": "root"}, nil)
	if resp.StatusCode != http.StatusBadRequest || errorCode(t, body) != "invalid_request" {
		t.Fatalf("expected 400 for missing password, got %d %s", resp.StatusCode, body)
	}
}

func TestLoginRateLimited(t *testing.T) {
	e := newTestEnv(t)

	for i := 0; i < 3; i++ {
		resp, body := e.do(t, "POST", "/api/admin/login",
			map[string]string{"username": testAdminUser, "password": "wrong-password"}, nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d %s", i+1, resp.StatusCode, body)
		}
	}

	resp, body := e.do(t, "POST", "/api/admin/login",
		map[string]string{"username": testAdminUser, "password": testAdminPass}, nil)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after repeated failures, got %d %s", resp.StatusCode, body)
	}
	if code := errorCode(t, body); code != "rate_limited" {
		t.Fatalf("expected rate_limited, got %s", code)
	}
}

func TestUnauthenticatedRequestsBlocked(t *testing.T) {
	e := newTestEnv(t)

	paths := []struct {
		method string
		path   string
	}{
		{"GET", "/api/users"},
		{"GET", "/api/servers"},
		{"GET", "/api/nodes"},
		{"GET", "/api/dashboard"},
		{"GET", "/api/settings"},
		{"GET", "/api/admin/me"},
	}
	for _, p := range paths {
		resp, body := e.do(t, p.method, p.path, nil, nil)
		if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
			t.Fatalf("%s %s: expected 401 unauthorized, got %d %s", p.method, p.path, resp.StatusCode, body)
		}
	}

	resp, body := e.do(t, "GET", "/api/users", nil, &http.Cookie{Name: adminauth.CookieName, Value: "forged"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("forged cookie: expected 401, got %d %s", resp.StatusCode, body)
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	resp, body := e.do(t, "GET", "/api/admin/me", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me before logout: %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "POST", "/api/admin/logout", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout: %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "GET", "/api/admin/me", nil, cookie)
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
		t.Fatalf("expected session invalidated after logout, got %d %s", resp.StatusCode, body)
	}

	resp, _ = e.do(t, "POST", "/api/admin/logout", nil, cookie)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("logout without session must be 401, got %d", resp.StatusCode)
	}
}

func TestAdminAndAgentAuthScopesDoNotCross(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")

	resp, body := e.do(t, "POST", fmt.Sprintf("/api/servers/%d/register-token", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register-token: %d %s", resp.StatusCode, body)
	}
	regToken := jsonMap(t, body)["register_token"].(string)
	resp, body = e.doAgent(t, "POST", "/api/agent/register",
		map[string]any{"register_token": regToken, "version": "1.0.0"}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent register: %d %s", resp.StatusCode, body)
	}
	agentToken := jsonMap(t, body)["agent_token"].(string)

	resp, body = e.do(t, "GET", "/api/agent/config?version=0", nil, cookie)
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
		t.Fatalf("admin cookie must not grant agent config access, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "POST", "/api/agent/heartbeat",
		map[string]any{"version": "1.0.0", "cpu_percent": 1.0, "memory_percent": 1.0, "disk_percent": 1.0, "uptime_seconds": 1}, cookie)
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
		t.Fatalf("admin cookie must not grant agent heartbeat access, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.doAgent(t, "GET", "/api/users", nil, agentToken)
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
		t.Fatalf("agent token must not grant admin user list access, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.doAgent(t, "GET", "/api/admin/me", nil, agentToken)
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
		t.Fatalf("agent token must not grant admin me access, got %d %s", resp.StatusCode, body)
	}
}
