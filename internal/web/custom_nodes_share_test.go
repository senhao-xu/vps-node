package web_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCustomNodeShareLinksAndContent(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	const link = "ss://aes-128-gcm:secret@ext.example.com:8388#ext-node"
	const content = link + "\nbad line"
	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "share-links", "source_type": "links", "content": content,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	id := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "GET", "/api/custom-nodes/"+formatID(id)+"/share", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("share: %d %s", resp.StatusCode, body)
	}
	share := jsonMap(t, body)
	if share["source_type"] != "links" {
		t.Fatalf("source_type: %s", body)
	}
	if share["has_cache"] != false || share["fetched_at"] != nil {
		t.Fatalf("links share must report no cache: %s", body)
	}
	clash, _ := share["clash"].(string)
	if !strings.Contains(clash, "proxies:") || !strings.Contains(clash, "ext.example.com") || !strings.Contains(clash, "ext-node") {
		t.Fatalf("clash fragment: %q", clash)
	}
	var fragment map[string]any
	if err := yaml.Unmarshal([]byte(clash), &fragment); err != nil {
		t.Fatalf("clash fragment is not valid yaml: %v (%q)", err, clash)
	}
	if len(fragment["proxies"].([]any)) != 1 {
		t.Fatalf("clash fragment must contain one proxy: %q", clash)
	}
	links, _ := share["links"].([]any)
	if len(links) != 2 || links[0] != link || links[1] != "bad line" {
		t.Fatalf("links must be the plaintext lines: %s", body)
	}
	skipped, _ := share["skipped"].([]any)
	if len(skipped) != 1 || skipped[0] != "bad line" {
		t.Fatalf("skipped: %s", body)
	}

	resp, body = e.do(t, "GET", "/api/custom-nodes/"+formatID(id)+"/content", nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["content"] != content {
		t.Fatalf("content: %d %s", resp.StatusCode, body)
	}

	// The list endpoint must never expose the plaintext content.
	resp, body = e.do(t, "GET", "/api/custom-nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %s", resp.StatusCode, body)
	}
	if strings.Contains(body, "secret") || strings.Contains(body, "ext.example.com") {
		t.Fatalf("list must not echo content: %s", body)
	}
	for _, raw := range jsonMap(t, body)["items"].([]any) {
		if _, ok := raw.(map[string]any)["content"]; ok {
			t.Fatalf("list item must not carry a content field: %s", body)
		}
	}
}

func TestCustomNodeShareSubscriptionSources(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	var hits atomic.Int64
	linksUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		payload := base64.StdEncoding.EncodeToString([]byte("ss://aes-128-gcm:secret@up.example.com:443#up-node\n"))
		_, _ = w.Write([]byte(payload))
	}))
	defer linksUpstream.Close()

	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "share-sub-links", "source_type": "subscription", "content": linksUpstream.URL,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create sub links: %d %s", resp.StatusCode, body)
	}
	subID := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "GET", "/api/custom-nodes/"+formatID(subID)+"/share", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("share subscription: %d %s", resp.StatusCode, body)
	}
	share := jsonMap(t, body)
	if share["has_cache"] != true || share["fetched_at"] == nil {
		t.Fatalf("lazy fetch must populate the cache: %s", body)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected one upstream fetch, got %d", hits.Load())
	}
	links, _ := share["links"].([]any)
	if len(links) != 1 || !strings.Contains(links[0].(string), "up-node") {
		t.Fatalf("subscription share links: %s", body)
	}
	clash, _ := share["clash"].(string)
	if !strings.Contains(clash, "up.example.com") {
		t.Fatalf("subscription share clash: %q", clash)
	}

	// The subscription plaintext is the upstream URL.
	resp, body = e.do(t, "GET", "/api/custom-nodes/"+formatID(subID)+"/content", nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["content"] != linksUpstream.URL {
		t.Fatalf("subscription content: %d %s", resp.StatusCode, body)
	}

	// A Clash-YAML-only upstream yields proxies but no link lines.
	clashUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("proxies:\n" +
			"  - {name: UP-A, type: ss, server: clash.example.com, port: 443, cipher: aes-128-gcm, password: pw}\n"))
	}))
	defer clashUpstream.Close()
	resp, body = e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "share-sub-clash", "source_type": "subscription", "content": clashUpstream.URL,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create sub clash: %d %s", resp.StatusCode, body)
	}
	clashID := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "GET", "/api/custom-nodes/"+formatID(clashID)+"/share", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("share clash subscription: %d %s", resp.StatusCode, body)
	}
	share = jsonMap(t, body)
	links, _ = share["links"].([]any)
	if len(links) != 0 {
		t.Fatalf("clash-only upstream must have no link lines: %s", body)
	}
	clash, _ = share["clash"].(string)
	if !strings.Contains(clash, "clash.example.com") || !strings.Contains(clash, "UP-A") {
		t.Fatalf("clash-only upstream proxies: %q", clash)
	}
}

func TestCustomNodeShareAuthAndNotFound(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	const link = "ss://aes-128-gcm:secret@auth.example.com:1#auth"
	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "share-auth", "source_type": "links", "content": link,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	id := int64(jsonMap(t, body)["id"].(float64))

	for _, suffix := range []string{"/share", "/content"} {
		resp, _ = e.do(t, "GET", "/api/custom-nodes/"+formatID(id)+suffix, nil, nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s: %d", suffix, resp.StatusCode)
		}
		resp, body = e.do(t, "GET", "/api/custom-nodes/99999"+suffix, nil, cookie)
		if resp.StatusCode != http.StatusNotFound || errorCode(t, body) != "not_found" {
			t.Fatalf("unknown id %s: %d %s", suffix, resp.StatusCode, body)
		}
	}
}
