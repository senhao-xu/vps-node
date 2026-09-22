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
	"testing"
	"time"

	"vps-node/internal/agentclient"
	kernelsingbox "vps-node/internal/kernel/singbox"
)

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
		"settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
	}, cookie)
	ssID := int64(out["id"].(float64))

	_, out = env.do("POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "VLESS", "protocol": "vless", "port": 443,
		"settings": map[string]any{
			"private_key":      privateKey,
			"reality_settings": map[string]any{"server_name": "example.com", "short_id": "0123abcd"},
		},
	}, cookie)
	vlessID := int64(out["id"].(float64))

	_, out = env.do("POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "HY2", "protocol": "hysteria2", "port": 8443,
		"settings": map[string]any{
			"tls":         map[string]any{"server_name": "full01.example.com"},
			"certificate": cert, "private_key": tlsKey,
			"bandwidth": map[string]any{"up": 100, "down": 200},
		},
	}, cookie)
	hy2ID := int64(out["id"].(float64))

	_, out = env.do("POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "ANYTLS", "protocol": "anytls", "port": 8444,
		"settings": map[string]any{
			"tls":         map[string]any{"server_name": "full01.example.com"},
			"certificate": cert, "private_key": tlsKey,
		},
	}, cookie)
	anytlsID := int64(out["id"].(float64))

	expires := time.Now().AddDate(0, 0, 30).UTC().Format(time.RFC3339)
	for i, nodeIDs := range [][]int64{{ssID, vlessID, hy2ID, anytlsID}, {vlessID}} {
		_, out = env.do("POST", "/api/users", map[string]any{
			"username":        fmt.Sprintf("e2euser%d", i+1),
			"transfer_enable": 1 << 30,
			"started_at":      time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
			"expires_at":      expires,
			"node_ids":        nodeIDs,
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

func registerAgent(t *testing.T, env *panelEnv, registerToken string) *agentclient.Client {
	t.Helper()
	client, err := agentclient.New(env.ts.URL)
	if err != nil {
		t.Fatalf("new agent client: %v", err)
	}
	resp, err := client.Register(context.Background(), agentclient.RegisterRequest{
		RegisterToken: registerToken, Version: "0.1.0-test",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	client.SetToken(resp.AgentToken)
	return client
}

func fetchRenderedConfig(t *testing.T, env *panelEnv, registerToken string) (*agentclient.Client, *agentclient.ConfigResponse) {
	t.Helper()
	client := registerAgent(t, env, registerToken)
	cfgResp, err := client.Config(context.Background(), 0)
	if err != nil {
		t.Fatalf("config poll: %v", err)
	}
	if cfgResp.Status != "updated" || cfgResp.Config == nil {
		t.Fatalf("unexpected config response %+v", cfgResp.Status)
	}
	if len(cfgResp.Users) != 2 {
		t.Fatalf("expected 2 eligible users, got %d", len(cfgResp.Users))
	}
	return client, cfgResp
}

func TestEmbeddedSingBoxValidatesRenderedPanelPayload(t *testing.T) {
	env, _, registerToken := seedProtocols(t)
	_, cfgResp := fetchRenderedConfig(t, env, registerToken)

	t.Logf("renderer_version=%s users=%d revision=%d", cfgResp.RendererVersion, len(cfgResp.Users), cfgResp.Revision)
	if err := kernelsingbox.Validate(cfgResp.Config.Singbox); err != nil {
		t.Fatalf("embedded sing-box rejected panel-rendered config: %v", err)
	}
	t.Log("embedded sing-box parsed the rendered configuration")

	if err := kernelsingbox.Validate([]byte(`{"this is not valid json"`)); err == nil {
		t.Fatal("expected malformed config to be rejected")
	}
}
