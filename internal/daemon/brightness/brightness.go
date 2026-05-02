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

type Config struct {
	ScreenshotDir           string
	WorkspaceAnimationDelay time.Duration
	ContentSwitchDelay      time.Duration
	Enabled                 bool
}

func DefaultConfig() Config {
	return Config{
		ScreenshotDir:           "/tmp/quickshell/brightness/antiflashbang",
		WorkspaceAnimationDelay: 500 * time.Millisecond,
		ContentSwitchDelay:      500 * time.Millisecond,
		Enabled:                 true,
	}
}

type Service struct {
	cfg      Config
	callback func(map[string]any)
	mu       sync.RWMutex

	monitors    []string
	multipliers map[string]float64
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
}
