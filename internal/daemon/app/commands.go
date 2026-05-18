package app

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/hyprland"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/proc"
	imgPkg "github.com/sonroyaalmerol/snry-shell-qs/internal/image"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/wallpaper"
)

const maxKey = 248

func dispatchCommand(a *App, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}
	switch fields[0] {
	// ── Input ──────────────────────────────────────────────
	case "press":
		if len(fields) != 2 {
			return
		}
		code, err := strconv.ParseUint(fields[1], 10, 16)
		if err != nil || code > maxKey {
			return
		}
		a.uinput.Press(uint16(code))
	case "release":
		if len(fields) != 2 {
			return
		}
		code, err := strconv.ParseUint(fields[1], 10, 16)
		if err != nil || code > maxKey {
			return
		}
		a.uinput.Release(uint16(code))
	case "releaseall":
		a.uinput.ReleaseAll()
	case "combo":
		if len(fields) < 2 {
			return
		}
		var codes []uint16
		for i := 1; i < len(fields); i++ {
			code, err := strconv.ParseUint(fields[i], 10, 16)
			if err != nil || code > maxKey {
				return
			}
			codes = append(codes, uint16(code))
		}
		a.uinput.Combo(codes)
	case "auth":
		if len(fields) < 2 || a.lockscreenSvc == nil {
			return
		}
		password := strings.Join(fields[1:], " ")
		go a.lockscreenSvc.Authenticate(password)

	case "lock":
		if len(fields) >= 2 && fields[1] == "startup" {
			if a.lockscreenSvc != nil {
				a.lockscreenSvc.LockWithAutoUnlock()
			} else if a.idleSvc != nil {
				a.idleSvc.Lock()
			}
			return
		}
		if a.lockscreenSvc != nil {
			if !a.lockscreenSvc.IsLocked() {
				a.lockscreenSvc.Lock()
				go a.dispatchQsLock()
			}
		} else if a.idleSvc != nil {
			a.idleSvc.Lock()
		}
	case "unlock":
		if a.lockscreenSvc != nil {
			a.lockscreenSvc.Unlock()
		} else if a.idleSvc != nil {
			a.idleSvc.Unlock()
		}
	case "auto-unlock":
		if a.lockscreenSvc != nil {
			a.lockscreenSvc.TryAutoUnlock()
		} else if a.idleSvc != nil {
			a.idleSvc.Unlock()
		}
	case "power-button", "lid-close":
		if a.idleSvc != nil {
			a.idleSvc.SuppressDisplayOn(true)
		}
		if a.lockscreenSvc != nil {
			a.lockscreenSvc.Lock()
		}
		if a.idleSvc != nil {
			a.idleSvc.SetDisplay(false)
		}
	case "resources":
		if a.resourcesSvc != nil {
			a.resourcesSvc.EmitSnapshot(a.socketServer.Emitter().Emit)
		}
	case "weather":
		if len(fields) >= 2 && fields[1] == "refresh" && a.weatherSvc != nil {
			go a.weatherSvc.RefreshNow(context.Background())
		}
	case "cliphist":
		if len(fields) < 2 || a.cliphistSvc == nil {
			return
		}
		switch fields[1] {
		case "list":
			go a.cliphistSvc.EmitList(context.Background())
		case "delete":
			entry := strings.TrimPrefix(line, "cliphist delete ")
			go a.cliphistSvc.DeleteEntry(context.Background(), entry)
		case "wipe":
			go a.cliphistSvc.Wipe(context.Background())
		}
	case "autoscale":
		go a.handleAutoscale()
	case "checkdeps":
		go a.handleCheckdeps()
	case "diagnose":
		go a.handleDiagnose()
	case "config":
		a.routeConfig(fields[1:])
	case "reload":
		go a.hyprlandSvc.Reload()
	case "zoom":
		a.handleZoom(fields[1:])
	case "ai-summary":
		a.handleAiSummary()
	case "start-geoclue":
		a.handleStartGeoclue()
	case "launch":
		rest := strings.TrimPrefix(line, "launch ")
		args := parseQuotedArgs(rest)
		go a.handleLaunch(args)
	case "snip-search":
		go a.handleSnipSearch()
	case "emoji":
		mode := "type"
		if len(fields) >= 2 {
			mode = fields[1]
		}
		go a.handleEmoji(mode)
	case "random-wallpaper":
		source := "konachan"
		if len(fields) >= 2 {
			source = fields[1]
		}
		go a.handleRandomWallpaper(source)
	case "generate-thumbnails":
		size := "normal"
		mode := ""
		target := ""
		for i := 1; i < len(fields); i++ {
			switch fields[i] {
			case "--size", "-s":
				if i+1 < len(fields) {
					size = fields[i+1]
					i++
				}
			case "--file", "-f":
				mode = "file"
				if i+1 < len(fields) {
					target = fields[i+1]
					i++
				}
			case "--directory", "-d":
				mode = "dir"
				if i+1 < len(fields) {
					target = fields[i+1]
					i++
				}
			}
		}
		if mode != "" && target != "" {
			go a.handleGenerateThumbnails(size, mode, target)
		}

	// State commands — forwarded to state loop.
	case "set-mode":
		if len(fields) >= 2 {
			a.stateCh <- stateEvent{kind: "command", cmd: "set-mode", arg: fields[1]}
		}
	case "cycle-mode":
		a.stateCh <- stateEvent{kind: "command", cmd: "cycle-mode"}
	case "osk":
		if len(fields) < 2 {
			return
		}
		a.stateCh <- stateEvent{kind: "command", cmd: "osk-" + fields[1]}

	// New daemon service commands
	case "keybinds":
		if len(fields) >= 2 && fields[1] == "reload" && a.hyprKeybindsSvc != nil {
			a.hyprKeybindsSvc.Reload()
		}
	case "easyeffects":
		if len(fields) < 2 || a.easyEffectsSvc == nil {
			return
		}
		switch fields[1] {
		case "toggle":
			if a.easyEffectsSvc.IsActive() {
				a.easyEffectsSvc.Disable()
			} else {
				a.easyEffectsSvc.Enable()
			}
		case "enable":
			a.easyEffectsSvc.Enable()
		case "disable":
			a.easyEffectsSvc.Disable()
		}
	case "hyprsunset":
		if len(fields) < 2 || a.hyprsunsetSvc == nil {
			return
		}
		switch fields[1] {
		case "gamma":
			if len(fields) >= 3 {
				if gamma, err := strconv.Atoi(fields[2]); err == nil {
					a.hyprsunsetSvc.SetGamma(gamma)
				}
			}
		case "enable":
			a.hyprsunsetSvc.EnableTemperature()
		case "disable":
			a.hyprsunsetSvc.DisableTemperature()
		case "toggle":
			active := true
			if len(fields) >= 3 {
				active = fields[2] != "false" && fields[2] != "0"
			}
			a.hyprsunsetSvc.ToggleTemperature(active)
		}
	case "wifi":
		if len(fields) < 2 || a.networkSvc == nil {
			return
		}
		switch fields[1] {
		case "enable":
			a.networkSvc.EnableWifi(context.Background(), true)
		case "disable":
			a.networkSvc.EnableWifi(context.Background(), false)
		case "toggle":
			a.networkSvc.ToggleWifi(context.Background())
		case "rescan":
			go a.networkSvc.RescanWifi(context.Background())
		case "connect":
			if len(fields) >= 3 {
				ssid := strings.Join(fields[2:], " ")
				go func() {
					err := a.networkSvc.ConnectWifi(context.Background(), ssid)
					if err != nil {
						log.Printf("[app] wifi connect: %v", err)
						a.socketServer.Emitter().Emit(map[string]any{
							"event": "network_connect_result",
							"data": map[string]any{
								"success":        false,
								"askingPassword": true,
								"ssid":           ssid,
							},
						})
					}
				}()
			}
		case "disconnect":
			if len(fields) >= 3 {
				go a.networkSvc.DisconnectWifi(context.Background(), strings.Join(fields[2:], " "))
			}
		case "change-password":
			if len(fields) >= 4 {
				go a.networkSvc.ChangePassword(context.Background(), fields[2], fields[3])
			}
		}
	case "brightness":
		if len(fields) < 3 || a.brightnessSvc == nil {
			return
		}
		switch fields[1] {
		case "set":
			if len(fields) >= 4 {
				value, err := strconv.ParseFloat(fields[3], 64)
				if err == nil {
					a.brightnessSvc.SetBrightness(fields[2], value)
				}
			}
		case "increment":
			if len(fields) >= 4 {
				delta, err := strconv.ParseFloat(fields[3], 64)
				if err == nil {
					a.brightnessSvc.IncrementBrightness(fields[2], delta)
				}
			}
		case "get":
			value := a.brightnessSvc.GetBrightness(fields[2])
			a.socketServer.Emitter().Emit(map[string]any{
				"event": "brightness_value",
				"data": map[string]any{
					"screen":     fields[2],
					"brightness": value,
				},
			})
		}
	case "gamemode":
		if len(fields) < 2 || a.gamemodeSvc == nil {
			return
		}
		switch fields[1] {
		case "enable":
			go a.gamemodeSvc.Enable()
		case "disable":
			go a.gamemodeSvc.Disable()
		case "toggle":
			go a.gamemodeSvc.Toggle()
		}
	case "conflict":
		if len(fields) >= 2 && fields[1] == "check" && a.conflictSvc != nil {
			go a.handleConflictCheck()
		}
	case "fprintd":
		if len(fields) >= 2 && fields[1] == "check" {
			user := os.Getenv("USER")
			if user == "" {
				user = "root"
			}
			out, err := exec.Command("fprintd-list", user).Output()
			if err != nil {
				a.socketServer.Emitter().Emit(map[string]any{
					"event": "fprintd_result",
					"data":  map[string]any{"available": false, "enrolled": false},
				})
				return
			}
			output := string(out)
			enrolled := strings.Contains(output, "Finger") || strings.Contains(output, "finger")
			a.socketServer.Emitter().Emit(map[string]any{
				"event": "fprintd_result",
				"data":  map[string]any{"available": true, "enrolled": enrolled},
			})
		}
	case "fps":
		if len(fields) >= 3 && fields[1] == "set" {
			fpsValue := fields[2]
			if _, err := strconv.Atoi(fpsValue); err != nil {
				return
			}
			cfgPaths := []string{
				os.ExpandEnv("$HOME/.config/MangoHud/MangoHud.conf"),
			}
			for _, path := range cfgPaths {
				data, err := os.ReadFile(path)
				if err != nil {
					os.WriteFile(path, []byte("fps_limit="+fpsValue+"\n"), 0644)
					continue
				}
				lines := strings.Split(string(data), "\n")
				found := false
				for i, line := range lines {
					if strings.HasPrefix(line, "fps_limit=") {
						lines[i] = "fps_limit=" + fpsValue
						found = true
						break
					}
				}
				if !found {
					lines = append(lines, "fps_limit="+fpsValue)
				}
				os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
			}
			exec.Command("pkill", "-SIGUSR2", "mangohud").Run()
		}
	case "hyprconfig":
		if len(fields) < 3 {
			return
		}
		switch fields[1] {
		case "get":
			key := strings.Join(fields[2:], " ")
			out, err := a.hyprlandSvc.GetOption(key)
			if err != nil {
				return
			}
			a.socketServer.Emitter().Emit(map[string]any{
				"event": "hyprconfig_value",
				"data": map[string]any{
					"key":   key,
					"value": strings.TrimSpace(string(out)),
				},
			})
		case "set":
			if len(fields) >= 4 {
				a.hyprlandSvc.SetOption(fields[2], strings.Join(fields[3:], " "))
			}
		case "reset":
			a.hyprlandSvc.ResetOption(strings.Join(fields[2:], " "))
		case "edit":
			if len(fields) >= 3 {
				go a.handleHyprconfigEdit(fields[1:])
			}
		}
	case "record":
		go a.handleRecord(fields[1:])
	case "keyring":
		if len(fields) < 2 {
			return
		}
		switch fields[1] {
		case "check":
			go a.handleKeyringCheck()
		case "lookup":
			go a.handleKeyringLookup()
		case "unlock":
			if len(fields) >= 3 {
				go a.handleKeyringUnlock(fields[2])
			}
		}
	case "capslock":
		if len(fields) >= 2 && fields[1] == "check" && a.hyprlandSvc != nil {
			go a.handleCapslockCheck()
		}
	case "battery":
		if len(fields) >= 2 && fields[1] == "status" {
			go a.handleBatteryStatus()
		}
	case "restore-video-wallpaper":
		go a.handleRestoreVideoWallpaper()
	case "apply-colors":
		if len(fields) >= 2 {
			switch fields[1] {
			case "terminal":
				go a.handleApplyTerminalColors()
			case "vscode":
				go a.handleApplyVscodeColor()
			case "kvantum":
				go a.handleApplyKvantumTheme()
			case "nvim":
				go a.handleNvimApplyColors()
			}
		}
	case "switch-wallpaper":
		go a.handleSwitchWallpaper(fields[1:])
	case "find-regions":
		go a.handleFindRegions(fields[1:])
	case "least-busy-region":
		go a.handleLeastBusyRegion(fields[1:])
	case "text-color":
		go a.handleTextColor(fields[1:])
	case "restart":
		a.softReload()
	}
}

// dispatchQsLock sends a lock command to Quickshell via IPC.
// This is more reliable than the socket-based DaemonSocket for triggering
// the QML lock screen.
func (a *App) dispatchQsLock() {
	binary := a.cfg.QuickshellCfg.Binary
	configPath := a.cfg.QuickshellCfg.ConfigPath
	if err := exec.Command(binary, "-p", configPath, "ipc", "call", "lock", "activate").Run(); err != nil {
		log.Printf("[app] qs ipc lock failed: %v", err)
	}
}

func (a *App) routeConfig(args []string) {
	if len(args) < 1 || a.hyprlandSvc == nil {
		return
	}
	switch args[0] {
	case "set":
		if len(args) >= 3 {
			go a.hyprlandSvc.SetOption(args[1], strings.Join(args[2:], " "))
		}
	case "reset":
		if len(args) >= 2 {
			go a.hyprlandSvc.ResetOption(args[1])
		}
	case "animation":
		// config animation <leaf> <enabled> <speed> <curve> [style]
		if len(args) >= 5 {
			enabled := args[2] == "true"
			speed, _ := strconv.ParseFloat(args[3], 64)
			style := ""
			if len(args) >= 6 {
				style = args[5]
			}
			go a.hyprlandSvc.SetAnimation(args[1], enabled, speed, args[4], style)
		}
	}
}

func (a *App) handleZoom(args []string) {
	if len(args) < 1 || a.hyprlandSvc == nil {
		return
	}
	action := args[0]
	step := 0.3
	if len(args) >= 2 {
		if s, err := strconv.ParseFloat(args[1], 64); err == nil {
			step = s
		}
	}

	if action == "reset" {
		if err := a.hyprlandSvc.SetOption("cursor.zoom_factor", "1.0"); err != nil {
			log.Printf("[app] zoom reset: %v", err)
		}
		return
	}

	data, err := a.hyprlandSvc.GetOption("cursor.zoom_factor")
	if err != nil {
		log.Printf("[app] zoom getoption: %v", err)
		return
	}
	var opt struct {
		Float float64 `json:"float"`
	}
	if err := json.Unmarshal(data, &opt); err != nil {
		log.Printf("[app] zoom parse: %v", err)
		return
	}

	var newZoom float64
	switch action {
	case "increase":
		newZoom = math.Min(opt.Float+step, 3.0)
	case "decrease":
		newZoom = math.Max(opt.Float-step, 1.0)
	default:
		return
	}

	if err := a.hyprlandSvc.SetOption("cursor.zoom_factor", fmt.Sprintf("%f", newZoom)); err != nil {
		log.Printf("[app] zoom set: %v", err)
	}
}

func (a *App) handleAiSummary() {
	go func() {
		sel, err := exec.Command("wl-paste", "-p").Output()
		if err != nil || len(sel) == 0 {
			sel, err = exec.Command("xclip", "-selection", "primary", "-o").Output()
		}
		if err != nil || len(sel) == 0 {
			exec.Command("notify-send", "AI Summary", "No text selected", "-a", "Hyprland").Run()
			return
		}

		resp, err := exec.Command("ollama", "run", "llama3.2", "Summarize the following text concisely:\n"+string(sel)).Output()
		if err != nil || len(resp) == 0 {
			exec.Command("notify-send", "AI Summary", "Failed to get response from ollama", "-a", "Hyprland", "-u", "critical").Run()
			return
		}

		wc := exec.Command("wl-copy")
		wc.Stdin = strings.NewReader(string(resp))
		if err := wc.Run(); err != nil {
			log.Printf("[app] ai-summary wl-copy: %v", err)
		}

		exec.Command("notify-send", "AI Summary", string(resp), "-a", "Hyprland", "-t", "10000").Run()
	}()
}

func (a *App) handleStartGeoclue() {
	out, err := exec.Command("pgrep", "-f", "geoclue-2.0/demos/agent").Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		return
	}

	paths := []string{
		"/usr/libexec/geoclue-2.0/demos/agent",
		"/usr/lib/geoclue-2.0/demos/agent",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			cmd := exec.Command(p)
			if err := cmd.Start(); err != nil {
				log.Printf("[app] start-geoclue: %v", err)
			}
			return
		}
	}
	log.Printf("[app] start-geoclue: agent not found")
}

func parseQuotedArgs(s string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '"':
			inQuote = !inQuote
		case ch == ' ' && !inQuote:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

func (a *App) handleLaunch(candidates []string) {
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		// Extract the binary name (first word) for LookPath check.
		bin := strings.Fields(candidate)[0]
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		cmd := exec.Command("sh", "-c", candidate)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			log.Printf("[app] launch start %q: %v", candidate, err)
			continue
		}
		return
	}
}

func (a *App) handleSnipSearch() {
	// Capture region via grim+slurp.
	if err := exec.Command("sh", "-c", `grim -g "$(slurp)" /tmp/image.png`).Run(); err != nil {
		log.Printf("[app] snip-search grim: %v", err)
		return
	}

	// Upload to uguu.se.
	out, err := exec.Command("curl", "-sF", "files[]=@/tmp/image.png", "https://uguu.se/upload").Output()
	if err != nil {
		log.Printf("[app] snip-search upload: %v", err)
		os.Remove("/tmp/image.png")
		return
	}
	var resp struct {
		Files []struct {
			URL string `json:"url"`
		} `json:"files"`
	}
	if err := json.Unmarshal(out, &resp); err != nil || len(resp.Files) == 0 {
		log.Printf("[app] snip-search parse response: %v", err)
		os.Remove("/tmp/image.png")
		return
	}

	// Open Google Lens.
	url := fmt.Sprintf("https://lens.google.com/uploadbyurl?url=%s", resp.Files[0].URL)
	if err := exec.Command("xdg-open", url).Start(); err != nil {
		log.Printf("[app] snip-search open: %v", err)
	}

	os.Remove("/tmp/image.png")
}

func (a *App) handleEmoji(mode string) {
	// Pipe emoji data to fuzzel for selection.
	fuzzel := exec.Command("fuzzel", "--match-mode", "fzf", "--dmenu")
	fuzzel.Stdin = strings.NewReader(emojiData)
	out, err := fuzzel.Output()
	if err != nil {
		return // user cancelled
	}
	// Get first field (the emoji).
	emoji := strings.TrimSpace(string(out))
	if emoji == "" {
		return
	}
	fields := strings.Fields(emoji)
	if len(fields) == 0 {
		return
	}
	emojiChar := fields[0]

	switch mode {
	case "copy":
		wc := exec.Command("wl-copy")
		wc.Stdin = strings.NewReader(emojiChar)
		if err := wc.Run(); err != nil {
			log.Printf("[app] emoji wl-copy: %v", err)
		}
	case "type":
		if err := exec.Command("wtype", emojiChar).Run(); err != nil {
			log.Printf("[app] emoji wtype: %v", err)
			// Fallback to wl-copy.
			wc := exec.Command("wl-copy")
			wc.Stdin = strings.NewReader(emojiChar)
			if err := wc.Run(); err != nil {
				log.Printf("[app] emoji wl-copy fallback: %v", err)
			}
		}
	}
}

func getPicturesDir() string {
	out, err := exec.Command("xdg-user-dir", "PICTURES").Output()
	if err == nil {
		if dir := strings.TrimSpace(string(out)); dir != "" {
			return dir
		}
	}
	return filepath.Join(os.Getenv("HOME"), "Pictures")
}

func (a *App) handleRandomWallpaper(source string) {
	picturesDir := getPicturesDir()
	wallpapersDir := filepath.Join(picturesDir, "Wallpapers")
	os.MkdirAll(wallpapersDir, 0755)

	var downloadURL, filename string

	switch source {
	case "osu":
		out, err := exec.Command("curl", "-s", "https://osu.ppy.sh/api/v2/seasonal-backgrounds").Output()
		if err != nil {
			return
		}
		var resp struct {
			Backgrounds []struct {
				URL string `json:"url"`
			} `json:"backgrounds"`
		}
		if json.Unmarshal(out, &resp) != nil || len(resp.Backgrounds) == 0 {
			return
		}
		idx := rand.Intn(len(resp.Backgrounds))
		link := resp.Backgrounds[idx].URL
		ext := filepath.Ext(link)
		if ext == "" {
			ext = ".jpg"
		}
		filename = fmt.Sprintf("random_wallpaper%s", ext)
		downloadURL = link
	default: // konachan
		page := rand.Intn(1000) + 1
		url := fmt.Sprintf("https://konachan.net/post.json?tags=rating%%3Asafe&limit=1&page=%d", page)
		out, err := exec.Command("curl", "-s", url).Output()
		if err != nil {
			return
		}
		var posts []struct {
			FileURL string `json:"file_url"`
		}
		if json.Unmarshal(out, &posts) != nil || len(posts) == 0 {
			return
		}
		link := posts[0].FileURL
		ext := filepath.Ext(link)
		if ext == "" {
			ext = ".jpg"
		}
		filename = fmt.Sprintf("random_wallpaper%s", ext)
		downloadURL = link
	}

	downloadPath := filepath.Join(wallpapersDir, filename)

	// Check if same as current wallpaper
	configDir := a.configDir()
	configFile := filepath.Join(configDir, "snry-shell", "config.json")
	if data, err := os.ReadFile(configFile); err == nil {
		var cfg map[string]any
		if json.Unmarshal(data, &cfg) == nil {
			if bg, ok := cfg["background"].(map[string]any); ok {
				if wp, ok := bg["wallpaperPath"].(string); ok && wp == downloadPath {
					downloadPath = filepath.Join(wallpapersDir, strings.TrimSuffix(filename, filepath.Ext(filename))+"-1"+filepath.Ext(filename))
				}
			}
		}
	}

	// Download
	if err := exec.Command("curl", "-s", "-o", downloadPath, downloadURL).Run(); err != nil {
		return
	}

	// Apply wallpaper via daemon command
	a.stateCh <- stateEvent{kind: "command", cmd: "apply-wallpaper", arg: downloadPath}
	a.socketServer.Emitter().Emit(map[string]any{
		"event": "random_wallpaper_ready",
		"data":  map[string]any{"path": downloadPath},
	})
}

func (a *App) handleGenerateThumbnails(sizeName, mode, target string) {
	sizeMap := map[string]int{"normal": 128, "large": 256, "x-large": 512, "xx-large": 1024}
	thumbnailSize := 128
	if s, ok := sizeMap[sizeName]; ok {
		thumbnailSize = s
	}

	home := os.Getenv("HOME")
	cacheDir := filepath.Join(home, ".cache", "thumbnails", sizeName)
	os.MkdirAll(cacheDir, 0755)

	generate := func(src string) {
		absPath, err := filepath.Abs(src)
		if err != nil {
			return
		}

		// Skip GIFs and videos
		lower := strings.ToLower(absPath)
		for _, ext := range []string{".gif", ".mp4", ".webm", ".mkv", ".avi", ".mov"} {
			if strings.HasSuffix(lower, ext) {
				return
			}
		}

		// Generate URI and MD5 hash
		uri := "file://" + absPath
		hash := md5.Sum([]byte(uri))
		outPath := filepath.Join(cacheDir, fmt.Sprintf("%x.png", hash))

		if _, err := os.Stat(outPath); err == nil {
			return
		} // already exists

		exec.Command("magick", absPath, "-resize",
			fmt.Sprintf("%dx%d", thumbnailSize, thumbnailSize), outPath).Run()
	}

	switch mode {
	case "file":
		generate(target)
	case "dir":
		entries, err := os.ReadDir(target)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				generate(filepath.Join(target, entry.Name()))
			}
		}
	}

	a.socketServer.Emitter().Emit(map[string]any{
		"event": "thumbnail_generated",
		"data":  map[string]any{"directory": target, "size": sizeName},
	})
}

func (a *App) handleCapslockCheck() {
	data, err := a.hyprlandSvc.GetDevices()
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	capsActive := false
	inMainKB := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "main: yes") {
			inMainKB = true
		}
		if inMainKB && strings.Contains(trimmed, "capsLock") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 && parts[len(parts)-1] == "yes" {
				capsActive = true
			}
			break
		}
	}
	var msg string
	if capsActive {
		msg = "Caps Lock active"
	}
	a.socketServer.Emitter().Emit(map[string]any{
		"event": "capslock_result",
		"data":  map[string]any{"active": capsActive, "message": msg},
	})
}

func (a *App) handleBatteryStatus() {
	entries, err := os.ReadDir("/sys/class/power_supply")
	if err != nil {
		return
	}
	var result strings.Builder
	for _, entry := range entries {
		if !strings.Contains(entry.Name(), "BAT") {
			continue
		}
		basePath := filepath.Join("/sys/class/power_supply", entry.Name())
		statusData, _ := os.ReadFile(filepath.Join(basePath, "status"))
		status := strings.TrimSpace(string(statusData))
		charging := status == "Charging"

		capData, _ := os.ReadFile(filepath.Join(basePath, "capacity"))
		capacity := strings.TrimSpace(string(capData))

		if capacity != "" {
			if charging {
				result.WriteString("(+) ")
			}
			result.WriteString(capacity + "%")
			if !charging {
				result.WriteString(" remaining")
			}
		}
		break
	}
	a.socketServer.Emitter().Emit(map[string]any{
		"event": "battery_status_result",
		"data":  map[string]any{"status": result.String()},
	})
}

func (a *App) handleRestoreVideoWallpaper() {
	cfg := wallpaper.DefaultConfig()
	shellCfg, err := wallpaper.LoadShellConfig(cfg.ShellConfigFile)
	if err != nil {
		return
	}
	videoPath := shellCfg.Background.WallpaperPath
	if videoPath == "" {
		return
	}
	if !wallpaper.IsVideoFile(videoPath) {
		return
	}
	if err := wallpaper.StartVideoWallpaper(videoPath); err != nil {
		log.Printf("[wallpaper] restore video wallpaper: %v", err)
	}
}

func (a *App) handleRecord(args []string) {
	if _, err := exec.LookPath("wf-recorder"); err != nil {
		return
	}

	if err := exec.Command("pgrep", "-x", "wf-recorder").Run(); err == nil {
		exec.Command("pkill", "wf-recorder").Run()
		exec.Command("notify-send", "Recording Stopped", "Stopped", "-a", "Recorder").Run()
		return
	}

	sound := false
	fullscreen := false
	region := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--sound":
			sound = true
		case "--fullscreen":
			fullscreen = true
		case "--region":
			if i+1 < len(args) {
				region = args[i+1]
				i++
			}
		}
	}

	saveDir := getRecordSaveDir()
	os.MkdirAll(saveDir, 0755)

	filename := fmt.Sprintf("%s/recording_%s.mp4", saveDir, time.Now().Format("2006-01-02_15.04.05"))

	cmdArgs := []string{"--pixel-format", "yuv420p", "-f", filename, "-t"}

	if fullscreen {
		if a.hyprlandSvc != nil {
			if monitor, err := a.hyprlandSvc.GetMonitors(); err == nil {
				var monitors []map[string]any
				if json.Unmarshal(monitor, &monitors) == nil {
					for _, m := range monitors {
						if focused, ok := m["focused"].(bool); ok && focused {
							if name, ok := m["name"].(string); ok {
								cmdArgs = append([]string{"-o", name}, cmdArgs...)
								break
							}
						}
					}
				}
			}
		}
	} else {
		if region == "" {
			out, err := exec.Command("slurp").Output()
			if err != nil {
				return
			}
			region = strings.TrimSpace(string(out))
		}
		cmdArgs = append(cmdArgs, "--geometry", region)
	}

	if sound {
		out, err := exec.Command("pactl", "list", "sources", "short").Output()
		if err == nil {
			for line := range strings.SplitSeq(string(out), "\n") {
				if strings.Contains(line, "monitor") {
					fields := strings.Fields(line)
					if len(fields) > 0 {
						cmdArgs = append(cmdArgs, "--audio", fields[0])
						break
					}
				}
			}
		}
	}

	exec.Command("notify-send", "Starting recording", filepath.Base(filename), "-a", "Recorder").Run()
	exec.Command("wf-recorder", cmdArgs...).Run()
}

func getRecordSaveDir() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	configFile := filepath.Join(configDir, "snry-shell", "config.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), "Videos")
	}
	var cfg map[string]any
	if json.Unmarshal(data, &cfg) != nil {
		return filepath.Join(os.Getenv("HOME"), "Videos")
	}
	sr, ok := cfg["screenRecord"].(map[string]any)
	if !ok {
		return filepath.Join(os.Getenv("HOME"), "Videos")
	}
	if path, ok := sr["savePath"].(string); ok && path != "" {
		return path
	}
	return filepath.Join(os.Getenv("HOME"), "Videos")
}

func (a *App) handleKeyringCheck() {
	cmd := exec.Command("secret-tool", "lookup", "service", "snry-shell")
	if cmd.Run() == nil {
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "keyring_status",
			"data":  map[string]any{"unlocked": true},
		})
		return
	}
	cmd = exec.Command("busctl", "--user", "get-property", "org.freedesktop.secrets",
		"/org/freedesktop/secrets/collection/login", "org.freedesktop.Secret.Collection", "Locked")
	out, err := cmd.Output()
	if err == nil && strings.Contains(string(out), "false") {
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "keyring_status",
			"data":  map[string]any{"unlocked": true},
		})
	} else {
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "keyring_status",
			"data":  map[string]any{"unlocked": false},
		})
	}
}

func (a *App) handleKeyringLookup() {
	cmd := exec.Command("secret-tool", "lookup", "application", "snry-shell")
	out, err := cmd.Output()
	if err != nil {
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "keyring_lookup_result",
			"data":  map[string]any{"status": "not_found"},
		})
		return
	}
	a.socketServer.Emitter().Emit(map[string]any{
		"event": "keyring_lookup_result",
		"data":  map[string]any{"status": "ok", "data": strings.TrimSpace(string(out))},
	})
}

func (a *App) handleKeyringUnlock(password string) {
	cmd := exec.Command("bash", "-c", fmt.Sprintf("echo '%s' | gnome-keyring-daemon --daemonize --login", password))
	_, err := cmd.Output()
	if err == nil {
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "keyring_unlock_result",
			"data":  map[string]any{"success": true},
		})
	} else {
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "keyring_unlock_result",
			"data":  map[string]any{"success": false, "error": err.Error()},
		})
	}
}

func (a *App) handleNvimApplyColors() {
	nvimFile := filepath.Join(a.genDir(), "nvim_colors.json")

	colors, err := wallpaper.LoadColorMapFromJSON(a.colorsJSONPath())
	if err != nil {
		log.Printf("[wallpaper] nvim colors: no colors.json: %v", err)
		return
	}

	// Convert to map[string]any for JSON output
	colorsAny := make(map[string]any, len(colors))
	for k, v := range colors {
		colorsAny[k] = v
	}

	jsonOut, err := json.MarshalIndent(colorsAny, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(nvimFile), 0755)
	os.WriteFile(nvimFile, jsonOut, 0644)

	proc.ForEachComm(func(comm string, pid int) bool {
		if comm == "nvim" {
			syscall.Kill(pid, syscall.SIGUSR1)
		}
		return true
	})
}

func (a *App) handleApplyVscodeColor() {
	colorFile := a.colorFilePath()
	newColor, err := os.ReadFile(colorFile)
	if err != nil {
		return
	}
	newColorStr := strings.TrimSpace(string(newColor))
	if newColorStr == "" {
		if colors, err := wallpaper.LoadColorMapFromJSON(a.colorsJSONPath()); err == nil {
			if primary, ok := colors["primary"]; ok {
				newColorStr = strings.TrimSpace(primary)
			}
		}
		if newColorStr == "" {
			return
		}
	}

	configDir := a.configDir()

	settingsPaths := []string{
		filepath.Join(configDir, "Code/User/settings.json"),
		filepath.Join(configDir, "VSCodium/User/settings.json"),
		filepath.Join(configDir, "Code - OSS/User/settings.json"),
		filepath.Join(configDir, "Code - Insiders/User/settings.json"),
		filepath.Join(configDir, "Cursor/User/settings.json"),
		filepath.Join(configDir, "Antigravity/User/settings.json"),
	}

	key := "material-code.primaryColor"
	for _, path := range settingsPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		pattern := `"material-code.primaryColor"`
		if strings.Contains(content, pattern) {
			re := regexp.MustCompile(`("material-code\.primaryColor"\s*:\s*)"[^"]*"`)
			content = re.ReplaceAllString(content, fmt.Sprintf(`${1}"%s"`, newColorStr))
		} else {
			lastBrace := strings.LastIndex(content, "}")
			if lastBrace >= 0 {
				content = content[:lastBrace] + fmt.Sprintf(",\n  \"%s\": \"%s\"\n}", key, newColorStr)
			}
		}
		os.WriteFile(path, []byte(content), 0644)
	}
}

func (a *App) handleApplyKvantumTheme() {
	configDir := a.configDir()

	colloidDir := filepath.Join(configDir, "Kvantum", "Colloid")
	materialAdwDir := filepath.Join(configDir, "Kvantum", "MaterialAdw")
	if _, err := os.Stat(colloidDir); err != nil {
		exec.Command("notify-send", "Colloid-kde theme required",
			fmt.Sprintf("The folder '%s' does not exist.", colloidDir)).Run()
		return
	}
	os.MkdirAll(materialAdwDir, 0755)

	// Detect dark/light mode
	isDark := false
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
	if err == nil && strings.Contains(string(out), "prefer-dark") {
		isDark = true
	}

	var srcConfig string
	if isDark {
		srcConfig = filepath.Join(colloidDir, "ColloidDark.kvconfig")
	} else {
		srcConfig = filepath.Join(colloidDir, "Colloid.kvconfig")
	}

	dstConfig := filepath.Join(materialAdwDir, "MaterialAdw.kvconfig")

	// Copy source config
	data, err := os.ReadFile(srcConfig)
	if err != nil {
		return
	}
	os.WriteFile(dstConfig, data, 0644)

	colorMap := make(map[string]string)
	if colors, err := wallpaper.LoadColorMapFromJSON(a.colorsJSONPath()); err == nil {
		for k, v := range colors {
			colorMap[snakeToCamel(k)] = v
		}
	}

	// Apply color mappings to kvconfig
	content := string(data)
	mappings := map[string]string{
		"window.color":                  "background",
		"base.color":                    "background",
		"alt.base.color":                "background",
		"button.color":                  "surfaceContainer",
		"light.color":                   "surfaceContainerLow",
		"mid.light.color":               "surfaceContainer",
		"dark.color":                    "surfaceContainerHighest",
		"mid.color":                     "surfaceContainerHigh",
		"highlight.color":               "primary",
		"inactive.highlight.color":      "primary",
		"text.color":                    "onBackground",
		"window.text.color":             "onBackground",
		"button.text.color":             "onBackground",
		"disabled.text.color":           "onBackground",
		"tooltip.text.color":            "onBackground",
		"highlight.text.color":          "onSurface",
		"link.color":                    "tertiary",
		"link.visited.color":            "tertiaryFixed",
		"progress.indicator.text.color": "onBackground",
		"text.normal.color":             "onBackground",
		"text.focus.color":              "onBackground",
		"text.press.color":              "onsecondarycontainer",
		"text.toggle.color":             "onsecondarycontainer",
		"text.disabled.color":           "surfaceDim",
	}
	for kvKey, scssName := range mappings {
		if hexVal, ok := colorMap[scssName]; ok {
			re := regexp.MustCompile(regexp.QuoteMeta(kvKey) + `=\s*#[0-9a-fA-F]+`)
			content = re.ReplaceAllString(content, kvKey+"="+hexVal)
		}
	}
	os.WriteFile(dstConfig, []byte(content), 0644)
}

func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

func (a *App) handleSwitchWallpaper(args []string) {
	var imgPath, mode, schemeTypeStr, color string
	var noswitch bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--image":
			// Consume all following tokens until next flag (handles paths with spaces)
			i++
			var parts []string
			for i < len(args) && !strings.HasPrefix(args[i], "--") {
				parts = append(parts, args[i])
				i++
			}
			if len(parts) > 0 {
				imgPath = strings.Join(parts, " ")
			}
			i-- // back up one so loop increments correctly
		case "--mode":
			if i+1 < len(args) {
				mode = args[i+1]
				i++
			}
		case "--type":
			if i+1 < len(args) {
				schemeTypeStr = args[i+1]
				i++
			}
		case "--color":
			if i+1 < len(args) {
				color = args[i+1]
				i++
			}
		case "--noswitch":
			noswitch = true
		default:
			// Positional argument without flag = image path
			if imgPath == "" && !strings.HasPrefix(args[i], "--") {
				var parts []string
				parts = append(parts, args[i])
				for j := i + 1; j < len(args) && !strings.HasPrefix(args[j], "--"); j++ {
					parts = append(parts, args[j])
				}
				imgPath = strings.Join(parts, " ")
				i += len(parts) - 1
			}
		}
	}

	// Handle --noswitch: use current wallpaper path from config
	if noswitch && imgPath == "" {
		shellCfg, err := wallpaper.LoadShellConfig(wallpaper.DefaultConfig().ShellConfigFile)
		if err == nil {
			imgPath = shellCfg.Background.WallpaperPath
		}
	}

	if imgPath == "" && color == "" && !noswitch {
		// No image specified — open a file picker dialog
		picturesDir := os.Getenv("XDG_PICTURES_DIR")
		if picturesDir == "" {
			homeDir, _ := os.UserHomeDir()
			picturesDir = filepath.Join(homeDir, "Pictures")
		}
		wallpaperDir := filepath.Join(picturesDir, "Wallpapers")
		if _, err := os.Stat(wallpaperDir); err != nil {
			wallpaperDir = picturesDir
		}

		pickerCmd := exec.Command("kdialog", "--getopenfilename", wallpaperDir,
			"--title", "Choose wallpaper",
			"--filter", "*.jpg *.jpeg *.png *.webp *.avif *.bmp *.svg *.mp4 *.webm *.mkv")
		out, err := pickerCmd.Output()
		if err != nil {
			return // User cancelled or error
		}
		imgPath = strings.TrimSpace(string(out))
		if imgPath == "" {
			return
		}
		// Strip file:// prefix if present
		imgPath = strings.TrimPrefix(imgPath, "file://")
	}

	schemeType := wallpaper.ParseSchemeType(schemeTypeStr)

	// If no scheme type specified, read from config
	if schemeTypeStr == "" {
		shellCfg, err := wallpaper.LoadShellConfig(wallpaper.DefaultConfig().ShellConfigFile)
		if err == nil && shellCfg.Appearance.Palette.Type != "" {
			schemeType = wallpaper.ParseSchemeType(shellCfg.Appearance.Palette.Type)
		}
	}
	if schemeType == "" {
		schemeType = wallpaper.SchemeAuto
	}

	// Handle accent color from config if not explicitly set
	if color == "" {
		shellCfg, err := wallpaper.LoadShellConfig(wallpaper.DefaultConfig().ShellConfigFile)
		if err == nil && shellCfg.Appearance.Palette.AccentColor != "" {
			color = shellCfg.Appearance.Palette.AccentColor
		}
	}

	// When using an image (not noswitch), clear accent color
	if imgPath != "" && !noswitch {
		wallpaper.ClearAccentColor(wallpaper.DefaultConfig().ShellConfigFile)
		color = ""
	}

	// Handle color=clear explicitly
	if color == "clear" {
		wallpaper.ClearAccentColor(wallpaper.DefaultConfig().ShellConfigFile)
		color = ""
	}

	if mode == "" {
		if wallpaper.IsDarkMode() {
			mode = "dark"
		} else {
			mode = "light"
		}
	}

	cfg := wallpaper.DefaultConfig()
	cfg.RepoRoot = a.repoRoot()

	if err := wallpaper.FullWallpaperSwitch(cfg, imgPath, mode, schemeType, color); err != nil {
		log.Printf("[wallpaper] switch failed: %v", err)
	}
}

func (a *App) handleApplyTerminalColors() {
	configDir := a.configDir()

	wpCfg := wallpaper.DefaultConfig()
	wpCfg.ConfigHome = configDir
	wpCfg.RepoRoot = a.repoRoot()

	genDir := wpCfg.GenDir()

	// Read colors.json (populated by matugen + MaterialThemeLoader)
	colors, err := wallpaper.LoadColorMapFromJSON(a.colorsJSONPath())
	if err != nil {
		log.Printf("[wallpaper] no colors.json: %v", err)
		return
	}

	// Load shell config for terminal settings
	termCfg := wallpaper.DefaultTerminalConfig()
	shellCfg, err := wallpaper.LoadShellConfig(wpCfg.ShellConfigFile)
	if err == nil {
		termCfg.EnableTerminal = shellCfg.Appearance.WallpaperTheming.EnableTerminal
		termCfg.ForceDarkMode = shellCfg.Appearance.WallpaperTheming.TerminalGenerationProps.ForceDarkMode
		termCfg.Harmony = shellCfg.Appearance.WallpaperTheming.TerminalGenerationProps.Harmony
		termCfg.HarmonizeThreshold = shellCfg.Appearance.WallpaperTheming.TerminalGenerationProps.HarmonizeThreshold
		termCfg.TermFgBoost = shellCfg.Appearance.WallpaperTheming.TerminalGenerationProps.TermFgBoost
	}

	if !termCfg.EnableTerminal {
		return
	}

	// Determine dark mode
	isDark := true
	if termCfg.ForceDarkMode {
		isDark = true
	} else {
		isDark = wallpaper.IsDarkMode()
	}

	// Load terminal base scheme
	schemePath := wallpaper.FindTerminalScheme(wpCfg)
	scheme, err := wallpaper.LoadTerminalScheme(schemePath)
	if err != nil {
		log.Printf("[wallpaper] terminal scheme: %v", err)
		return
	}

	// Generate harmonized terminal colors
	termColors := wallpaper.GenerateTerminalColors(colors, scheme, isDark,
		termCfg.Harmony, termCfg.HarmonizeThreshold, termCfg.TermFgBoost)

	// Apply terminal theme files
	if err := wallpaper.ApplyTerminalTheme(colors, termColors, isDark, genDir); err != nil {
		log.Printf("[wallpaper] apply terminal theme: %v", err)
	}

	// Also apply nvim and vscode colors
	a.handleNvimApplyColors()
	a.handleApplyVscodeColor()
}

func (a *App) handleHyprconfigEdit(args []string) {
	filePath := ""
	var setArgs [][2]string
	var resetKeys []string

	configDir := a.configDir()
	// Prefer hyprland.lua (new format), fall back to hyprland.conf (legacy)
	defaultLua := filepath.Join(configDir, "hypr", "hyprland.lua")
	defaultConf := filepath.Join(configDir, "hypr", "hyprland.conf")
	defaultFile := defaultConf
	if _, err := os.Stat(defaultLua); err == nil {
		defaultFile = defaultLua
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file":
			if i+1 < len(args) {
				filePath = args[i+1]
				i++
			}
		case "--set":
			if i+2 < len(args) {
				key, val := args[i+1], args[i+2]
				if val == "[[EMPTY]]" {
					resetKeys = append(resetKeys, key)
				} else {
					setArgs = append(setArgs, [2]string{key, val})
				}
				i += 2
			}
		case "--reset":
			if i+1 < len(args) {
				resetKeys = append(resetKeys, args[i+1])
				i++
			}
		}
	}

	if filePath == "" {
		filePath = defaultFile
	}
	filePath = os.ExpandEnv(filePath)

	if len(setArgs) == 0 && len(resetKeys) == 0 {
		return
	}

	// Use IPC for runtime set/reset
	for _, kv := range setArgs {
		a.hyprlandSvc.SetOption(kv[0], kv[1])
	}
	for _, key := range resetKeys {
		a.hyprlandSvc.ResetOption(key)
	}

	// Also update the config file
	if strings.HasSuffix(filePath, ".lua") {
		a.editLuaConfig(filePath, setArgs, resetKeys)
	} else {
		a.editConfConfig(filePath, setArgs, resetKeys)
	}
}

func (a *App) editConfConfig(filePath string, setArgs [][2]string, resetKeys []string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if len(setArgs) > 0 {
			var lines []string
			for _, kv := range setArgs {
				lines = append(lines, fmt.Sprintf("%s = %s", kv[0], kv[1]))
			}
			os.MkdirAll(filepath.Dir(filePath), 0755)
			os.WriteFile(filePath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
		}
		return
	}

	lines := strings.Split(string(data), "\n")
	patterns := make(map[string]*regexp.Regexp)
	for _, kv := range setArgs {
		patterns[kv[0]] = regexp.MustCompile("(?m)^\\s*" + regexp.QuoteMeta(kv[0]) + "\\s*=")
	}
	for _, key := range resetKeys {
		patterns[key] = regexp.MustCompile("(?m)^\\s*" + regexp.QuoteMeta(key) + "\\s*=")
	}

	var newLines []string
	found := make(map[string]bool)

	for _, line := range lines {
		matched := false
		for _, key := range resetKeys {
			if patterns[key].MatchString(line) {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		for _, kv := range setArgs {
			if patterns[kv[0]].MatchString(line) {
				newLines = append(newLines, fmt.Sprintf("%s = %s", kv[0], kv[1]))
				found[kv[0]] = true
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		newLines = append(newLines, line)
	}

	for _, kv := range setArgs {
		if !found[kv[0]] {
			if len(newLines) > 0 && !strings.HasSuffix(newLines[len(newLines)-1], "\n") {
				newLines[len(newLines)-1] += "\n"
			}
			newLines = append(newLines, fmt.Sprintf("%s = %s", kv[0], kv[1]))
		}
	}

	os.WriteFile(filePath, []byte(strings.Join(newLines, "\n")), 0644)
}

func (a *App) editLuaConfig(filePath string, setArgs [][2]string, resetKeys []string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if len(setArgs) > 0 {
			var lines []string
			for _, kv := range setArgs {
				lines = append(lines, hyprland.BuildConfigLua(kv[0], kv[1]))
			}
			os.MkdirAll(filepath.Dir(filePath), 0755)
			os.WriteFile(filePath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
		}
		return
	}

	content := string(data)

	// For each set arg, find and replace the value in Lua table syntax.
	// Keys use dot notation: "general.border_size" → look for "border_size = VALUE"
	for _, kv := range setArgs {
		parts := strings.Split(kv[0], ".")
		leafKey := parts[len(parts)-1]
		// Match Lua table key: key = VALUE (with optional trailing comma)
		pattern := regexp.MustCompile("(?m)(\\b" + regexp.QuoteMeta(leafKey) + "\\s*=\\s*)[^,}\n]+")
		if pattern.MatchString(content) {
			content = pattern.ReplaceAllStringFunc(content, func(match string) string {
				return parts[0] + " = " + kv[1]
			})
			content = pattern.ReplaceAllString(content, "${1}"+kv[1])
		} else {
			// Key not found in file, append as hl.config call
			content = strings.TrimRight(content, "\n") + "\n" + hyprland.BuildConfigLua(kv[0], kv[1]) + "\n"
		}
	}

	// For each reset key, remove the line containing it
	for _, key := range resetKeys {
		parts := strings.Split(key, ".")
		leafKey := parts[len(parts)-1]
		pattern := regexp.MustCompile("(?m)^.*\\b" + regexp.QuoteMeta(leafKey) + "\\s*=.*$\n?")
		content = pattern.ReplaceAllString(content, "")
	}

	os.WriteFile(filePath, []byte(content), 0644)
}

func (a *App) handleFindRegions(args []string) {
	params := imgPkg.DefaultFindRegionsParams("")
	params.Hyprctl = true

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--image", "-i":
			if i+1 < len(args) {
				params.ImagePath = args[i+1]
				i++
			}
		case "--hyprctl":
			params.Hyprctl = true
		case "--single":
			params.Single = true
		case "--quality":
			params.Quality = true
		case "--min-width":
			if i+1 < len(args) {
				params.MinWidth, _ = strconv.Atoi(args[i+1])
				i++
			}
		case "--min-height":
			if i+1 < len(args) {
				params.MinHeight, _ = strconv.Atoi(args[i+1])
				i++
			}
		case "--max-width":
			if i+1 < len(args) {
				v, _ := strconv.Atoi(args[i+1])
				params.MaxWidth = &v
				i++
			}
		case "--max-height":
			if i+1 < len(args) {
				v, _ := strconv.Atoi(args[i+1])
				params.MaxHeight = &v
				i++
			}
		case "--k":
			if i+1 < len(args) {
				params.K, _ = strconv.Atoi(args[i+1])
				i++
			}
		case "--min-size":
			if i+1 < len(args) {
				params.MinSize, _ = strconv.Atoi(args[i+1])
				i++
			}
		case "--sigma":
			if i+1 < len(args) {
				params.Sigma, _ = strconv.ParseFloat(args[i+1], 64)
				i++
			}
		case "--resize-factor":
			if i+1 < len(args) {
				params.ResizeFactor, _ = strconv.ParseFloat(args[i+1], 64)
				i++
			}
		case "--debug-output", "-do":
			if i+1 < len(args) {
				params.DebugOutput = args[i+1]
				i++
			}
		}
	}

	if params.ImagePath == "" {
		log.Printf("[image] find-regions: no image path specified")
		return
	}

	result, err := imgPkg.FindRegionsJSON(params)
	if err != nil {
		log.Printf("[image] find-regions error: %v", err)
		return
	}

	a.socketServer.Emitter().Emit(map[string]any{
		"event": "find-regions",
		"data":  map[string]any{"result": result, "imagePath": params.ImagePath},
	})
}

func (a *App) handleLeastBusyRegion(args []string) {
	params := imgPkg.DefaultLeastBusyRegionParams("")

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--image", "-i":
			// positional arg: consume tokens until next flag
			i++
			var parts []string
			for i < len(args) && !strings.HasPrefix(args[i], "-") {
				parts = append(parts, args[i])
				i++
			}
			if len(parts) > 0 {
				params.ImagePath = strings.Join(parts, " ")
			}
			i--
		case "--width":
			if i+1 < len(args) {
				params.RegionWidth, _ = strconv.Atoi(args[i+1])
				i++
			}
		case "--height":
			if i+1 < len(args) {
				params.RegionHeight, _ = strconv.Atoi(args[i+1])
				i++
			}
		case "--screen-width":
			if i+1 < len(args) {
				params.ScreenWidth, _ = strconv.Atoi(args[i+1])
				i++
			}
		case "--screen-height":
			if i+1 < len(args) {
				params.ScreenHeight, _ = strconv.Atoi(args[i+1])
				i++
			}
		case "--stride":
			if i+1 < len(args) {
				params.Stride, _ = strconv.Atoi(args[i+1])
				i++
			}
		case "--screen-mode":
			if i+1 < len(args) {
				params.ScreenMode = args[i+1]
				i++
			}
		case "--horizontal-padding", "-hp":
			if i+1 < len(args) {
				params.HorizontalPadding, _ = strconv.Atoi(args[i+1])
				i++
			}
		case "--vertical-padding", "-vp":
			if i+1 < len(args) {
				params.VerticalPadding, _ = strconv.Atoi(args[i+1])
				i++
			}
		case "--busiest":
			params.Busiest = true
		case "--visual-output", "-v":
			params.VisualOutput = true
		}
	}

	if params.ImagePath == "" {
		log.Printf("[image] least-busy-region: no image path specified")
		return
	}

	result, err := imgPkg.FindLeastBusyRegionJSON(params)
	if err != nil {
		log.Printf("[image] least-busy-region error: %v", err)
		return
	}

	a.socketServer.Emitter().Emit(map[string]any{
		"event": "least-busy-region",
		"data":  map[string]any{"result": result, "imagePath": params.ImagePath},
	})
}

func (a *App) handleTextColor(args []string) {
	var imagePath string
	var cropX, cropY, cropW, cropH int
	hasCrop := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--image", "-i":
			if i+1 < len(args) {
				imagePath = args[i+1]
				i++
			}
		case "--crop-x":
			if i+1 < len(args) {
				cropX, _ = strconv.Atoi(args[i+1])
				hasCrop = true
				i++
			}
		case "--crop-y":
			if i+1 < len(args) {
				cropY, _ = strconv.Atoi(args[i+1])
				hasCrop = true
				i++
			}
		case "--crop-w", "--crop-width":
			if i+1 < len(args) {
				cropW, _ = strconv.Atoi(args[i+1])
				hasCrop = true
				i++
			}
		case "--crop-h", "--crop-height":
			if i+1 < len(args) {
				cropH, _ = strconv.Atoi(args[i+1])
				hasCrop = true
				i++
			}
		}
	}

	var result *imgPkg.TextColorResult
	var err error

	if imagePath != "" {
		if hasCrop && cropW > 0 && cropH > 0 {
			result, err = imgPkg.DetectTextColorFromPathCropped(imagePath, cropX, cropY, cropW, cropH)
		} else {
			result, err = imgPkg.DetectTextColorFromPath(imagePath)
		}
	} else {
		result, err = imgPkg.DetectTextColorFromReader(os.Stdin)
	}

	if err != nil {
		log.Printf("[image] text-color error: %v", err)
		return
	}

	jsonResult, _ := json.Marshal(result)
	a.socketServer.Emitter().Emit(map[string]any{
		"event": "text-color",
		"data":  map[string]any{"result": string(jsonResult)},
	})
}
