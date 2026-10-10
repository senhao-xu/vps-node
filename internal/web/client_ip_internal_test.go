package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestObservedClientIP(t *testing.T) {
	newReq := func(remote string, headers map[string]string) *http.Request {
		r := httptest.NewRequest("POST", "/api/agent/heartbeat", nil)
		r.RemoteAddr = remote
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		return r
	}

	tests := []struct {
		name    string
		remote  string
		headers map[string]string
		want    string
	}{
		{"public peer ignores headers", "203.0.113.5:4444", map[string]string{"X-Forwarded-For": "198.51.100.1"}, "203.0.113.5"},
		{"loopback peer honors xff", "127.0.0.1:5000", map[string]string{"X-Forwarded-For": "203.0.113.77"}, "203.0.113.77"},
		{"docker peer honors xff chain", "172.18.0.3:5000", map[string]string{"X-Forwarded-For": "198.51.100.9, 10.0.0.2"}, "198.51.100.9"},
		{"private peer honors x-real-ip", "192.168.1.2:5000", map[string]string{"X-Real-IP": "198.51.100.3"}, "198.51.100.3"},
		{"private peer without headers is dropped", "172.17.0.1:5000", nil, ""},
		{"private forwarded value is dropped", "10.1.1.1:5000", map[string]string{"X-Forwarded-For": "172.20.0.9"}, ""},
		{"invalid peer falls back to raw", "not-an-addr", nil, "not-an-addr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := observedClientIP(newReq(tt.remote, tt.headers)); got != tt.want {
				t.Fatalf("observedClientIP()=%q want %q", got, tt.want)
			}
		})
	}
}
