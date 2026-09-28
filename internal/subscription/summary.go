package subscription

import "fmt"

// NodeSummary is a display-only digest of a custom node entry.
type NodeSummary struct {
	Name   string
	Type   string
	Server string
	Port   int
}

// Summarize converts share links and/or Clash proxy mappings into display
// summaries. Entries that cannot be parsed or lack the required fields are
// returned in skipped (the raw link line, or the proxy name / "proxy #i").
func Summarize(links []string, proxies []map[string]any) (summaries []NodeSummary, skipped []string) {
	summaries = []NodeSummary{}
	skipped = []string{}
	for _, line := range links {
		proxy, err := ParseShareURI(line)
		if err != nil {
			skipped = append(skipped, line)
			continue
		}
		summary, err := summarizeProxy(proxy)
		if err != nil {
			skipped = append(skipped, line)
			continue
		}
		summaries = append(summaries, summary)
	}
	for i, proxy := range proxies {
		summary, err := summarizeProxy(proxy)
		if err != nil {
			skipped = append(skipped, proxyLabel(proxy, i))
			continue
		}
		summaries = append(summaries, summary)
	}
	return summaries, skipped
}

func summarizeProxy(proxy map[string]any) (NodeSummary, error) {
	name, _ := proxy["name"].(string)
	proxyType, _ := proxy["type"].(string)
	server, _ := proxy["server"].(string)
	if name == "" || server == "" || proxyType == "" {
		return NodeSummary{}, fmt.Errorf("missing name, server or type")
	}
	port, err := looseInt(proxy["port"])
	if err != nil || port < 1 || port > 65535 {
		return NodeSummary{}, fmt.Errorf("invalid port %v", proxy["port"])
	}
	return NodeSummary{Name: name, Type: proxyType, Server: server, Port: port}, nil
}

func proxyLabel(proxy map[string]any, index int) string {
	if name, _ := proxy["name"].(string); name != "" {
		return name
	}
	return fmt.Sprintf("proxy #%d", index)
}
