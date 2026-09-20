package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"vps-node/internal/agentclient"
)

const (
	clashRequestTimeout = 10 * time.Second
	maxClashBodyBytes   = 32 << 20
)

const (
	metaInboundTag  = "inbound"
	metaInboundPort = "inboundPort"
	metaInboundUser = "inboundUser"
	metaSourceIP    = "sourceIP"
)

type clashConn struct {
	ID       string         `json:"id"`
	Upload   int64          `json:"upload"`
	Download int64          `json:"download"`
	Start    string         `json:"start"`
	Metadata map[string]any `json:"metadata"`
}

type clashConnections struct {
	Connections []clashConn `json:"connections"`
}

type ClashSource interface {
	Connections(ctx context.Context) (*clashConnections, error)
}

type ClashClient struct {
	baseURL string
	secret  string
	hc      *http.Client
}

func NewClashClient(baseURL, secret string) (*ClashClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return nil, fmt.Errorf("invalid clash api base URL %q", baseURL)
	}
	return &ClashClient{
		baseURL: baseURL,
		secret:  secret,
		hc:      &http.Client{Timeout: clashRequestTimeout},
	}, nil
}

func (c *ClashClient) Connections(ctx context.Context) (*clashConnections, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/connections", nil)
	if err != nil {
		return nil, err
	}
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("clash api %s returned %d", c.baseURL, resp.StatusCode)
	}
	var out clashConnections
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxClashBodyBytes)).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode clash connections: %w", err)
	}
	return &out, nil
}

func EndpointFromConfig(raw json.RawMessage) (baseURL, secret string, err error) {
	var cfg struct {
		Experimental struct {
			ClashAPI struct {
				ExternalController string `json:"external_controller"`
				Secret             string `json:"secret"`
			} `json:"clash_api"`
		} `json:"experimental"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", "", fmt.Errorf("parse rendered config: %w", err)
	}
	controller := strings.TrimSpace(cfg.Experimental.ClashAPI.ExternalController)
	if controller == "" {
		return "", "", fmt.Errorf("rendered config has no experimental.clash_api.external_controller")
	}
	baseURL = controller
	if !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}
	return baseURL, cfg.Experimental.ClashAPI.Secret, nil
}

type NodeEntry struct {
	ID       int64
	Protocol string
	Port     int
	Tag      string
}

type pairKey struct {
	user int64
	node int64
}

type Table struct {
	nodesByTag  map[string]NodeEntry
	nodesByPort map[int]NodeEntry
	usersByName map[string]int64
	pairs       map[pairKey]bool
}

func BuildTable(users []agentclient.User) *Table {
	tb := &Table{
		nodesByTag:  map[string]NodeEntry{},
		nodesByPort: map[int]NodeEntry{},
		usersByName: map[string]int64{},
		pairs:       map[pairKey]bool{},
	}
	nodesByID := map[int64]NodeEntry{}
	for i := range users {
		u := &users[i]
		tb.usersByName[userName(u.ID)] = u.ID
		for _, n := range u.Nodes {
			entry := NodeEntry{ID: n.ID, Protocol: n.Protocol, Port: n.Port, Tag: inboundTag(n.Protocol, n.ID)}
			nodesByID[n.ID] = entry
			tb.pairs[pairKey{user: u.ID, node: n.ID}] = true
		}
	}
	for _, e := range nodesByID {
		tb.nodesByTag[e.Tag] = e
		tb.nodesByPort[e.Port] = e
	}
	return tb
}

func inboundTag(protocol string, id int64) string {
	return protocol + "-" + strconv.FormatInt(id, 10)
}

func userName(id int64) string {
	return "u-" + strconv.FormatInt(id, 10)
}

type attributedConn struct {
	id        string
	userID    int64
	node      NodeEntry
	sourceIP  string
	upload    int64
	download  int64
	connected time.Time
}

func (tb *Table) attribute(conn clashConn) (*attributedConn, bool) {
	if conn.ID == "" {
		return nil, false
	}
	start, err := parseClashTime(conn.Start)
	if err != nil {
		return nil, false
	}
	node, ok := tb.resolveNode(conn.Metadata)
	if !ok {
		return nil, false
	}
	ip := metaString(conn.Metadata, metaSourceIP)
	userID := tb.resolveUser(conn.Metadata)
	if userID == 0 || !tb.pairs[pairKey{user: userID, node: node.ID}] {
		return nil, false
	}
	return &attributedConn{
		id:        conn.ID,
		userID:    userID,
		node:      node,
		sourceIP:  ip,
		upload:    conn.Upload,
		download:  conn.Download,
		connected: start,
	}, true
}

func (tb *Table) resolveNode(meta map[string]any) (NodeEntry, bool) {
	if tag := metaString(meta, metaInboundTag); tag != "" {
		if e, ok := tb.nodesByTag[tag]; ok {
			return e, true
		}
	}
	if p := metaString(meta, metaInboundPort); p != "" {
		if port, err := strconv.Atoi(p); err == nil {
			if e, ok := tb.nodesByPort[port]; ok {
				return e, true
			}
		}
	}
	return NodeEntry{}, false
}

func (tb *Table) resolveUser(meta map[string]any) int64 {
	name := metaString(meta, metaInboundUser)
	if name == "" {
		return 0
	}
	if id, ok := tb.usersByName[name]; ok {
		return id
	}
	return 0
}

func metaString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	s, _ := meta[key].(string)
	return s
}

func parseClashTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, fmt.Errorf("empty connection start time")
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable connection start time %q", raw)
}
