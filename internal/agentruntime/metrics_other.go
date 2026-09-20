//go:build !linux

package agentruntime

import (
	"errors"
	"sync"
)

type Metrics struct {
	CPUPercent    float64
	MemoryPercent float64
	DiskPercent   float64
	UptimeSeconds int64
}

type MetricsCollector struct {
	mu sync.Mutex
}

func NewMetricsCollector() *MetricsCollector { return &MetricsCollector{} }

func (m *MetricsCollector) Collect() (Metrics, error) {
	return Metrics{}, errors.New("system metrics collection is only supported on linux")
}
