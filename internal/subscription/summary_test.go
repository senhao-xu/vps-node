package subscription

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestSummarizeShareLinks(t *testing.T) {
	vmessPayload, err := json.Marshal(map[string]any{
		"ps": "VM-1", "add": "9.9.9.9", "port": 10086,
		"id": "11111111-1111-1111-1111-111111111111", "net": "tcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	links := []string{
		"ss://aes-128-gcm:secret@1.2.3.4:8388#HK-1",
		"vless://11111111-1111-1111-1111-111111111111@5.6.7.8:443?security=tls&sni=example.com#JP-1",
		"vmess://" + base64.StdEncoding.EncodeToString(vmessPayload),
		"not-a-link",
	}
	summaries, skipped := Summarize(links, nil)
	want := []NodeSummary{
		{Name: "HK-1", Type: "ss", Server: "1.2.3.4", Port: 8388},
		{Name: "JP-1", Type: "vless", Server: "5.6.7.8", Port: 443},
		{Name: "VM-1", Type: "vmess", Server: "9.9.9.9", Port: 10086},
	}
	if len(summaries) != len(want) {
		t.Fatalf("summaries = %+v, want %+v", summaries, want)
	}
	for i := range want {
		if summaries[i] != want[i] {
			t.Fatalf("summary %d = %+v, want %+v", i, summaries[i], want[i])
		}
	}
	if len(skipped) != 1 || skipped[0] != "not-a-link" {
		t.Fatalf("skipped = %v", skipped)
	}
}

func TestSummarizeClashProxies(t *testing.T) {
	// yaml.v3 decodes integer ports as int; upstream templates may also use
	// strings or floats, so every shape must convert through looseInt.
	proxies := []map[string]any{
		{"name": "P1", "type": "trojan", "server": "p1.example.com", "port": 443},
		{"name": "P2", "type": "ss", "server": "p2.example.com", "port": "8443"},
		{"name": "P3", "type": "hysteria2", "server": "p3.example.com", "port": float64(2096)},
		{"type": "ss", "server": "no-name.example.com", "port": 80},
		{"name": "bad-port", "type": "ss", "server": "p4.example.com", "port": "abc"},
		{"name": "no-server", "type": "ss", "port": 80},
	}
	summaries, skipped := Summarize(nil, proxies)
	want := []NodeSummary{
		{Name: "P1", Type: "trojan", Server: "p1.example.com", Port: 443},
		{Name: "P2", Type: "ss", Server: "p2.example.com", Port: 8443},
		{Name: "P3", Type: "hysteria2", Server: "p3.example.com", Port: 2096},
	}
	if len(summaries) != len(want) {
		t.Fatalf("summaries = %+v, want %+v", summaries, want)
	}
	for i := range want {
		if summaries[i] != want[i] {
			t.Fatalf("summary %d = %+v, want %+v", i, summaries[i], want[i])
		}
	}
	wantSkipped := []string{"proxy #3", "bad-port", "no-server"}
	if len(skipped) != len(wantSkipped) {
		t.Fatalf("skipped = %v, want %v", skipped, wantSkipped)
	}
	for i := range wantSkipped {
		if skipped[i] != wantSkipped[i] {
			t.Fatalf("skipped[%d] = %q, want %q", i, skipped[i], wantSkipped[i])
		}
	}
}

func TestSummarizeEmpty(t *testing.T) {
	summaries, skipped := Summarize(nil, nil)
	if len(summaries) != 0 || len(skipped) != 0 {
		t.Fatalf("expected empty result, got %v / %v", summaries, skipped)
	}
	if summaries == nil || skipped == nil {
		t.Fatal("Summarize must return non-nil slices so JSON encodes []")
	}
}
