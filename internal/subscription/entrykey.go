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

// ResolvedEntry is a parsed custom-node entry with its stable key and the raw
// Clash proxy mapping. The proxy is what the chain renderer converts into a
// sing-box outbound; Link is the original share link for link entries and empty
// for upstream Clash proxies.
type ResolvedEntry struct {
	Key   string
	Name  string
	Proxy map[string]any
	Link  string
}

// ResolveEntries parses share links and/or upstream Clash proxies and attaches
// the stable entry key to every valid entry. Parse failures and entries missing
// required fields are skipped, matching SummarizeEntries' filtering. The error
// is non-nil only when an entry key cannot be computed.
func ResolveEntries(appKey []byte, links []string, proxies []map[string]any) ([]ResolvedEntry, error) {
	entries, _, err := resolveEntries(appKey, links, proxies)
	return entries, err
}

// resolveEntries is the shared parse/key/validate pass behind SummarizeEntries
// and ResolveEntries. It collects the skipped labels exactly as the previous
// inline implementation did, so subscription digests stay byte-identical.
func resolveEntries(appKey []byte, links []string, proxies []map[string]any) ([]ResolvedEntry, []string, error) {
	entries := []ResolvedEntry{}
	skipped := []string{}
	var firstErr error
	for _, line := range links {
		proxy, err := ParseShareURI(line)
		if err != nil {
			skipped = append(skipped, line)
			continue
		}
		resolved, ok, err := resolveEntry(appKey, proxy, line)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if !ok {
			skipped = append(skipped, line)
			continue
		}
		entries = append(entries, resolved)
	}
	for i, proxy := range proxies {
		resolved, ok, err := resolveEntry(appKey, proxy, "")
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if !ok {
			skipped = append(skipped, proxyLabel(proxy, i))
			continue
		}
		entries = append(entries, resolved)
	}
	return entries, skipped, firstErr
}

// resolveEntry validates a parsed proxy and computes its stable key. ok is
// false for entries missing required display fields (a skip, not an error);
// err is non-nil only when the key itself cannot be computed.
func resolveEntry(appKey []byte, proxy map[string]any, link string) (ResolvedEntry, bool, error) {
	summary, err := summarizeProxy(proxy)
	if err != nil {
		return ResolvedEntry{}, false, nil
	}
	key, err := EntryKey(appKey, proxy)
	if err != nil {
		return ResolvedEntry{}, false, err
	}
	return ResolvedEntry{Key: key, Name: summary.Name, Proxy: proxy, Link: link}, true, nil
}

// SummarizeEntries behaves like Summarize and attaches the stable entry key to
// every parsed entry. Entries that cannot be parsed or lack the required fields
// are returned in skipped exactly as Summarize does (the raw link line, or the
// proxy name / "proxy #i").
func SummarizeEntries(appKey []byte, links []string, proxies []map[string]any) (entries []EntrySummary, skipped []string) {
	resolved, skipped, _ := resolveEntries(appKey, links, proxies)
	entries = make([]EntrySummary, 0, len(resolved))
	for _, r := range resolved {
		summary, err := summarizeProxy(r.Proxy)
		if err != nil {
			continue
		}
		entries = append(entries, EntrySummary{Key: r.Key, Name: summary.Name, Type: summary.Type, Server: summary.Server, Port: summary.Port})
	}
	return entries, skipped
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
