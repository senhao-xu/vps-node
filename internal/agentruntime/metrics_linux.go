//go:build linux

package agentruntime

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Metrics struct {
	CPUPercent    float64
	MemoryPercent float64
	DiskPercent   float64
	UptimeSeconds int64
}

type MetricsCollector struct {
	mu       sync.Mutex
	prev     *cpuSample
	minDelay time.Duration
}

type cpuSample struct {
	total float64
	idle  float64
	at    time.Time
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{minDelay: 250 * time.Millisecond}
}

func (m *MetricsCollector) Collect() (Metrics, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	mem, err := memoryPercent()
	if err != nil {
		return Metrics{}, err
	}
	disk, err := diskPercent("/")
	if err != nil {
		return Metrics{}, err
	}
	uptime, err := systemUptime()
	if err != nil {
		return Metrics{}, err
	}
	cpu, err := m.cpuPercent()
	if err != nil {
		return Metrics{}, err
	}
	return Metrics{
		CPUPercent:    cpu,
		MemoryPercent: mem,
		DiskPercent:   disk,
		UptimeSeconds: uptime,
	}, nil
}

func (m *MetricsCollector) cpuPercent() (float64, error) {
	if m.prev != nil && time.Since(m.prev.at) < m.minDelay {
		time.Sleep(m.minDelay - time.Since(m.prev.at))
	}
	sample, err := readCPUSample()
	if err != nil {
		return 0, err
	}
	if m.prev == nil {
		m.prev = sample
		return 0, nil
	}
	prev := m.prev
	m.prev = sample
	totalDelta := sample.total - prev.total
	idleDelta := sample.idle - prev.idle
	if totalDelta <= 0 {
		return 0, nil
	}
	used := (totalDelta - idleDelta) / totalDelta * 100
	if used < 0 {
		used = 0
	}
	if used > 100 {
		used = 100
	}
	return used, nil
}

func readCPUSample() (*cpuSample, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return nil, fmt.Errorf("unexpected /proc/stat cpu line %q", line)
		}
		var total, idle float64
		for i, f := range fields[1:] {
			v, err := strconv.ParseFloat(f, 64)
			if err != nil {
				return nil, fmt.Errorf("parse /proc/stat: %w", err)
			}
			total += v
			if i == 3 || i == 4 {
				idle += v
			}
		}
		return &cpuSample{total: total, idle: idle, at: time.Now()}, nil
	}
	return nil, fmt.Errorf("cpu line not found in /proc/stat")
}

func memoryPercent() (float64, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	var total, available float64
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = meminfoKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			available = meminfoKB(line)
		}
	}
	if total <= 0 {
		return 0, fmt.Errorf("MemTotal missing in /proc/meminfo")
	}
	used := (total - available) / total * 100
	if used < 0 {
		used = 0
	}
	if used > 100 {
		used = 100
	}
	return used, nil
}

func meminfoKB(line string) float64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0
	}
	return v
}

func diskPercent(path string) (float64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	blocks := float64(st.Blocks)
	free := float64(st.Bfree)
	if blocks <= 0 {
		return 0, fmt.Errorf("invalid block count for %s", path)
	}
	used := (blocks - free) / blocks * 100
	if used < 0 {
		used = 0
	}
	if used > 100 {
		used = 100
	}
	return used, nil
}

func systemUptime() (int64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("unexpected /proc/uptime content")
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse /proc/uptime: %w", err)
	}
	if v < 0 {
		v = 0
	}
	return int64(v), nil
}
