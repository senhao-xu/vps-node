package singbox

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalConfig = `{"log":{"disabled":true},"outbounds":[{"type":"direct","tag":"direct"}]}`

func TestRuntimeStartReloadStop(t *testing.T) {
	runtime := NewRuntime()
	if err := runtime.Start([]byte(minimalConfig), nil); err != nil {
		t.Fatalf("start: %v", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Traffic == nil {
		t.Fatal("snapshot traffic map must not be nil")
	}
	if err := runtime.Start([]byte(minimalConfig), nil); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("second stop must be a no-op: %v", err)
	}
	if snapshot := runtime.Snapshot(); len(snapshot.Traffic) != 0 {
		t.Fatalf("stopped runtime must expose empty traffic, got %+v", snapshot.Traffic)
	}
}

func TestRuntimeRejectsInvalidConfig(t *testing.T) {
	runtime := NewRuntime()
	if err := runtime.Start([]byte(`{"outbounds":`), nil); err == nil {
		t.Fatal("malformed json must be rejected")
	}
	if err := runtime.Start([]byte(`{"inbounds":[{"type":"missing-protocol"}]}`), nil); err == nil {
		t.Fatal("unknown inbound type must be rejected")
	}
}

func TestValidateAcceptsMinimalConfig(t *testing.T) {
	if err := Validate([]byte(minimalConfig)); err != nil {
		t.Fatalf("validate minimal config: %v", err)
	}
	if err := Validate([]byte(`{"inbounds":`)); err == nil {
		t.Fatal("validate must reject malformed json")
	}
}

func TestPrepareConfigStripsExperimental(t *testing.T) {
	prepared, err := prepareConfig([]byte(`{"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090"}},"outbounds":[]}`))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if got := string(prepared); got != `{"outbounds":[]}` {
		t.Fatalf("experimental must be stripped, got %s", got)
	}
	if _, err := prepareConfig([]byte(`{"outbounds":[]}`)); err != nil {
		t.Fatalf("config without experimental: %v", err)
	}
}

func TestRuntimeReportsBothApplyAndRestorationFailures(t *testing.T) {
	tlsServer := httptest.NewTLSServer(nil)
	t.Cleanup(tlsServer.Close)
	certificate := tlsServer.TLS.Certificates[0]
	keyDER, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(`{"log":{"disabled":true},"inbounds":[{"type":"http","listen":"127.0.0.1","listen_port":%d,"users":[{"username":"u-1","password":"p1"}],"tls":{"enabled":true,"certificate_path":%q,"key_path":%q}}],"outbounds":[{"type":"direct"}]}`, freePort(t), certPath, keyPath)
	runtime := NewRuntime()
	if err := runtime.Start([]byte(config), nil); err != nil {
		t.Fatalf("initial TLS start: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Stop() })
	if err := os.Remove(certPath); err != nil {
		t.Fatal(err)
	}
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	badConfig := fmt.Sprintf(`{"log":{"disabled":true},"inbounds":[{"type":"http","listen":"127.0.0.1","listen_port":%d,"users":[{"username":"u-1","password":"p1"}]}],"outbounds":[{"type":"direct"}]}`, occupied.Addr().(*net.TCPAddr).Port)
	err = runtime.Reload([]byte(badConfig), nil)
	if err == nil || !strings.Contains(err.Error(), "address already in use") || !strings.Contains(err.Error(), "restore previous sing-box config") || !strings.Contains(err.Error(), certPath) {
		t.Fatalf("error must include the original bind failure and missing rollback certificate, got %v", err)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("stop after failed restoration: %v", err)
	}
	if runtime.lastConfig != nil || runtime.lastUsers != nil || runtime.tracker != nil {
		t.Fatal("explicit stop must discard restoration state")
	}
}
