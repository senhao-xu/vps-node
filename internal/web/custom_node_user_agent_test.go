package web_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"vps-node/internal/subscription"
)

// uaUpstream returns a subscription server that records the last received
// User-Agent and serves a single ss entry.
func uaUpstream(t *testing.T, seen *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.UserAgent())
		payload := "ss://aes-128-gcm:secret@up.example.com:443#up-node\n"
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCustomNodeUserAgentCreateListAndRefresh(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	var seen atomic.Value
	upstream := uaUpstream(t, &seen)

	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "ua-sub", "source_type": "subscription", "content": upstream.URL,
		"user_agent": "Mihomo/1.18.0",
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	created := jsonMap(t, body)
	if created["user_agent"] != "Mihomo/1.18.0" {
		t.Fatalf("create user_agent: %s", body)
	}
	id := int64(created["id"].(float64))

	resp, body = e.do(t, "GET", "/api/custom-nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %s", resp.StatusCode, body)
	}
	item := jsonMap(t, body)["items"].([]any)[0].(map[string]any)
	if item["user_agent"] != "Mihomo/1.18.0" {
		t.Fatalf("list user_agent: %s", body)
	}

	// Refresh sends the configured User-Agent upstream.
	resp, body = e.do(t, "POST", "/api/custom-nodes/"+formatID(id)+"/refresh", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh: %d %s", resp.StatusCode, body)
	}
	if got := seen.Load(); got != "Mihomo/1.18.0" {
		t.Fatalf("upstream user agent during refresh: %v", got)
	}

	// Changing the User-Agent clears the upstream cache and the next refresh
	// carries the new value.
	resp, body = e.do(t, "PUT", "/api/custom-nodes/"+formatID(id), map[string]any{
		"user_agent": "sing-box/1.10.0",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update ua: %d %s", resp.StatusCode, body)
	}
	updated := jsonMap(t, body)
	if updated["user_agent"] != "sing-box/1.10.0" {
		t.Fatalf("update user_agent: %s", body)
	}
	if updated["has_cache"] != false {
		t.Fatalf("ua change must clear cache: %s", body)
	}
	resp, body = e.do(t, "POST", "/api/custom-nodes/"+formatID(id)+"/refresh", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh after update: %d %s", resp.StatusCode, body)
	}
	if got := seen.Load(); got != "sing-box/1.10.0" {
		t.Fatalf("upstream user agent after update: %v", got)
	}

	// Omitting user_agent keeps the stored value.
	resp, body = e.do(t, "PUT", "/api/custom-nodes/"+formatID(id), map[string]any{"name": "ua-sub"}, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["user_agent"] != "sing-box/1.10.0" {
		t.Fatalf("update without ua: %d %s", resp.StatusCode, body)
	}

	// An explicit empty string restores the default-UA behavior.
	resp, body = e.do(t, "PUT", "/api/custom-nodes/"+formatID(id), map[string]any{"user_agent": ""}, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["user_agent"] != "" {
		t.Fatalf("clear ua: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "POST", "/api/custom-nodes/"+formatID(id)+"/refresh", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh default ua: %d %s", resp.StatusCode, body)
	}
	if got := seen.Load(); got != subscription.DefaultUserAgent {
		t.Fatalf("default user agent: %v want %s", got, subscription.DefaultUserAgent)
	}
}

func TestCustomNodeUserAgentRenderLazyFetch(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	user := e.seedUser(t, "ua-render")

	var seen atomic.Value
	upstream := uaUpstream(t, &seen)

	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "ua-render-sub", "source_type": "subscription", "content": upstream.URL,
		"user_agent": "Shadowrocket/2.2.30",
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	id := int64(jsonMap(t, body)["id"].(float64))

	if resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{"custom_node_ids": []int64{id}}, cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("authorize: %d %s", resp.StatusCode, body)
	}
	if resp, body = e.do(t, "POST", "/api/users/"+formatID(user)+"/subscription", nil, cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("subscription: %d %s", resp.StatusCode, body)
	}
	link := extractPath(subscriptionURL(t, body))

	resp, body = e.do(t, "GET", link+"?flag=general", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("render: %d %s", resp.StatusCode, body)
	}
	decoded, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(string(decoded), "up-node") {
		t.Fatalf("custom node must render: %s", decoded)
	}
	if got := seen.Load(); got != "Shadowrocket/2.2.30" {
		t.Fatalf("upstream user agent during render: %v", got)
	}
}

func TestCustomNodeUserAgentValidationAndLinks(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	// links ignores the User-Agent entirely.
	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "links-ua", "source_type": "links",
		"content": "ss://aes-128-gcm:secret@1.2.3.4:8388#ok", "user_agent": "Mihomo/1.18.0",
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create links: %d %s", resp.StatusCode, body)
	}
	linksID := int64(jsonMap(t, body)["id"].(float64))
	if jsonMap(t, body)["user_agent"] != "" {
		t.Fatalf("links must not store a user_agent: %s", body)
	}
	resp, body = e.do(t, "PUT", "/api/custom-nodes/"+formatID(linksID), map[string]any{"user_agent": "Mihomo/1.18.0"}, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["user_agent"] != "" {
		t.Fatalf("links update must ignore ua: %d %s", resp.StatusCode, body)
	}

	for name, payload := range map[string]map[string]any{
		"newline":  {"name": "bad-nl", "source_type": "subscription", "content": "https://example.com/sub", "user_agent": "bad\r\nInjected: 1"},
		"control":  {"name": "bad-ctl", "source_type": "subscription", "content": "https://example.com/sub", "user_agent": "bad\x01ua"},
		"too long": {"name": "bad-long", "source_type": "subscription", "content": "https://example.com/sub", "user_agent": strings.Repeat("a", 256)},
	} {
		resp, body = e.do(t, "POST", "/api/custom-nodes", payload, cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
			t.Fatalf("%s create: %d %s", name, resp.StatusCode, body)
		}
	}

	resp, body = e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "valid-ua", "source_type": "subscription", "content": "https://example.com/sub",
		"user_agent": "v2rayN/6.60",
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create valid: %d %s", resp.StatusCode, body)
	}
	id := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "PUT", "/api/custom-nodes/"+formatID(id), map[string]any{"user_agent": "bad\nua"}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("invalid update: %d %s", resp.StatusCode, body)
	}
	if resp, body = e.do(t, "GET", "/api/custom-nodes", nil, cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %s", resp.StatusCode, body)
	}
}
