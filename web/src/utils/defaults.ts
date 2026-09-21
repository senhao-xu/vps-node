export const DefaultClashMetaTemplate = `mixed-port: 7890
allow-lan: false
mode: rule
log-level: info
proxy-groups:
  - name: 节点选择
    type: select
    proxies:
      - __ALL_PROXIES__
rules:
  - MATCH,节点选择
`
