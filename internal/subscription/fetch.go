package subscription

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// FetchTimeout bounds a single upstream subscription fetch.
	FetchTimeout = 5 * time.Second
	// MaxFetchBytes caps the upstream response body (1 MiB).
	MaxFetchBytes = 1 << 20
	// MaxFetchRedirects bounds redirect hops to the upstream.
	MaxFetchRedirects = 3
	// CacheTTL is how long a fetched upstream subscription is reused.
	CacheTTL = 5 * time.Minute
)

var errTooManyRedirects = fmt.Errorf("too many redirects")

var fetchClient = &http.Client{
	Timeout: FetchTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= MaxFetchRedirects {
			return errTooManyRedirects
		}
		return nil
	},
}

// FetchSubscription downloads an upstream subscription document. SSRF
// surface is limited to http/https with a hard timeout and a response size
// cap; administrators may legitimately point at in-network URLs, so no IP
// range filtering is applied.
func FetchSubscription(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("invalid subscription URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("subscription URL must be http or https")
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("subscription URL has no host")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build subscription request: %w", err)
	}
	req.Header.Set("User-Agent", "vps-node-panel")
	resp, err := fetchClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch subscription: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch subscription: upstream returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxFetchBytes+1))
	if err != nil {
		return "", fmt.Errorf("read subscription: %w", err)
	}
	if len(body) > MaxFetchBytes {
		return "", fmt.Errorf("fetch subscription: response exceeds %d bytes", MaxFetchBytes)
	}
	return string(body), nil
}

// NormalizeFetchedContent classifies an upstream subscription payload into
// share links and/or Clash proxies. Three payload shapes are supported:
// a Clash YAML document with a proxies: sequence, a base64-encoded list of
// share links, or a plain newline-separated list of share links.
func NormalizeFetchedContent(content string) (links []string, proxies []map[string]any) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, nil
	}
	if proxies := extractClashProxies(trimmed); proxies != nil {
		return nil, proxies
	}
	if decoded, err := decodeBase64(strings.Join(strings.Fields(trimmed), "")); err == nil {
		if lines := shareLinkLines(string(decoded)); len(lines) > 0 {
			return lines, nil
		}
	}
	return shareLinkLines(trimmed), nil
}

// extractClashProxies returns the proxies: sequence of a Clash YAML document,
// or nil when the content is not such a document.
func extractClashProxies(content string) []map[string]any {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil
	}
	raw, ok := doc["proxies"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	proxies := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		proxy, ok := item.(map[string]any)
		if !ok {
			return nil
		}
		name, _ := proxy["name"].(string)
		server, _ := proxy["server"].(string)
		proxyType, _ := proxy["type"].(string)
		if name == "" || server == "" || proxyType == "" {
			return nil
		}
		proxies = append(proxies, proxy)
	}
	return proxies
}

// shareLinkLines returns the non-empty lines of content that look like share
// links (they carry a scheme separator).
func shareLinkLines(content string) []string {
	lines := []string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "://") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// SplitLinkLines splits administrator-entered link content verbatim: every
// non-empty line is kept, even ones that cannot be parsed (general format
// passes them through unchanged).
func SplitLinkLines(content string) []string {
	lines := []string{}
	for _, line := range strings.Split(content, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
