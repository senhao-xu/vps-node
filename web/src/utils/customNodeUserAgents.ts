/**
 * Single source for the upstream subscription User-Agent presets offered by
 * the custom node form. `DEFAULT_CUSTOM_NODE_USER_AGENT` mirrors the backend
 * `subscription.DefaultUserAgent`; an empty stored value falls back to it.
 */
export const DEFAULT_CUSTOM_NODE_USER_AGENT = 'clash-verge/v2.0.0'

/** Sentinel for the "custom" AppSelect option (never a real stored UA). */
export const CUSTOM_USER_AGENT_CHOICE = '__custom__'

export type UserAgentPreset = {
  value: string
  label: string
}

export const USER_AGENT_PRESETS: UserAgentPreset[] = [
  { value: DEFAULT_CUSTOM_NODE_USER_AGENT, label: 'Clash Verge（默认）' },
  { value: 'Mihomo/1.18.0', label: 'Mihomo 1.18.0' },
  { value: 'sing-box/1.10.0', label: 'sing-box 1.10.0' },
  { value: 'v2rayN/6.60', label: 'v2rayN 6.60' },
  { value: 'Shadowrocket/2.2.30', label: 'Shadowrocket 2.2.30' },
]

export function isUserAgentPreset(value: string): boolean {
  return USER_AGENT_PRESETS.some((preset) => preset.value === value)
}
