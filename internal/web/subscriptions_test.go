package web_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"vps-node/internal/adminauth"
	"vps-node/internal/repo"
)

func subscriptionURL(t *testing.T, body string) string {
	t.Helper()
	return jsonMap(t, body)["url"].(string)
}

func TestSubscriptionAdminLifecycleAndPublicFiltering(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	historical := e.seedUser(t, "historical")
	resp, body := e.do(t, "GET", "/api/users/"+formatID(historical)+"/subscription", nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["configured"] != false {
		t.Fatalf("historical GET: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "POST", "/api/users/"+formatID(historical)+"/subscription", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	old := subscriptionURL(t, body)
	resp, body = e.do(t, "POST", "/api/users/"+formatID(historical)+"/subscription", nil, cookie)
	if resp.StatusCode != http.StatusConflict || errorCode(t, body) != "conflict" {
		t.Fatalf("duplicate: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "POST", "/api/users/"+formatID(historical)+"/subscription/rotate", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rotate: %d %s", resp.StatusCode, body)
	}
	newURL := subscriptionURL(t, body)
	if old == newURL {
		t.Fatal("rotation did not change URL")
	}
	resp, _ = e.do(t, "GET", extractPath(old), nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("old URL status: %d", resp.StatusCode)
	}
	resp, _ = e.do(t, "GET", extractPath(newURL), nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("new URL status: %d url=%s path=%s", resp.StatusCode, newURL, extractPath(newURL))
	}
	token := strings.TrimPrefix(extractPath(newURL), "/s/")
	if len(token) != 22 {
		t.Fatalf("expected 22-char subscription token, got %d (%q)", len(token), token)
	}
}

func TestSubscriptionEligibilityAndNodeFiltering(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	server := e.seedServer(t, "sub")
	disabledServer := e.seedServer(t, "disabled-sub")
	node, err := e.repo.CreateNode(context.Background(), repo.NewNode{ServerID: server, Name: "active", Protocol: repo.ProtocolShadowsocks, Port: 443, ProtocolSettings: `{"cipher":"2022-blake3-aes-128-gcm"}`})
	if err != nil {
		t.Fatal(err)
	}
	disabledServerNode, err := e.repo.CreateNode(context.Background(), repo.NewNode{ServerID: disabledServer, Name: "disabled-server-node", Protocol: repo.ProtocolShadowsocks, Port: 445, ProtocolSettings: `{"cipher":"2022-blake3-aes-128-gcm"}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.repo.SetServerStatus(context.Background(), disabledServer, repo.ServerStatusDisabled); err != nil {
		t.Fatal(err)
	}
	disabledNode, err := e.repo.CreateNode(context.Background(), repo.NewNode{ServerID: server, Name: "disabled", Protocol: repo.ProtocolShadowsocks, Port: 444, ProtocolSettings: `{"cipher":"2022-blake3-aes-128-gcm"}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.repo.SetNodeStatus(context.Background(), disabledNode, repo.NodeStatusDisabled); err != nil {
		t.Fatal(err)
	}
	user := e.seedUser(t, "eligible")
	if err := e.repo.AuthorizeUserNode(context.Background(), user, node); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.AuthorizeUserNode(context.Background(), user, disabledServerNode); err != nil {
		t.Fatal(err)
	}
	resp, body := e.do(t, "POST", "/api/users/"+formatID(user)+"/subscription", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}
	link := subscriptionURL(t, body)
	u, _ := url.Parse(link)
	token := strings.TrimPrefix(u.Path, "/s/")
	if _, err := e.repo.ResolveSubscription(context.Background(), adminauth.HashToken(token)); err != nil {
		t.Fatalf("direct resolve: %v token=%s", err, token)
	}
	resp, body = e.do(t, "GET", extractPath(link), nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("eligible: %d %s url=%s path=%s", resp.StatusCode, body, link, extractPath(link))
	}
	if got := resp.Header.Get("Subscription-Userinfo"); got != "upload=0; download=0; total=1000" {
		t.Fatalf("subscription userinfo header: %q", got)
	}
	decoded, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("general subscription is not base64: %v", err)
	}
	if strings.Contains(string(decoded), "disabled") || strings.Contains(string(decoded), "disabled-server-node") {
		t.Fatalf("disabled node or server rendered: %s", decoded)
	}
	for _, mutate := range []func(){func() { _ = e.repo.SetUserStatus(context.Background(), user, repo.UserStatusDisabled) }, func() {
		_ = e.repo.SetUserStatus(context.Background(), user, repo.UserStatusActive)
		_ = e.repo.AddUserUsedBytes(context.Background(), user, 1000, 0)
	}} {
		mutate()
		resp, _ = e.do(t, "GET", extractPath(link), nil, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("ineligible status: %d", resp.StatusCode)
		}
	}
}

func TestSubscriptionTimeEligibilityAndInvalidToken(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	resp, body := e.do(t, "GET", "/s/not-a-token", nil, nil)
	if resp.StatusCode != http.StatusNotFound || errorCode(t, body) != "not_found" {
		t.Fatalf("invalid token: %d %s", resp.StatusCode, body)
	}
	for name, times := range map[string]map[string]string{
		"future":  {"started_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)},
		"expired": {"expires_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)},
	} {
		payload := map[string]any{"username": name}
		for k, v := range times {
			payload[k] = v
		}
		resp, body = e.do(t, "POST", "/api/users", payload, cookie)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create %s: %d %s", name, resp.StatusCode, body)
		}
		link := jsonMap(t, body)["subscription_url"].(string)
		resp, body = e.do(t, "GET", extractPath(link), nil, nil)
		if resp.StatusCode != http.StatusForbidden || errorCode(t, body) != "forbidden" {
			t.Fatalf("%s: %d %s", name, resp.StatusCode, body)
		}
	}
}

func TestSubscriptionSettingsPathOriginsAndValidation(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	valid := map[string]any{"subscribe_urls": "https://one.example, http://two.example/", "subscribe_path": "feed"}
	resp, body := e.do(t, "PUT", "/api/settings", valid, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid settings: %d %s", resp.StatusCode, body)
	}
	user := e.seedUser(t, "path-user")
	resp, body = e.do(t, "POST", "/api/users/"+formatID(user)+"/subscription", nil, cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(subscriptionURL(t, body), "/feed/") {
		t.Fatalf("URL path: %d %s", resp.StatusCode, body)
	}
	link := subscriptionURL(t, body)
	resp, _ = e.do(t, "GET", extractPath(link), nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("custom path unavailable: %d", resp.StatusCode)
	}
	oldPath := "/s/" + strings.TrimPrefix(extractPath(link), "/feed/")
	resp, _ = e.do(t, "GET", oldPath, nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("old path still active: %d", resp.StatusCode)
	}
	for _, urls := range []string{"ftp://x", "https://user:pass@x", "https://x/path", "https://x?q=1", "https://x#f"} {
		resp, body = e.do(t, "PUT", "/api/settings", map[string]any{"subscribe_urls": urls}, cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("invalid origin %q: %d %s", urls, resp.StatusCode, body)
		}
	}
	resp, body = e.do(t, "PUT", "/api/settings", map[string]any{"subscribe_path": "bad/path"}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid path: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "PUT", "/api/settings", map[string]any{"clash_meta_template": "not: [yaml"}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid template: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "PUT", "/api/settings", map[string]any{"clash_meta_template": "proxies: []"}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("forbidden template: %d %s", resp.StatusCode, body)
	}
}

func TestSubscriptionNameHeaders(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	user := e.seedUser(t, "name-user")
	resp, body := e.do(t, "POST", "/api/users/"+formatID(user)+"/subscription", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}
	link := extractPath(subscriptionURL(t, body))
	resp, _ = e.do(t, "GET", link, nil, nil)
	if resp.Header.Get("Profile-Title") != "" || resp.Header.Get("Content-Disposition") != "" {
		t.Fatalf("name headers without configured name: %v", resp.Header)
	}
	resp, body = e.do(t, "PUT", "/api/settings", map[string]any{"subscribe_name": " 我的节点 "}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set name: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", link, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}
	wantTitle := "base64:" + base64.StdEncoding.EncodeToString([]byte("我的节点"))
	if got := resp.Header.Get("Profile-Title"); got != wantTitle {
		t.Fatalf("profile title: %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, "attachment") || !strings.Contains(got, "%E6%88%91") {
		t.Fatalf("content disposition: %q", got)
	}
	resp, body = e.do(t, "PUT", "/api/settings", map[string]any{"subscribe_name": "bad\nname"}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid name: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "PUT", "/api/settings", map[string]any{"subscribe_name": ""}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear name: %d %s", resp.StatusCode, body)
	}
	resp, _ = e.do(t, "GET", link, nil, nil)
	if resp.Header.Get("Profile-Title") != "" {
		t.Fatalf("name header after clear: %v", resp.Header)
	}
}

func TestSubscriptionNegotiationAndLogging(t *testing.T) {
	e, logs := newTestEnvWithLogger(t)
	cookie := e.login(t)
	user := e.seedUser(t, "log-user")
	resp, body := e.do(t, "POST", "/api/users/"+formatID(user)+"/subscription", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}
	url := subscriptionURL(t, body)
	tokenPath := extractPath(url)
	resp, body = e.doWithHeader(t, "GET", tokenPath, nil, "User-Agent", "Mihomo/1")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/yaml; charset=utf-8" {
		t.Fatalf("UA negotiation: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", tokenPath+"?flag=general", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}
	if _, err := base64.StdEncoding.DecodeString(body); err != nil {
		t.Fatalf("general output is not base64: %v", err)
	}
	if strings.Contains(logs.String(), strings.TrimPrefix(tokenPath, "/s/")) || !strings.Contains(logs.String(), "/s/:token") {
		t.Fatalf("token logging: %s", logs.String())
	}
}

func formatID(id int64) string { return strconv.FormatInt(id, 10) }

func extractPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.RequestURI()
}
