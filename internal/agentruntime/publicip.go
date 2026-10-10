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
	defaultPublicIPv6URL  = "https://api6.ipify.org"
	publicIPCacheTTL      = 30 * time.Minute
	publicIPFetchTimeout  = 5 * time.Second
	publicIPResponseLimit = 64
)

type ipFamilyCache struct {
	value     string
	fetchedAt time.Time
}

type PublicIPProvider struct {
	staticIP   string
	staticIPv6 string
	url        string
	url6       string
	client     *http.Client

	mu sync.Mutex
	v4 ipFamilyCache
	v6 ipFamilyCache
}

func NewPublicIPProvider(staticIP, staticIPv6, url, url6 string) *PublicIPProvider {
	if strings.TrimSpace(url) == "" {
		url = defaultPublicIPURL
	}
	if strings.TrimSpace(url6) == "" {
		url6 = defaultPublicIPv6URL
	}
	return &PublicIPProvider{
		staticIP:   strings.TrimSpace(staticIP),
		staticIPv6: strings.TrimSpace(staticIPv6),
		url:        url,
		url6:       url6,
		client:     &http.Client{Timeout: publicIPFetchTimeout},
	}
}

func (p *PublicIPProvider) Get(ctx context.Context) string {
	return p.get(ctx, p.staticIP, p.url, &p.v4, false)
}

func (p *PublicIPProvider) Get6(ctx context.Context) string {
	return p.get(ctx, p.staticIPv6, p.url6, &p.v6, true)
}

func (p *PublicIPProvider) get(ctx context.Context, static, url string, cache *ipFamilyCache, wantV6 bool) string {
	if static != "" {
		return static
	}
	p.mu.Lock()
	if cache.value != "" && time.Since(cache.fetchedAt) < publicIPCacheTTL {
		ip := cache.value
		p.mu.Unlock()
		return ip
	}
	p.mu.Unlock()

	ip := p.fetch(ctx, url, wantV6)
	p.mu.Lock()
	if ip != "" {
		cache.value = ip
		cache.fetchedAt = time.Now()
	}
	stale := cache.value
	p.mu.Unlock()
	if ip == "" {
		return stale
	}
	return ip
}

func (p *PublicIPProvider) fetch(ctx context.Context, url string, wantV6 bool) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	return normalizePublicIP(string(body), wantV6)
}

func normalizePublicIP(raw string, wantV6 bool) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if wantV6 != addr.Is6() {
		return ""
	}
	if addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() || addr.IsMulticast() {
		return ""
	}
	if ip := net.ParseIP(addr.String()); ip == nil {
		return ""
	}
	return addr.String()
}
