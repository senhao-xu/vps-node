//go:build integration

package e2e

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"vps-node/internal/agentclient"
	"vps-node/internal/agentruntime"
	"vps-node/internal/agentstate"
)

func requireSingBox(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("sing-box")
	if err != nil {
		t.Skip("sing-box binary not found in PATH; integration test skipped")
	}
	out, err := exec.Command(bin, "version").CombinedOutput()
	if err != nil {
		t.Skipf("sing-box binary not runnable: %v: %s", err, out)
	}
	t.Logf("pinned sing-box: %s", string(out))
	return bin
}

func seedProtocols(t *testing.T) (env *panelEnv, cookie *http.Cookie, registerToken string) {
	t.Helper()
	env = newPanelEnv(t)
	cookie = env.login()

	_, out := env.do("POST", "/api/servers", map[string]string{
		"name": "FULL-01", "address": "full01.example.com",
	}, cookie)
	serverID := int64(out["id"].(float64))

	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("x25519: %v", err)
	}
	privateKey := base64.RawURLEncoding.EncodeToString(key.Bytes())

	now := time.Now()
	cert, tlsKey := e2eTLSMaterial(t, "full01.example.com", now.Add(-time.Hour), now.Add(24*time.Hour))

	_, out = env.do("POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "SS", "protocol": "shadowsocks", "port": 8388,
		"settings": map[string]any{"method": "2022-blake3-aes-128-gcm"},
	}, cookie)
	ssID := int64(out["id"].(float64))

	_, out = env.do("POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "VLESS", "protocol": "vless", "port": 443,
		"settings": map[string]any{
			"private_key":  privateKey,
			"server_names": []string{"example.com"},
			"short_id":     "0123abcd",
		},
	}, cookie)
	vlessID := int64(out["id"].(float64))

	_, out = env.do("POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "HY2", "protocol": "hysteria2", "port": 8443,
		"settings": map[string]any{
			"server_name": "full01.example.com", "certificate": cert, "private_key": tlsKey,
			"up_mbps": 100, "down_mbps": 200,
		},
	}, cookie)
	hy2ID := int64(out["id"].(float64))

	_, out = env.do("POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "ANYTLS", "protocol": "anytls", "port": 8444,
		"settings": map[string]any{
			"server_name": "full01.example.com", "certificate": cert, "private_key": tlsKey,
		},
	}, cookie)
	anytlsID := int64(out["id"].(float64))

	expires := time.Now().AddDate(0, 0, 30).UTC().Format(time.RFC3339)
	for _, nodeIDs := range [][]int64{{ssID, vlessID, hy2ID, anytlsID}, {vlessID}} {
		_, out = env.do("POST", "/api/users", map[string]any{
			"quota_bytes": 1 << 30,
			"started_at":  time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
			"expires_at":  expires,
			"node_ids":    nodeIDs,
		}, cookie)
	}

	_, out = env.do("POST", fmt.Sprintf("/api/servers/%d/register-token", serverID), nil, cookie)
	registerToken, _ = out["register_token"].(string)
	if registerToken == "" {
		t.Fatal("expected register_token")
	}
	return env, cookie, registerToken
}

func e2eTLSMaterial(t *testing.T, host string, notBefore, notAfter time.Time) (string, string) {
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

func TestSingBoxCheckOnRenderedPanelPayload(t *testing.T) {
	bin := requireSingBox(t)
	env, _, registerToken := seedProtocols(t)

	state, err := agentstate.Load(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	client, _ := agentclient.New(env.ts.URL)

	loop := agentruntime.NewLoop(agentruntime.LoopOptions{
		Config: agentConfigForTest(env.ts.URL, registerToken),
		Client: client, State: state, StatePath: "unused",
		Applier: agentruntime.NewApplier(filepath.Join(t.TempDir(), "config.json"),
			agentruntime.NewSingBoxChecker(bin), nil, nil),
		Metrics: nil, Version: "0.1.0-test",
	})
	if err := loop.EnsureIdentity(context.Background()); err != nil {
		t.Fatalf("register: %v", err)
	}

	cfgResp, err := client.Config(context.Background(), 0)
	if err != nil {
		t.Fatalf("config poll: %v", err)
	}
	if cfgResp.Status != "updated" || cfgResp.Config == nil {
		t.Fatalf("unexpected config response %+v", cfgResp.Status)
	}
	t.Logf("renderer_version=%s users=%d revision=%d", cfgResp.RendererVersion, len(cfgResp.Users), cfgResp.Revision)
	if len(cfgResp.Users) != 2 {
		t.Fatalf("expected 2 eligible users, got %d", len(cfgResp.Users))
	}

	tmp := filepath.Join(t.TempDir(), "rendered.json")
	if err := os.WriteFile(tmp, cfgResp.Config.Singbox, 0o600); err != nil {
		t.Fatal(err)
	}
	checker := agentruntime.NewSingBoxChecker(bin)
	if err := checker.Check(tmp); err != nil {
		t.Fatalf("real sing-box check rejected panel-rendered config: %v", err)
	}
	t.Log("sing-box check accepted the rendered configuration")
}

func TestAgentApplyFlowWithRealSingBox(t *testing.T) {
	bin := requireSingBox(t)
	dir := t.TempDir()
	active := filepath.Join(dir, "config.json")
	applier := agentruntime.NewApplier(active, agentruntime.NewSingBoxChecker(bin), nil, nil)
	ctx := context.Background()

	valid := []byte(`{"log":{"level":"info"},"outbounds":[{"type":"direct","tag":"direct"}]}`)
	if err := applier.Apply(ctx, valid); err != nil {
		t.Fatalf("apply valid config: %v", err)
	}

	invalid := []byte(`{"this is not valid json"`)
	if err := applier.Apply(ctx, invalid); err == nil {
		t.Fatal("expected check failure for invalid config")
	}
	stored, err := os.ReadFile(active)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(valid) {
		t.Fatalf("previous config must survive failed apply, got %q", stored)
	}
}
