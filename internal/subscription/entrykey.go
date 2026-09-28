package subscription

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// EntryKey returns the stable identifier of a Clash proxy entry: the lowercase
// hex HMAC-SHA256 of the proxy's canonical JSON with the display "name"
// removed. Excluding the name keeps an authorization valid when an upstream
// renames a line without touching its connection parameters; keying the digest
// with the panel appKey keeps the stored value from being a bare,
// offline-crackable hash of the proxy credentials. Two entries that differ
// only by name collapse into the same key.
func EntryKey(appKey []byte, proxy map[string]any) (string, error) {
	// json.Marshal sorts map keys (including nested ones), so the encoding is
	// deterministic regardless of map iteration order.
	canonical, err := json.Marshal(withoutName(proxy))
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, appKey)
	mac.Write(canonical)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// LinkEntryKey parses a share link and computes its entry key. ok is false when
// the link cannot be parsed, in which case there is no stable identity.
func LinkEntryKey(appKey []byte, link string) (key string, ok bool) {
	proxy, err := ParseShareURI(link)
	if err != nil {
		return "", false
	}
	key, err = EntryKey(appKey, proxy)
	if err != nil {
		return "", false
	}
	return key, true
}

// EntrySummary is a display digest of a custom node entry together with its
// stable entry key.
type EntrySummary struct {
	Key    string
	Name   string
	Type   string
	Server string
	Port   int
}

// SummarizeEntries behaves like Summarize and attaches the stable entry key to
// every parsed entry. Entries that cannot be parsed or lack the required fields
// are returned in skipped exactly as Summarize does (the raw link line, or the
// proxy name / "proxy #i").
func SummarizeEntries(appKey []byte, links []string, proxies []map[string]any) (entries []EntrySummary, skipped []string) {
	entries = []EntrySummary{}
	skipped = []string{}
	for _, line := range links {
		proxy, err := ParseShareURI(line)
		if err != nil {
			skipped = append(skipped, line)
			continue
		}
		if summary, err := summarizeEntry(appKey, proxy); err == nil {
			entries = append(entries, summary)
		} else {
			skipped = append(skipped, line)
		}
	}
	for i, proxy := range proxies {
		if summary, err := summarizeEntry(appKey, proxy); err == nil {
			entries = append(entries, summary)
		} else {
			skipped = append(skipped, proxyLabel(proxy, i))
		}
	}
	return entries, skipped
}

func summarizeEntry(appKey []byte, proxy map[string]any) (EntrySummary, error) {
	summary, err := summarizeProxy(proxy)
	if err != nil {
		return EntrySummary{}, err
	}
	key, err := EntryKey(appKey, proxy)
	if err != nil {
		return EntrySummary{}, err
	}
	return EntrySummary{Key: key, Name: summary.Name, Type: summary.Type, Server: summary.Server, Port: summary.Port}, nil
}

func withoutName(proxy map[string]any) map[string]any {
	out := make(map[string]any, len(proxy))
	for key, value := range proxy {
		if key == "name" {
			continue
		}
		out[key] = value
	}
	return out
}
