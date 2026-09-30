package web_test

import (
	"net/http"
	"strings"
	"testing"

	"vps-node/deploy"
)

func TestAgentInstallerIsPublicShellScript(t *testing.T) {
	env := newTestEnv(t)
	resp, body := env.do(t, http.MethodGet, "/install-agent.sh", nil, nil)
	if resp.StatusCode != http.StatusOK || body != deploy.AgentInstallScript || !strings.HasPrefix(body, "#!/bin/sh\n") {
		t.Fatalf("installer status=%d, script not served", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/x-shellscript; charset=utf-8" {
		t.Fatalf("content type=%q", got)
	}
	resp, body = env.do(t, http.MethodHead, "/install-agent.sh", nil, nil)
	if resp.StatusCode != http.StatusOK || body != "" {
		t.Fatalf("HEAD status=%d body=%q", resp.StatusCode, body)
	}
}
