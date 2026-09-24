const KEY_PLACEHOLDER = '<agent_key>'

function keyOrPlaceholder(key: string): string {
  return key === '' ? KEY_PLACEHOLDER : key
}

export function binaryInstallCommand(origin: string, serverId: number, key: string): string {
  const resolved = keyOrPlaceholder(key)
  return [
    `PANEL_URL=${origin} SERVER_ID=${serverId} AGENT_KEY=${resolved} \\`,
    `  curl -fsSL ${origin}/install-agent.sh | sh`,
  ].join('\n')
}

export function dockerInstallCommand(origin: string, serverId: number, key: string): string {
  const resolved = keyOrPlaceholder(key)
  return [
    'docker run -d --name panel-agent --init --restart unless-stopped \\',
    '  --network host \\',
    `  -e AGENT_PANEL_URL=${origin} \\`,
    `  -e AGENT_SERVER_ID=${serverId} \\`,
    `  -e AGENT_KEY=${resolved} \\`,
    '  ghcr.io/senhao-xu/vps-node-agent:latest',
  ].join('\n')
}
