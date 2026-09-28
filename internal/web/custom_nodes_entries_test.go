package web_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCustomNodeEntriesViewLinks(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "links-ext", "source_type": "links",
		"content": "ss://aes-128-gcm:secret@1.2.3.4:8388#HK-1\nbad line",
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	id := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "GET", "/api/custom-nodes/"+formatID(id)+"/nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("view links: %d %s", resp.StatusCode, body)
	}
	got := jsonMap(t, body)
	if got["source_type"] != "links" || got["has_cache"] != false {
		t.Fatalf("links metadata: %s", body)
	}
	if got["fetched_at"] != nil {
		t.Fatalf("links must not report fetched_at: %s", body)
	}
	entries := got["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("entries: %s", body)
	}
	entry := entries[0].(map[string]any)
	if entry["name"] != "HK-1" || entry["type"] != "ss" || entry["server"] != "1.2.3.4" || entry["port"].(float64) != 8388 {
		t.Fatalf("entry: %v", entry)
	}
	skipped := got["skipped"].([]any)
	if len(skipped) != 1 || skipped[0] != "bad line" {
		t.Fatalf("skipped: %s", body)
	}

	resp, body = e.do(t, "GET", "/api/custom-nodes/9999/nodes", nil, cookie)
	if resp.StatusCode != http.StatusNotFound || errorCode(t, body) != "not_found" {
		t.Fatalf("unknown id: %d %s", resp.StatusCode, body)
	}
	resp, _ = e.do(t, "GET", "/api/custom-nodes/"+formatID(id)+"/nodes", nil, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated view: %d", resp.StatusCode)
	}
}

func TestCustomNodeEntriesSubscriptionNoCacheDoesNotFetch(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte("ss://aes-128-gcm:secret@up.example.com:443#up-node\n"))))
	}))
	defer upstream.Close()

	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "sub-no-cache", "source_type": "subscription", "content": upstream.URL,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	id := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "GET", "/api/custom-nodes/"+formatID(id)+"/nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("view no-cache: %d %s", resp.StatusCode, body)
	}
	got := jsonMap(t, body)
	if got["has_cache"] != false || got["fetched_at"] != nil {
		t.Fatalf("no-cache metadata: %s", body)
	}
	if len(got["entries"].([]any)) != 0 {
		t.Fatalf("no-cache entries: %s", body)
	}
	if hits.Load() != 0 {
		t.Fatalf("view must not fetch upstream, hits=%d", hits.Load())
	}
}

func TestCustomNodeRefreshSubscription(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		payload := "ss://aes-128-gcm:secret@up.example.com:443#up-node\nfoo://unsupported"
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	}))
	defer upstream.Close()

	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "sub-refresh", "source_type": "subscription", "content": upstream.URL,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	id := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "POST", "/api/custom-nodes/"+formatID(id)+"/refresh", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh: %d %s", resp.StatusCode, body)
	}
	got := jsonMap(t, body)
	if got["has_cache"] != true || got["fetched_at"] == nil {
		t.Fatalf("refresh metadata: %s", body)
	}
	entries := got["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("refresh entries: %s", body)
	}
	entry := entries[0].(map[string]any)
	if entry["name"] != "up-node" || entry["type"] != "ss" || entry["server"] != "up.example.com" || entry["port"].(float64) != 443 {
		t.Fatalf("refresh entry: %v", entry)
	}
	if skipped := got["skipped"].([]any); len(skipped) != 1 || skipped[0] != "foo://unsupported" {
		t.Fatalf("refresh skipped: %s", body)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected one upstream fetch, got %d", hits.Load())
	}

	// The view now reads the freshly written cache without refetching.
	resp, body = e.do(t, "GET", "/api/custom-nodes/"+formatID(id)+"/nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["has_cache"] != true {
		t.Fatalf("view after refresh: %d %s", resp.StatusCode, body)
	}
	if hits.Load() != 1 {
		t.Fatalf("cached view must not refetch, hits=%d", hits.Load())
	}

	// The list response reflects the new cache timestamp.
	resp, body = e.do(t, "GET", "/api/custom-nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %s", resp.StatusCode, body)
	}
	item := jsonMap(t, body)["items"].([]any)[0].(map[string]any)
	if item["has_cache"] != true || item["fetched_at"] == nil {
		t.Fatalf("list cache state: %s", body)
	}

	resp, body = e.do(t, "POST", "/api/custom-nodes/9999/refresh", nil, cookie)
	if resp.StatusCode != http.StatusNotFound || errorCode(t, body) != "not_found" {
		t.Fatalf("refresh unknown id: %d %s", resp.StatusCode, body)
	}
	resp, _ = e.do(t, "POST", "/api/custom-nodes/"+formatID(id)+"/refresh", nil, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated refresh: %d", resp.StatusCode)
	}
}

func TestCustomNodeRefreshLinksValidation(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "links-refresh", "source_type": "links", "content": "ss://aes-128-gcm:secret@1.2.3.4:8388#HK-1",
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	id := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "POST", "/api/custom-nodes/"+formatID(id)+"/refresh", nil, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("refresh links: %d %s", resp.StatusCode, body)
	}
}

func TestCustomNodeRefreshUpstreamFailureKeepsCache(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte("ss://aes-128-gcm:secret@up.example.com:443#up-node\n"))))
	}))

	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "sub-stale", "source_type": "subscription", "content": upstream.URL,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	id := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "POST", "/api/custom-nodes/"+formatID(id)+"/refresh", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first refresh: %d %s", resp.StatusCode, body)
	}
	before := jsonMap(t, body)
	if len(before["entries"].([]any)) != 1 || before["fetched_at"] == nil {
		t.Fatalf("first refresh entries: %s", body)
	}
	fetchedAt := before["fetched_at"].(string)

	upstream.Close()

	resp, body = e.do(t, "POST", "/api/custom-nodes/"+formatID(id)+"/refresh", nil, cookie)
	if resp.StatusCode != http.StatusInternalServerError || errorCode(t, body) != "internal" {
		t.Fatalf("failed refresh: %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "failed to fetch upstream subscription") {
		t.Fatalf("failed refresh must carry a readable message: %s", body)
	}

	resp, body = e.do(t, "GET", "/api/custom-nodes/"+formatID(id)+"/nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("view after failure: %d %s", resp.StatusCode, body)
	}
	after := jsonMap(t, body)
	if after["has_cache"] != true {
		t.Fatalf("cache must survive a failed refresh: %s", body)
	}
	if after["fetched_at"] != fetchedAt {
		t.Fatalf("fetched_at changed on failure: before=%v after=%v", fetchedAt, after["fetched_at"])
	}
	entries := after["entries"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["name"] != "up-node" {
		t.Fatalf("stale entries lost: %s", body)
	}
}
