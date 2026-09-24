package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxAttempts = 4
	defaultBaseDelay   = 500 * time.Millisecond
	defaultMaxDelay    = 8 * time.Second
	requestTimeout     = 15 * time.Second
)

type Client struct {
	base        string
	key         string
	hc          *http.Client
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	randomize   func(n int) int
}

func New(baseURL string) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("panel base URL is required")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid panel base URL %q", baseURL)
	}
	return &Client{
		base:        base,
		hc:          &http.Client{Timeout: requestTimeout},
		maxAttempts: defaultMaxAttempts,
		baseDelay:   defaultBaseDelay,
		maxDelay:    defaultMaxDelay,
		randomize:   rand.IntN,
	}, nil
}

func (c *Client) SetKey(key string) {
	c.key = key
}

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("panel %d %s: %s", e.Status, e.Code, e.Message)
}

func (e *APIError) Retryable() bool {
	return e.Status == http.StatusRequestTimeout ||
		e.Status == http.StatusTooManyRequests ||
		e.Status >= 500
}

type HeartbeatRequest struct {
	Version        string  `json:"version"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryPercent  float64 `json:"memory_percent"`
	DiskPercent    float64 `json:"disk_percent"`
	UptimeSeconds  int64   `json:"uptime_seconds"`
	LastApplyError string  `json:"last_apply_error,omitempty"`
}

type HeartbeatResponse struct {
	OK                       bool  `json:"ok"`
	ServerID                 int64 `json:"server_id"`
	ServerRevision           int64 `json:"server_revision"`
	HeartbeatIntervalSeconds int   `json:"heartbeat_interval_seconds"`
	TrafficSeq               int64 `json:"traffic_seq"`
	DeviceSeq                int64 `json:"device_seq"`
	VisitSeq                 int64 `json:"visit_seq"`
}

type UserNode struct {
	ID         int64          `json:"id"`
	Protocol   string         `json:"protocol"`
	Port       int            `json:"port"`
	Credential map[string]any `json:"credential"`
}

type User struct {
	ID             int64      `json:"id"`
	UUID           string     `json:"uuid"`
	Status         string     `json:"status"`
	TransferEnable int64      `json:"transfer_enable"`
	U              int64      `json:"u"`
	D              int64      `json:"d"`
	SpeedLimit     int64      `json:"speed_limit"`
	DeviceLimit    int64      `json:"device_limit"`
	ExpiresAt      *string    `json:"expires_at"`
	Nodes          []UserNode `json:"nodes"`
}

type ConfigResponse struct {
	Status          string `json:"status"`
	Revision        int64  `json:"revision"`
	RendererVersion string `json:"renderer_version"`
	Config          *struct {
		Singbox json.RawMessage `json:"singbox"`
	} `json:"config"`
	Users []User `json:"users"`
}

type TrafficRecord struct {
	UserID     int64  `json:"user_id"`
	NodeID     int64  `json:"node_id"`
	U          int64  `json:"u"`
	D          int64  `json:"d"`
	RecordedAt string `json:"recorded_at"`
}

type TrafficBatch struct {
	BatchSeq int64           `json:"batch_seq"`
	Records  []TrafficRecord `json:"records"`
}

type TrafficAck struct {
	Accepted bool  `json:"accepted"`
	BatchSeq int64 `json:"batch_seq"`
	Records  int64 `json:"records"`
}

type DeviceReport struct {
	UserID int64    `json:"user_id"`
	NodeID int64    `json:"node_id"`
	IPs    []string `json:"ips"`
	Online int      `json:"online"`
}

type DeviceBatch struct {
	BatchSeq   int64          `json:"batch_seq"`
	RecordedAt string         `json:"recorded_at"`
	Devices    []DeviceReport `json:"devices"`
}

type DeviceAck struct {
	Accepted bool  `json:"accepted"`
	BatchSeq int64 `json:"batch_seq"`
	Devices  int64 `json:"devices"`
}

type VisitRecord struct {
	UserID     int64  `json:"user_id"`
	NodeID     int64  `json:"node_id"`
	DestHost   string `json:"dest_host"`
	DestPort   int    `json:"dest_port"`
	Network    string `json:"network"`
	ClientIP   string `json:"client_ip,omitempty"`
	RecordedAt string `json:"recorded_at"`
}

type VisitBatch struct {
	BatchSeq int64         `json:"batch_seq"`
	Records  []VisitRecord `json:"records"`
}

type VisitAck struct {
	Accepted bool  `json:"accepted"`
	BatchSeq int64 `json:"batch_seq"`
	Records  int64 `json:"records"`
}

func (c *Client) Heartbeat(ctx context.Context, req HeartbeatRequest) (*HeartbeatResponse, error) {
	var out HeartbeatResponse
	if err := c.do(ctx, http.MethodPost, "/api/agent/heartbeat", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Config(ctx context.Context, appliedRevision int64) (*ConfigResponse, error) {
	q := url.Values{}
	if appliedRevision > 0 {
		q.Set("version", strconv.FormatInt(appliedRevision, 10))
	}
	var out ConfigResponse
	if err := c.do(ctx, http.MethodGet, "/api/agent/config", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Traffic(ctx context.Context, batch TrafficBatch) (*TrafficAck, error) {
	var out TrafficAck
	if err := c.do(ctx, http.MethodPost, "/api/agent/traffic", nil, batch, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Devices(ctx context.Context, batch DeviceBatch) (*DeviceAck, error) {
	var out DeviceAck
	if err := c.do(ctx, http.MethodPost, "/api/agent/devices", nil, batch, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Visits(ctx context.Context, batch VisitBatch) (*VisitAck, error) {
	var out VisitAck
	if err := c.do(ctx, http.MethodPost, "/api/agent/visits", nil, batch, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	var body []byte
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = buf
	}
	full := c.base + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}

	var lastErr error
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		if attempt > 1 {
			delay := c.backoffDelay(attempt - 1)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		err := c.attempt(ctx, method, full, body, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !Retryable(err) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return fmt.Errorf("giving up after %d attempts: %w", c.maxAttempts, lastErr)
}

func (c *Client) attempt(ctx context.Context, method, full string, body []byte, out any) error {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, full, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{Status: resp.StatusCode, Code: "unknown", Message: string(data)}
		var envelope struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &envelope) == nil && envelope.Error.Code != "" {
			apiErr.Code = envelope.Error.Code
			apiErr.Message = envelope.Error.Message
		}
		return apiErr
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) backoffDelay(failedAttempts int) time.Duration {
	delay := c.baseDelay << (failedAttempts - 1)
	if delay > c.maxDelay || delay <= 0 {
		delay = c.maxDelay
	}
	jitter := 0
	if c.randomize != nil {
		jitter = c.randomize(int(c.baseDelay) + 1)
	}
	return delay + time.Duration(jitter)
}

// Retryable reports whether err is worth retrying. Permanent client errors
// (4xx other than 408/429) return false, so callers must not freeze a batch
// on them forever.
func Retryable(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable()
	}
	return true
}
