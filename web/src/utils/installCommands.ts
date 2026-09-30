const KEY_PLACEHOLDER = '<agent_key>'

function keyOrPlaceholder(key: string): string {
  return key === '' ? KEY_PLACEHOLDER : key
}

function shellQuote(value: string): string {
  return "'" + value.replaceAll("'", "'\\''") + "'"
}

export function binaryInstallCommand(origin: string, serverId: number, key: string): string {
  const resolved = keyOrPlaceholder(key)
  return [
    '(',
    '  set -eu',
    '  installer=$(mktemp)',
    `  trap 'rm -f "$installer"' EXIT`,
    `  curl -fsSL ${shellQuote(`${origin}/install-agent.sh`)} -o "$installer"`,
    `  PANEL_URL=${shellQuote(origin)} SERVER_ID=${shellQuote(String(serverId))} AGENT_KEY=${shellQuote(resolved)} sh "$installer"`,
    ')',
  ].join('\n')
}

export function dockerInstallCommand(origin: string, serverId: number, key: string): string {
  const resolved = keyOrPlaceholder(key)
  return [
    'docker run -d --name panel-agent --init --restart unless-stopped \\',
    '  --network host \\',
    `  -e AGENT_PANEL_URL=${shellQuote(origin)} \\`,
    `  -e AGENT_SERVER_ID=${shellQuote(String(serverId))} \\`,
    `  -e AGENT_KEY=${shellQuote(resolved)} \\`,
    '  ghcr.io/senhao-xu/vps-node-agent:latest',
  ].join('\n')
}
