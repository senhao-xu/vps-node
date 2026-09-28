package web_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"vps-node/internal/repo"
)

func TestNodeShareRendersUserLink(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	serverID := e.seedServer(t, "share")

	ssID, err := e.repo.CreateNode(ctx, repo.NewNode{
		ServerID: serverID, Address: "share.example.com", Name: "share-ss",
		Protocol: repo.ProtocolShadowsocks, Port: 8388,
		ProtocolSettings: `{"cipher":"2022-blake3-aes-128-gcm"}`,
	})
	if err != nil {
		t.Fatalf("seed ss node: %v", err)
	}
	vlessID, err := e.repo.CreateNode(ctx, repo.NewNode{
		ServerID: serverID, Address: "vless.example.com", Name: "share-vless",
		Protocol: repo.ProtocolVLESS, Port: 443,
		ProtocolSettings: `{"reality_settings":{"server_name":"www.example.com","public_key":"pubkey","short_id":"deadbeef"}}`,
	})
	if err != nil {
		t.Fatalf("seed vless node: %v", err)
	}
	userID := e.seedUser(t, "share-user")
	u, err := e.repo.GetUser(ctx, userID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}

	// An unauthorized user still gets a preview link, flagged not authorized.
	resp, body := e.do(t, "GET", fmt.Sprintf("/api/nodes/%d/share?user_id=%d", ssID, userID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("share ss: %d %s", resp.StatusCode, body)
	}
	share := jsonMap(t, body)
	if share["authorized"] != false {
		t.Fatalf("expected authorized=false before authorization, got %s", body)
	}
	links, _ := share["links"].([]any)
	if len(links) != 1 {
		t.Fatalf("expected 1 ss link, got %s", body)
	}
	ssLink, _ := links[0].(string)
	if !strings.HasPrefix(ssLink, "ss://") || !strings.Contains(ssLink, "share.example.com:8388") {
		t.Fatalf("unexpected ss link: %q", ssLink)
	}
	credential, err := decodeSSUserinfo(ssLink)
	if err != nil {
		t.Fatalf("decode ss credential: %v", err)
	}
	if !strings.HasPrefix(credential, "2022-blake3-aes-128-gcm:") {
		t.Fatalf("ss link must carry cipher:password, got %q", credential)
	}

	// The vless link embeds the chosen user's UUID and reality parameters.
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes/%d/share?user_id=%d", vlessID, userID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("share vless: %d %s", resp.StatusCode, body)
	}
	share = jsonMap(t, body)
	links, _ = share["links"].([]any)
	if len(links) != 1 {
		t.Fatalf("expected 1 vless link, got %s", body)
	}
	vlessLink, _ := links[0].(string)
	for _, want := range []string{"vless://" + u.UUID + "@vless.example.com:443", "security=reality", "pbk=pubkey", "sid=deadbeef"} {
		if !strings.Contains(vlessLink, want) {
			t.Fatalf("vless link missing %q: %q", want, vlessLink)
		}
	}

	// After authorization the flag flips.
	if err := e.repo.AuthorizeUserNode(ctx, userID, ssID); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes/%d/share?user_id=%d", ssID, userID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("share ss after auth: %d %s", resp.StatusCode, body)
	}
	if jsonMap(t, body)["authorized"] != true {
		t.Fatalf("expected authorized=true after authorization, got %s", body)
	}

	// Missing or unknown user_id is a validation error.
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes/%d/share", ssID), nil, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("missing user_id: expected 422 validation, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes/%d/share?user_id=99999", ssID), nil, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("unknown user_id: expected 422 validation, got %d %s", resp.StatusCode, body)
	}

	// Unknown node is a not-found error.
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes/99999/share?user_id=%d", userID), nil, cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown node: expected 404, got %d %s", resp.StatusCode, body)
	}
}

func TestNodeShareExpandsIPv6(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	serverID := e.seedServer(t, "share-v6")
	nodeID, err := e.repo.CreateNode(ctx, repo.NewNode{
		ServerID: serverID, Address: "v6.example.com", IPv6Enabled: true, IPv6Address: "2001:db8::1", Name: "v6-ss",
		Protocol: repo.ProtocolShadowsocks, Port: 8388,
		ProtocolSettings: `{"cipher":"2022-blake3-aes-128-gcm"}`,
	})
	if err != nil {
		t.Fatalf("seed node: %v", err)
	}
	userID := e.seedUser(t, "v6-user")

	resp, body := e.do(t, "GET", fmt.Sprintf("/api/nodes/%d/share?user_id=%d", nodeID, userID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("share: %d %s", resp.StatusCode, body)
	}
	links, _ := jsonMap(t, body)["links"].([]any)
	if len(links) != 2 {
		t.Fatalf("expected the IPv6 variant too, got %s", body)
	}
	if !strings.Contains(links[1].(string), "2001:db8::1") {
		t.Fatalf("second link must target the IPv6 address, got %s", body)
	}
}

// decodeSSUserinfo returns the decoded userinfo of an ss:// link
// (base64url(cipher:password)).
func decodeSSUserinfo(link string) (string, error) {
	rest := strings.TrimPrefix(link, "ss://")
	at := strings.Index(rest, "@")
	if at < 0 {
		return "", fmt.Errorf("ss link has no userinfo separator: %q", link)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(rest[:at])
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
