package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"vps-node/internal/adminauth"
	"vps-node/internal/config"
	"vps-node/internal/db"
	"vps-node/internal/repo"
	"vps-node/internal/web"
)

const (
	testAdminUser = "root"
	testAdminPass = "super-secret-password"
)

type panelEnv struct {
	t      *testing.T
	ts     *httptest.Server
	repo   *repo.Repo
	appKey []byte
}

func newPanelEnv(t *testing.T) *panelEnv {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("app key: %v", err)
	}
	handler, err := web.New(web.Options{
		DB:            d.DB,
		Repo:          repo.New(d.DB),
		Sessions:      adminauth.NewSessions(d.DB, time.Hour, false),
		Limiter:       adminauth.NewLimiter(1000, time.Minute, time.Minute),
		AppKey:        key,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		AdminUsername: testAdminUser,
		AdminPassword: testAdminPass,
	})
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return &panelEnv{t: t, ts: ts, repo: repo.New(d.DB), appKey: key}
}

func (e *panelEnv) do(method, path string, body any, cookie *http.Cookie) (*http.Response, map[string]any) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal: %v", err)
		}
		rd = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, e.ts.URL+path, rd)
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("read body: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode >= 300 {
		e.t.Fatalf("%s %s: status %d body %s", method, path, resp.StatusCode, raw)
	}
	return resp, out
}

func (e *panelEnv) login() *http.Cookie {
	e.t.Helper()
	resp, _ := e.do("POST", "/api/admin/login", map[string]string{
		"username": testAdminUser, "password": testAdminPass,
	}, nil)
	if resp.StatusCode != http.StatusOK {
		e.t.Fatal("login failed")
	}
	for _, c := range resp.Cookies() {
		if c.Name == adminauth.CookieName {
			return c
		}
	}
	e.t.Fatal("no session cookie")
	return nil
}

func (e *panelEnv) seedScenario(cookie *http.Cookie) (serverID, nodeID, userID int64) {
	e.t.Helper()
	_, out := e.do("POST", "/api/servers", map[string]string{
		"name": "HK-01", "address": "hk01.example.com",
	}, cookie)
	serverID = int64(out["id"].(float64))

	_, out = e.do("POST", "/api/nodes", map[string]any{
		"server_id": serverID,
		"name":      "HK-VLESS",
		"protocol":  "vless",
		"port":      443,
		"settings": map[string]any{
			"private_key":  "e2e-test-private-key",
			"server_names": []string{"example.com"},
			"short_id":     "0123abcd",
		},
	}, cookie)
	nodeID = int64(out["id"].(float64))

	in30Days := time.Now().AddDate(0, 0, 30).UTC().Format(time.RFC3339)
	_, out = e.do("POST", "/api/users", map[string]any{
		"username":    "e2euser",
		"quota_bytes": 1 << 30,
		"started_at":  time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		"expires_at":  in30Days,
		"node_ids":    []int64{nodeID},
	}, cookie)
	userID = int64(out["id"].(float64))
	return serverID, nodeID, userID
}

func installFakeSingBox(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "fake_singbox.sh"))
	if err != nil {
		t.Fatalf("read fake sing-box script: %v", err)
	}
	bin := filepath.Join(t.TempDir(), "fake-sing-box")
	if err := os.WriteFile(bin, src, 0o755); err != nil {
		t.Fatalf("write fake sing-box: %v", err)
	}
	if out, err := exec.Command(bin, "version").CombinedOutput(); err != nil {
		t.Fatalf("fake sing-box not runnable: %v: %s", err, out)
	}
	return bin
}

func agentConfigForTest(panelURL, registerToken string) *config.Agent {
	return &config.Agent{
		PanelURL:      panelURL,
		RegisterToken: registerToken,
		ServerID:      1,
		Collection:    config.Collection{Traffic: true, Sessions: true, ConnectionLogs: true},
	}
}
