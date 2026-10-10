package agentruntime

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const (
	defaultPublicIPURL    = "https://api.ipify.org"
	publicIPCacheTTL      = 30 * time.Minute
	publicIPFetchTimeout  = 5 * time.Second
	publicIPResponseLimit = 64
)

type PublicIPProvider struct {
	staticIP string
	url      string
	client   *http.Client

	mu        sync.Mutex
	cached    string
	fetchedAt time.Time
}

func NewPublicIPProvider(staticIP, url string) *PublicIPProvider {
	if strings.TrimSpace(url) == "" {
		url = defaultPublicIPURL
	}
	return &PublicIPProvider{
		staticIP: strings.TrimSpace(staticIP),
		url:      url,
		client:   &http.Client{Timeout: publicIPFetchTimeout},
	}
}

func (p *PublicIPProvider) Get(ctx context.Context) string {
	if p.staticIP != "" {
		return p.staticIP
	}
	p.mu.Lock()
	if p.cached != "" && time.Since(p.fetchedAt) < publicIPCacheTTL {
		ip := p.cached
		p.mu.Unlock()
		return ip
	}
	p.mu.Unlock()

	ip := p.fetch(ctx)
	p.mu.Lock()
	if ip != "" {
		p.cached = ip
		p.fetchedAt = time.Now()
	}
	stale := p.cached
	p.mu.Unlock()
	if ip == "" {
		return stale
	}
	return ip
}

func (p *PublicIPProvider) fetch(ctx context.Context) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return ""
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, publicIPResponseLimit))
	if err != nil {
		return ""
	}
	return normalizePublicIP(string(body))
}

func normalizePublicIP(raw string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() || addr.IsMulticast() {
		return ""
	}
	if addr.Is4() {
		return addr.String()
	}
	if ip := net.ParseIP(addr.String()); ip == nil {
		return ""
	}
	return addr.String()
}
