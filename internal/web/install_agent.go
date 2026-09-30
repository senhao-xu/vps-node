package web

import (
	"net/http"
	"strings"
	"time"

	"vps-node/deploy"
)

func handleAgentInstaller(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "install-agent.sh", time.Time{}, strings.NewReader(deploy.AgentInstallScript))
}
