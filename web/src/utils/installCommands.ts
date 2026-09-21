const TOKEN_PLACEHOLDER = '<register_token>'

function tokenOrPlaceholder(token: string): string {
  return token === '' ? TOKEN_PLACEHOLDER : token
}

export function binaryInstallCommand(origin: string, serverId: number, token: string): string {
  const resolved = tokenOrPlaceholder(token)
  return [
    `PANEL_URL=${origin} SERVER_ID=${serverId} REGISTER_TOKEN=${resolved} \\`,
    `  curl -fsSL ${origin}/install-agent.sh | sh`,
  ].join('\n')
}

export function dockerInstallCommand(origin: string, serverId: number, token: string): string {
  const resolved = tokenOrPlaceholder(token)
  return [
    'docker run -d --name panel-agent --init --restart unless-stopped \\',
    '  --network host \\',
    '  -v panel-agent-state:/var/lib/panel-agent \\',
    `  -e AGENT_PANEL_URL=${origin} \\`,
    `  -e AGENT_SERVER_ID=${serverId} \\`,
    `  -e AGENT_REGISTER_TOKEN=${resolved} \\`,
    '  ghcr.io/senhao-xu/vps-node-agent:latest',
  ].join('\n')
}
