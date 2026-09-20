//go:build linux

package agentruntime

import (
	"testing"
)

func TestMetricsCollectorSaneValues(t *testing.T) {
	c := NewMetricsCollector()
	m, err := c.Collect()
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if m.CPUPercent < 0 || m.CPUPercent > 100 {
		t.Fatalf("cpu percent out of range: %v", m.CPUPercent)
	}
	if m.MemoryPercent <= 0 || m.MemoryPercent > 100 {
		t.Fatalf("memory percent out of range: %v", m.MemoryPercent)
	}
	if m.DiskPercent <= 0 || m.DiskPercent > 100 {
		t.Fatalf("disk percent out of range: %v", m.DiskPercent)
	}
	if m.UptimeSeconds <= 0 {
		t.Fatalf("uptime must be positive, got %d", m.UptimeSeconds)
	}

	second, err := c.Collect()
	if err != nil {
		t.Fatalf("second collect: %v", err)
	}
	if second.CPUPercent < 0 || second.CPUPercent > 100 {
		t.Fatalf("second cpu percent out of range: %v", second.CPUPercent)
	}
}
