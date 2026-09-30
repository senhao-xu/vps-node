package deploy

import _ "embed"

//go:embed install-agent.sh
var AgentInstallScript string
