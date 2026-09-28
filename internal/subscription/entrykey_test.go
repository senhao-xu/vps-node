package subscription

import (
	"encoding/hex"
	"testing"
)

func mustParseProxy(t *testing.T, link string) map[string]any {
	t.Helper()
	proxy, err := ParseShareURI(link)
	if err != nil {
		t.Fatalf("parse %q: %v", link, err)
	}
	return proxy
}

func TestEntryKeyIgnoresName(t *testing.T) {
	appKey := []byte("0123456789abcdef0123456789abcdef")
	original := mustParseProxy(t, "ss://aes-128-gcm:secret@a.example.com:8388#A")
	renamed := mustParseProxy(t, "ss://aes-128-gcm:secret@a.example.com:8388#A-renamed")

	key, err := EntryKey(appKey, original)
	if err != nil {
		t.Fatalf("entry key: %v", err)
	}
	if len(key) != 64 {
		t.Fatalf("key must be 64 hex chars, got %q", key)
	}
	if _, err := hex.DecodeString(key); err != nil {
		t.Fatalf("key must be hex: %v", err)
	}
	renamedKey, err := EntryKey(appKey, renamed)
	if err != nil {
		t.Fatalf("renamed entry key: %v", err)
	}
	if key != renamedKey {
		t.Fatalf("key must not depend on the name: %q != %q", key, renamedKey)
	}
}

func TestEntryKeyDependsOnConnectionAndAppKey(t *testing.T) {
	appKey := []byte("0123456789abcdef0123456789abcdef")
	base := mustParseProxy(t, "ss://aes-128-gcm:secret@a.example.com:8388#A")
	otherServer := mustParseProxy(t, "ss://aes-128-gcm:secret@b.example.com:8388#A")
	otherPort := mustParseProxy(t, "ss://aes-128-gcm:secret@a.example.com:8389#A")

	baseKey, _ := EntryKey(appKey, base)
	serverKey, _ := EntryKey(appKey, otherServer)
	portKey, _ := EntryKey(appKey, otherPort)
	if baseKey == serverKey || baseKey == portKey {
		t.Fatalf("different connection parameters must yield different keys: %q %q %q", baseKey, serverKey, portKey)
	}

	otherAppKey, _ := EntryKey([]byte("fedcba9876543210fedcba9876543210"), base)
	if baseKey == otherAppKey {
		t.Fatal("a different appKey must yield a different key")
	}
}

func TestLinkEntryKeyUnparseable(t *testing.T) {
	if _, ok := LinkEntryKey([]byte("key"), "not-a-link"); ok {
		t.Fatal("unparseable link must not have a key")
	}
	key, ok := LinkEntryKey([]byte("key"), "ss://aes-128-gcm:secret@a.example.com:8388#A")
	if !ok || len(key) != 64 {
		t.Fatalf("parsable link must yield a 64-char key, got %q ok=%v", key, ok)
	}
}

func TestEntryKeyHandlesNestedMapsDeterministically(t *testing.T) {
	appKey := []byte("0123456789abcdef0123456789abcdef")
	proxy := map[string]any{
		"name": "P1", "type": "vmess", "server": "p1.example.com", "port": 443,
		"ws-opts": map[string]any{"path": "/ws", "headers": map[string]any{"Host": "p1.example.com"}},
	}
	first, err := EntryKey(appKey, proxy)
	if err != nil {
		t.Fatalf("entry key: %v", err)
	}
	// A shallow copy with a nested map replaced by an equal map must be stable.
	reordered := map[string]any{
		"ws-opts": map[string]any{"headers": map[string]any{"Host": "p1.example.com"}, "path": "/ws"},
		"port":    443, "server": "p1.example.com", "type": "vmess", "name": "renamed",
	}
	second, err := EntryKey(appKey, reordered)
	if err != nil {
		t.Fatalf("entry key: %v", err)
	}
	if first != second {
		t.Fatalf("nested maps must hash deterministically: %q != %q", first, second)
	}
	// Mutating the nested options must change the key.
	proxy["ws-opts"].(map[string]any)["path"] = "/other"
	changed, _ := EntryKey(appKey, proxy)
	if changed == first {
		t.Fatal("changing nested transport options must change the key")
	}
}

func TestSummarizeEntriesAttachesKeys(t *testing.T) {
	appKey := []byte("0123456789abcdef0123456789abcdef")
	links := []string{
		"ss://aes-128-gcm:secret@a.example.com:8388#A",
		"not-a-link",
	}
	proxies := []map[string]any{
		{"name": "P1", "type": "trojan", "server": "p1.example.com", "port": 443},
		{"type": "ss", "server": "no-name.example.com", "port": 80},
	}
	entries, skipped := SummarizeEntries(appKey, links, proxies)
	if len(entries) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Name != "A" || entries[0].Type != "ss" || entries[0].Server != "a.example.com" || entries[0].Port != 8388 {
		t.Fatalf("link summary: %+v", entries[0])
	}
	if entries[1].Name != "P1" || entries[1].Type != "trojan" {
		t.Fatalf("proxy summary: %+v", entries[1])
	}
	for _, entry := range entries {
		if len(entry.Key) != 64 {
			t.Fatalf("entry %q must carry a 64-char key: %+v", entry.Name, entry)
		}
	}
	if len(skipped) != 2 || skipped[0] != "not-a-link" || skipped[1] != "proxy #1" {
		t.Fatalf("skipped = %v", skipped)
	}
}

func TestResolveEntriesKeepsProxyAndLink(t *testing.T) {
	appKey := []byte("0123456789abcdef0123456789abcdef")
	links := []string{
		"ss://aes-128-gcm:secret@a.example.com:8388#A",
		"not-a-link",
	}
	proxies := []map[string]any{
		{"name": "P1", "type": "trojan", "server": "p1.example.com", "port": 443, "password": "pw"},
		{"type": "ss", "server": "no-name.example.com", "port": 80},
	}
	entries, err := ResolveEntries(appKey, links, proxies)
	if err != nil {
		t.Fatalf("resolve entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Name != "A" || entries[0].Link != links[0] || entries[0].Proxy["type"] != "ss" {
		t.Fatalf("link entry: %+v", entries[0])
	}
	if entries[1].Name != "P1" || entries[1].Link != "" || entries[1].Proxy["server"] != "p1.example.com" {
		t.Fatalf("proxy entry: %+v", entries[1])
	}
	// Keys match what SummarizeEntries reports for the same entries.
	summaries, _ := SummarizeEntries(appKey, links, proxies)
	for i, entry := range entries {
		if entry.Key != summaries[i].Key {
			t.Fatalf("resolved key %d mismatch: %q != %q", i, entry.Key, summaries[i].Key)
		}
	}
}
