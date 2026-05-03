package hyprkeybinds

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
)

type Service struct {
	callback  func(map[string]any)
	mu        sync.RWMutex
	defaultKB string
	userKB    string
}

func New(cb func(map[string]any)) *Service {
	return &Service{callback: cb}
}

type KeybindCategory struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Children    []Keybind `json:"children"`
}

type Keybind struct {
	Key            string `json:"key"`
	Dispatcher     string `json:"dispatcher"`
	DispatcherArgs string `json:"dispatcher_args"`
}

type KeybindTree struct {
	Children []KeybindCategory `json:"children"`
}

func (s *Service) Run(ctx context.Context) error {
	s.Reload()
	<-ctx.Done()
	return ctx.Err()
}

func parseKeybindsFile(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return `{"children":[]}`
	}
	defer file.Close()

	var categories []KeybindCategory
	var current *KeybindCategory

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#!") {
			continue
		}

		if after, ok := strings.CutPrefix(line, "#"); ok {
			comment := after
			comment = strings.TrimSpace(comment)
			if comment == "" {
				continue
			}
			parts := strings.SplitN(comment, ":", 2)
			name := strings.TrimSpace(parts[0])
			desc := ""
			if len(parts) > 1 {
				desc = strings.TrimSpace(parts[1])
			}
			categories = append(categories, KeybindCategory{
				Name:        name,
				Description: desc,
			})
			current = &categories[len(categories)-1]
			continue
		}

		if !strings.HasPrefix(line, "bind") {
			continue
		}

		_, after, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		bindContent := strings.TrimSpace(after)

		fields := strings.SplitN(bindContent, ",", 4)
		if len(fields) < 3 {
			continue
		}

		mods := strings.TrimSpace(fields[0])
		key := strings.TrimSpace(fields[1])
		dispatcher := strings.TrimSpace(fields[2])
		args := ""
		if len(fields) > 3 {
			args = strings.TrimSpace(fields[3])
		}

		displayKey := key
		if mods != "" {
			displayKey = mods + " + " + key
		}

		kb := Keybind{
			Key:            displayKey,
			Dispatcher:     dispatcher,
			DispatcherArgs: args,
		}

		if current != nil {
			current.Children = append(current.Children, kb)
		} else {
			categories = append(categories, KeybindCategory{
				Name:     "Other",
				Children: []Keybind{kb},
			})
			current = &categories[len(categories)-1]
		}
	}

	tree := KeybindTree{Children: categories}
	out, err := json.Marshal(tree)
	if err != nil {
		return `{"children":[]}`
	}
	return string(out)
}

func (s *Service) Reload() {
	home, _ := os.UserHomeDir()
	if home == "" {
		return
	}

	defaultPath := home + "/.config/hypr/hyprland/keybinds.conf"
	userPath := home + "/.config/hypr/custom/keybinds.conf"

	s.mu.Lock()
	s.defaultKB = parseKeybindsFile(defaultPath)
	s.userKB = parseKeybindsFile(userPath)
	s.mu.Unlock()

	s.callback(map[string]any{
		"event": "hypr_keybinds",
		"data": map[string]any{
			"default": s.defaultKB,
			"user":    s.userKB,
		},
	})
}

func (s *Service) EmitSnapshot(callback func(map[string]any)) {
	s.mu.RLock()
	def := s.defaultKB
	usr := s.userKB
	s.mu.RUnlock()

	callback(map[string]any{
		"event": "hypr_keybinds",
		"data": map[string]any{
			"default": def,
			"user":    usr,
		},
	})
}
