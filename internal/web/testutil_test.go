package web_test

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"vps-node/internal/adminauth"
	"vps-node/internal/db"
	"vps-node/internal/httpx"
	"vps-node/internal/repo"
	"vps-node/internal/web"
)

func testRealityPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate reality key: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(key.Bytes())
}

func testTLSMaterial(t *testing.T, host string, notBefore, notAfter time.Time) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: new(big.Int).SetInt64(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: notBefore, NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	private := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return string(cert), string(private)
}

const (
	testAdminUser = "root"
	testAdminPass = "super-secret-password"
)

type testEnv struct {
	ts     *httptest.Server
	repo   *repo.Repo
	db     *db.DB
	appKey []byte
}

func newTestEnvWithLogger(t *testing.T) (*testEnv, *bytes.Buffer) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
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
	var logs bytes.Buffer
	handler, err := web.New(web.Options{DB: d.DB, Repo: repo.New(d.DB), Sessions: adminauth.NewSessions(d.DB, time.Hour, false), Limiter: adminauth.NewLimiter(3, time.Minute, time.Minute), AppKey: key, Logger: slog.New(slog.NewTextHandler(&logs, nil)), AdminUsername: testAdminUser, AdminPassword: testAdminPass})
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	ts := httptest.NewServer(httpx.WrapHandler(handler, slog.New(slog.NewTextHandler(&logs, nil))))
	t.Cleanup(ts.Close)
	return &testEnv{ts: ts, repo: repo.New(d.DB), db: d, appKey: key}, &logs
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	return newTestEnvWith(t, false)
}

func newTestEnvWith(t *testing.T, secure bool) *testEnv {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
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
		Sessions:      adminauth.NewSessions(d.DB, time.Hour, secure),
		Limiter:       adminauth.NewLimiter(3, time.Minute, time.Minute),
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
	return &testEnv{ts: ts, repo: repo.New(d.DB), db: d, appKey: key}
}

func (e *testEnv) do(t *testing.T, method, path string, body any, cookie *http.Cookie) (*http.Response, string) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, e.ts.URL+path, rd)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, string(raw)
}

func (e *testEnv) doWithHeader(t *testing.T, method, path string, body any, header, value string) (*http.Response, string) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, e.ts.URL+path, rd)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(header, value)
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, string(raw)
}

func (e *testEnv) login(t *testing.T) *http.Cookie {
	t.Helper()
	resp, body := e.do(t, "POST", "/api/admin/login",
		map[string]string{"username": testAdminUser, "password": testAdminPass}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d body %s", resp.StatusCode, body)
	}
	for _, c := range resp.Cookies() {
		if c.Name == adminauth.CookieName {
			return c
		}
	}
	t.Fatal("login response missing session cookie")
	return nil
}

func (e *testEnv) seedServer(t *testing.T, name string) int64 {
	t.Helper()
	id, err := e.repo.CreateServer(context.Background(), name, repo.ServerStatusActive)
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}
	return id
}

func (e *testEnv) seedNode(t *testing.T, serverID int64, name string, port int) int64 {
	t.Helper()
	id, err := e.repo.CreateNode(context.Background(), repo.NewNode{
		ServerID: serverID, Address: name + ".example.com", Name: name, Protocol: repo.ProtocolVLESS, Port: port,
	})
	if err != nil {
		t.Fatalf("seed node: %v", err)
	}
	return id
}

func (e *testEnv) seedUser(t *testing.T, uuid string) int64 {
	t.Helper()
	id, err := e.repo.CreateUser(context.Background(), repo.NewUser{
		UUID: uuid, Username: "user-" + uuid, TokenHash: "hash-" + uuid, Status: repo.UserStatusActive, TransferEnable: 1000,
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func (e *testEnv) revision(t *testing.T, serverID int64) int64 {
	t.Helper()
	rev, err := e.repo.GetServerRevision(context.Background(), serverID)
	if err != nil {
		t.Fatalf("get revision: %v", err)
	}
	return rev
}

func jsonMap(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("decode json %q: %v", body, err)
	}
	return m
}

func errorCode(t *testing.T, body string) string {
	t.Helper()
	m := jsonMap(t, body)
	errObj, ok := m["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error envelope, got %s", body)
	}
	code, _ := errObj["code"].(string)
	return code
}

func assertNoSecrets(t *testing.T, body string, extra ...string) {
	t.Helper()
	forbidden := append([]string{"token_hash", "password_hash", "password", "secret_enc", "register_token_hash", "private_key"}, extra...)
	for _, f := range forbidden {
		if bytes.Contains([]byte(body), []byte(f)) {
			t.Fatalf("response body must not contain %q: %s", f, body)
		}
	}
}
