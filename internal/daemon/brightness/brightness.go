package brightness

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"math"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Monitor struct {
	Name          string
	IsDDC         bool
	BusNum        string
	MaxBrightness int
	Current       float64 // 0.0 to 1.0
	Multiplier    float64 // anti-flashbang multiplier
}

type Config struct {
	ScreenshotDir           string
	WorkspaceAnimationDelay time.Duration
	ContentSwitchDelay      time.Duration
	Enabled                 bool
	PollInterval            time.Duration // for brightness polling, default 2s
}

func DefaultConfig() Config {
	return Config{
		ScreenshotDir:           "/tmp/quickshell/brightness/antiflashbang",
		WorkspaceAnimationDelay: 500 * time.Millisecond,
		ContentSwitchDelay:      500 * time.Millisecond,
		Enabled:                 true,
		PollInterval:            2 * time.Second,
	}
}

type Service struct {
	cfg      Config
	callback func(map[string]any)
	mu       sync.RWMutex

	monitors    []string
	multipliers map[string]float64
	monitors2   []Monitor
}

func New(cfg Config, cb func(map[string]any)) *Service {
	return &Service{
		cfg:         cfg,
		callback:    cb,
		multipliers: make(map[string]float64),
	}
}

func hyprEventSocketPath() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	instance := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	return runtimeDir + "/hypr/" + instance + "/.socket2.sock"
}

func (s *Service) Run(ctx context.Context) error {
	if !s.cfg.Enabled {
		log.Println("[brightness] anti-flashbang disabled")
		<-ctx.Done()
		return nil
	}

	instance := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if instance == "" {
		log.Println("[brightness] HYPRLAND_INSTANCE_SIGNATURE not set, skipping")
		<-ctx.Done()
		return nil
	}

	// Ensure screenshot directory exists
	if err := os.MkdirAll(s.cfg.ScreenshotDir, 0o755); err != nil {
		log.Printf("[brightness] failed to create screenshot dir: %v", err)
	}

	// Initial monitor detection
	s.refreshMonitors()

	// Initial brightness calculation
	s.calculateAllMonitors()

	s.refreshAll()

	go func() {
		ticker := time.NewTicker(s.cfg.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refreshAll()
			}
		}
	}()

	// Event loop with reconnection
	backoff := 500 * time.Millisecond
	maxBackoff := 5 * time.Second
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Println("[brightness] subscribing to hyprland events...")
		err := s.subscribeEvents(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		log.Printf("[brightness] event subscription ended: %v, reconnecting in %v", err, backoff)
		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (s *Service) refreshMonitors() {
	out, err := exec.Command("hyprctl", "-j", "monitors").Output()
	if err != nil {
		log.Printf("[brightness] hyprctl monitors error: %v", err)
		return
	}

	var monitors []map[string]any
	if err := json.Unmarshal(out, &monitors); err != nil {
		log.Printf("[brightness] parse monitors error: %v", err)
		return
	}

	names := make([]string, 0, len(monitors))
	for _, m := range monitors {
		if name, ok := m["name"].(string); ok {
			names = append(names, name)
		}
	}

	s.mu.Lock()
	s.monitors = names
	s.mu.Unlock()
}

func (s *Service) subscribeEvents(ctx context.Context) error {
	sockPath := hyprEventSocketPath()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("connect event socket %s: %w", sockPath, err)
	}
	defer func() { _ = conn.Close() }()

	eventCh := make(chan string, 64)
	doneCh := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				select {
				case eventCh <- line:
				default:
				}
			}
		}
		if err := scanner.Err(); err != nil {
			doneCh <- err
		} else {
			doneCh <- fmt.Errorf("event socket closed")
		}
	}()

	debounce := time.NewTimer(0)
	if !debounce.Stop() {
		<-debounce.C
	}

	pending := false

	flush := func() {
		// Delay for workspace animation to complete
		time.Sleep(s.cfg.WorkspaceAnimationDelay)
		time.Sleep(s.cfg.ContentSwitchDelay)
		s.calculateAllMonitors()
	}

	for {
		select {
		case <-ctx.Done():
			debounce.Stop()
			return ctx.Err()

		case err := <-doneCh:
			debounce.Stop()
			if pending {
				go flush()
			}
			return err

		case line := <-eventCh:
			parts := strings.SplitN(line, ">>", 2)
			if len(parts) != 2 {
				continue
			}
			eventName := strings.TrimSpace(parts[0])
			switch eventName {
			case "activewindowv2", "windowtitlev2",
				"workspace", "workspacev2",
				"openwindow", "closewindow":
				pending = true
				debounce.Reset(200 * time.Millisecond)
			case "monitoradded", "monitorremoved":
				s.refreshMonitors()
				pending = true
				debounce.Reset(200 * time.Millisecond)
			}

		case <-debounce.C:
			if pending {
				pending = false
				go flush()
			}
		}
	}
}

func (s *Service) calculateAllMonitors() {
	s.mu.RLock()
	monitors := make([]string, len(s.monitors))
	copy(monitors, s.monitors)
	s.mu.RUnlock()

	for _, screen := range monitors {
		multiplier, err := s.calculateBrightness(screen)
		if err != nil {
			log.Printf("[brightness] calculate %s error: %v", screen, err)
			continue
		}

		s.mu.Lock()
		s.multipliers[screen] = multiplier
		s.mu.Unlock()

		s.callback(map[string]any{
			"event": "brightness_multiplier",
			"data": map[string]any{
				"screen":     screen,
				"multiplier": multiplier,
			},
		})
	}
}

func (s *Service) calculateBrightness(screen string) (float64, error) {
	// grim -o <screen> - | magick png:- -resize 64x64 -colorspace Gray -format "%[fx:mean*100]" info:
	grim := exec.Command("grim", "-o", screen, "-")
	magick := exec.Command("magick", "png:", "-resize", "64x64", "-colorspace", "Gray", "-format", "%[fx:mean*100]", "info:")

	pipe, err := grim.StdoutPipe()
	if err != nil {
		return 0, err
	}
	magick.Stdin = pipe

	if err := grim.Start(); err != nil {
		return 0, err
	}

	out, err := magick.Output()
	_ = grim.Wait()

	if err != nil {
		return 0, err
	}

	val, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, err
	}

	// Apply the brightness multiplier formula
	multiplier := (6.600135 + 216.360356*math.Exp(-0.0811129189*val)) / 100.0

	return multiplier, nil
}

func (s *Service) detectDDCMonitors() map[string]string {
	out, err := exec.Command("ddcutil", "detect", "--brief").Output()
	if err != nil {
		return nil
	}
	result := make(map[string]string)
	blocks := strings.SplitSeq(string(out), "\n\n")
	for block := range blocks {
		lines := strings.Split(block, "\n")
		if len(lines) < 2 || !strings.HasPrefix(lines[0], "Display ") {
			continue
		}
		var name, busNum string
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "DRM connector:") {
				parts := strings.SplitN(l, "-", 2)
				if len(parts) > 1 {
					name = parts[1]
				}
			} else if strings.HasPrefix(l, "I2C bus:") {
				parts := strings.SplitN(l, "/dev/i2c-", 2)
				if len(parts) > 1 {
					busNum = parts[1]
				}
			}
		}
		if name != "" && busNum != "" {
			result[name] = busNum
		}
	}
	return result
}

func (s *Service) initMonitorBrightness(name string, isDDC bool, busNum string) (float64, int, error) {
	if isDDC {
		out, err := exec.Command("ddcutil", "-b", busNum, "getvcp", "10", "--brief").Output()
		if err != nil {
			return 0, 100, err
		}
		parts := strings.Fields(strings.TrimSpace(string(out)))
		if len(parts) >= 4 {
			current, _ := strconv.Atoi(parts[len(parts)-2])
			max, _ := strconv.Atoi(parts[len(parts)-1])
			if max > 0 {
				return float64(current) / float64(max), max, nil
			}
		}
		return 0.5, 100, nil
	}
	out, err := exec.Command("sh", "-c", "brightnessctl g && brightnessctl m").Output()
	if err != nil {
		return 0, 100, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) >= 2 {
		current, _ := strconv.Atoi(strings.TrimSpace(lines[0]))
		max, _ := strconv.Atoi(strings.TrimSpace(lines[1]))
		if max > 0 {
			return float64(current) / float64(max), max, nil
		}
	}
	return 0.5, 100, nil
}

func (s *Service) setBrightness(name string, value float64, isDDC bool, busNum string, maxBrightness int) error {
	if isDDC {
		rawValue := int(math.Max(math.Floor(value*float64(maxBrightness)), 1))
		return exec.Command("ddcutil", "-b", busNum, "setvcp", "10", strconv.Itoa(rawValue)).Run()
	}
	percent := int(math.Max(math.Floor(value*100), 1))
	return exec.Command("brightnessctl", "--class", "backlight", "s", strconv.Itoa(percent)+"%", "--quiet").Run()
}

func (s *Service) refreshAll() {
	ddcMonitors := s.detectDDCMonitors()

	out, err := exec.Command("hyprctl", "-j", "monitors").Output()
	if err != nil {
		return
	}
	var hyprMonitors []map[string]any
	if err := json.Unmarshal(out, &hyprMonitors); err != nil {
		return
	}

	monitors := make([]Monitor, 0, len(hyprMonitors))
	usedBusNums := make(map[string]bool)

	for _, m := range hyprMonitors {
		name, _ := m["name"].(string)
		if name == "" {
			continue
		}

		busNum, isDDC := ddcMonitors[name]
		if isDDC && usedBusNums[busNum] {
			isDDC = false
		}
		if isDDC {
			usedBusNums[busNum] = true
		}

		current, max, err := s.initMonitorBrightness(name, isDDC, busNum)
		if err != nil {
			current = 0.5
			max = 100
		}

		s.mu.RLock()
		var multiplier float64 = 1.0
		if mult, ok := s.multipliers[name]; ok {
			multiplier = mult
		}
		s.mu.RUnlock()

		monitors = append(monitors, Monitor{
			Name:          name,
			IsDDC:         isDDC,
			BusNum:        busNum,
			MaxBrightness: max,
			Current:       current,
			Multiplier:    multiplier,
		})
	}

	s.mu.Lock()
	s.monitors2 = monitors
	s.mu.Unlock()

	s.emitBrightness()
}

func (s *Service) emitBrightness() {
	s.mu.RLock()
	monitors := make([]Monitor, len(s.monitors2))
	copy(monitors, s.monitors2)
	s.mu.RUnlock()

	data := make(map[string]any)
	for _, m := range monitors {
		data[m.Name] = map[string]any{
			"brightness": m.Current,
			"multiplier": m.Multiplier,
			"isDDC":      m.IsDDC,
			"maxRaw":     m.MaxBrightness,
		}
	}

	s.callback(map[string]any{
		"event": "brightness",
		"data":  data,
	})
}

func (s *Service) SetBrightness(screen string, value float64) {
	s.mu.Lock()
	for i := range s.monitors2 {
		if s.monitors2[i].Name == screen {
			value = math.Max(0, math.Min(1, value))
			s.monitors2[i].Current = value
			isDDC := s.monitors2[i].IsDDC
			busNum := s.monitors2[i].BusNum
			maxBrightness := s.monitors2[i].MaxBrightness
			s.mu.Unlock()

			mult := 1.0
			s.mu.RLock()
			if m, ok := s.multipliers[screen]; ok {
				mult = m
			}
			s.mu.RUnlock()

			effectiveValue := value * mult
			go s.setBrightness(screen, effectiveValue, isDDC, busNum, maxBrightness)
			s.emitBrightness()
			return
		}
	}
	s.mu.Unlock()
}

func (s *Service) IncrementBrightness(screen string, delta float64) {
	s.mu.Lock()
	for i := range s.monitors2 {
		if s.monitors2[i].Name == screen {
			newVal := math.Max(0, math.Min(1, s.monitors2[i].Current+delta))
			s.monitors2[i].Current = newVal
			isDDC := s.monitors2[i].IsDDC
			busNum := s.monitors2[i].BusNum
			maxBrightness := s.monitors2[i].MaxBrightness
			s.mu.Unlock()

			mult := 1.0
			s.mu.RLock()
			if m, ok := s.multipliers[screen]; ok {
				mult = m
			}
			s.mu.RUnlock()

			go s.setBrightness(screen, newVal*mult, isDDC, busNum, maxBrightness)
			s.emitBrightness()
			return
		}
	}
	s.mu.Unlock()
}

func (s *Service) GetBrightness(screen string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.monitors2 {
		if m.Name == screen {
			return m.Current
		}
	}
	return 0.5
}

func (s *Service) GetSnapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]any)
	for k, v := range s.multipliers {
		result[k] = v
	}
	return result
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.mu.RLock()
	mults := make(map[string]float64, len(s.multipliers))
	maps.Copy(mults, s.multipliers)
	monitors := make([]Monitor, len(s.monitors2))
	copy(monitors, s.monitors2)
	s.mu.RUnlock()

	for screen, multiplier := range mults {
		callback(map[string]any{
			"event": "brightness_multiplier",
			"data": map[string]any{
				"screen":     screen,
				"multiplier": multiplier,
			},
		})
	}

	s.emitBrightness()
}
