package resources

import (
	"context"
	"log"
	"maps"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Interval time.Duration
}

func DefaultConfig() Config {
	return Config{Interval: 3 * time.Second}
}

type Service struct {
	cfg      Config
	callback func(map[string]any)
	mu       sync.RWMutex
	lastData map[string]any
}

func New(cfg Config, cb func(map[string]any)) *Service {
	return &Service{
		cfg:      cfg,
		callback: cb,
		lastData: make(map[string]any),
	}
}

func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	var prevStat cpuStat
	initialized := false

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			data, err := s.readResources()
			if err != nil {
				log.Printf("[resources] read error: %v", err)
				continue
			}

			var cpuUsage float64
			cpuUsage, prevStat = s.calculateCPU(prevStat, initialized)
			data["cpuUsage"] = cpuUsage
			initialized = true

			if s.hasChanged(data) {
				s.mu.Lock()
				s.lastData = data
				s.mu.Unlock()

				m := map[string]any{
					"event": "resources",
					"data":  data,
				}
				s.callback(m)
			}
		}
	}
}

type cpuStat struct {
	user, nice, system, idle, iowait, irq, softirq uint64
}

func (s *Service) readResources() (map[string]any, error) {
	data := make(map[string]any)

	memData, err := s.readMeminfo()
	if err == nil {
		maps.Copy(data, memData)
	}

	return data, nil
}

func (s *Service) readMeminfo() (map[string]any, error) {
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil, err
	}

	result := make(map[string]any)
	lines := strings.SplitSeq(string(content), "\n")

	for line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			result["memoryTotal"] = parseMemValue(line)
		} else if strings.HasPrefix(line, "MemFree:") {
			result["memoryFree"] = parseMemValue(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			result["memoryAvailable"] = parseMemValue(line)
		} else if strings.HasPrefix(line, "SwapTotal:") {
			result["swapTotal"] = parseMemValue(line)
		} else if strings.HasPrefix(line, "SwapFree:") {
			result["swapFree"] = parseMemValue(line)
		}
	}

	return result, nil
}

func parseMemValue(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	val, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return val * 1024
}

func (s *Service) calculateCPU(prev cpuStat, initialized bool) (float64, cpuStat) {
	content, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, prev
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return 0, prev
	}

	fields := strings.Fields(lines[0])
	if len(fields) < 8 || fields[0] != "cpu" {
		return 0, prev
	}

	stat := cpuStat{
		user:    parseUint64(fields[1]),
		nice:    parseUint64(fields[2]),
		system:  parseUint64(fields[3]),
		idle:    parseUint64(fields[4]),
		iowait:  parseUint64(fields[5]),
		irq:     parseUint64(fields[6]),
		softirq: parseUint64(fields[7]),
	}

	if !initialized || prev.idle == 0 {
		return 0, stat
	}

	currentIdle := stat.idle + stat.iowait
	prevIdle := prev.idle + prev.iowait

	currentTotal := stat.user + stat.nice + stat.system + stat.idle + stat.iowait + stat.irq + stat.softirq
	prevTotal := prev.user + prev.nice + prev.system + prev.idle + prev.iowait + prev.irq + prev.softirq

	totalDiff := int64(currentTotal - prevTotal)
	idleDiff := int64(currentIdle - prevIdle)

	if totalDiff == 0 {
		return 0, stat
	}

	usage := float64(totalDiff-idleDiff) / float64(totalDiff)
	if usage < 0 {
		return 0, stat
	}

	return usage, stat
}

func parseUint64(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

func (s *Service) hasChanged(data map[string]any) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.lastData == nil {
		return true
	}

	threshold := 0.01

	check := func(key string) bool {
		newVal, ok := data[key].(uint64)
		if !ok {
			return true
		}
		oldVal, ok := s.lastData[key].(uint64)
		if !ok {
			return true
		}
		return oldVal == 0 || (newVal > oldVal+1024*1024) || (oldVal > newVal+1024*1024)
	}

	if check("memoryTotal") || check("memoryFree") || check("swapTotal") || check("swapFree") {
		return true
	}

	newCPU, ok := data["cpuUsage"].(float64)
	if !ok {
		return true
	}
	oldCPU, ok := s.lastData["cpuUsage"].(float64)
	if !ok {
		return true
	}

	return newCPU < oldCPU-threshold || newCPU > oldCPU+threshold
}

func (s *Service) GetSnapshot() map[string]any {
	data, _ := s.readResources()

	var prevStat cpuStat
	cpuUsage, _ := s.calculateCPU(prevStat, false)
	data["cpuUsage"] = cpuUsage

	return data
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	data := s.GetSnapshot()
	callback(map[string]any{
		"event": "resources",
		"data":  data,
	})
}
