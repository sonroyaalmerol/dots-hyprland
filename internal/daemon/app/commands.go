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
		if a.lockscreenSvc != nil {
			if !a.lockscreenSvc.IsLocked() {
				a.lockscreenSvc.Lock()
				// Dispatch to QS via IPC for reliable lock screen display
				go a.dispatchQsLock()
			}
		} else if a.idleSvc != nil {
			a.idleSvc.Lock()
		}
	case "lock-startup":
		if a.lockscreenSvc != nil {
			a.lockscreenSvc.LockWithAutoUnlock()
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
	case "weather-refresh":
		if a.weatherSvc != nil {
			go a.weatherSvc.RefreshNow(context.Background())
		}
	case "cliphist-list":
		if a.cliphistSvc != nil {
			go a.cliphistSvc.EmitList(context.Background())
		}
	case "cliphist-delete":
		if a.cliphistSvc != nil {
			entry := strings.TrimPrefix(line, "cliphist-delete ")
			go a.cliphistSvc.DeleteEntry(context.Background(), entry)
		}
	case "cliphist-wipe":
		if a.cliphistSvc != nil {
			go a.cliphistSvc.Wipe(context.Background())
		}
	case "autoscale":
		go a.handleAutoscale()
	case "checkdeps":
		go a.handleCheckdeps()
	case "diagnose":
		go a.handleDiagnose()
	case "workspace-action":
		a.handleWorkspaceAction(fields[1:])
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
	case "osk-dismiss":
		a.stateCh <- stateEvent{kind: "command", cmd: "osk-dismiss"}
	case "osk-undismiss":
		a.stateCh <- stateEvent{kind: "command", cmd: "osk-undismiss"}
	case "osk-toggle":
		a.stateCh <- stateEvent{kind: "command", cmd: "osk-toggle"}
	case "osk-show":
		a.stateCh <- stateEvent{kind: "command", cmd: "osk-show"}
	case "osk-hide":
		a.stateCh <- stateEvent{kind: "command", cmd: "osk-hide"}
	case "osk-pin":
		a.stateCh <- stateEvent{kind: "command", cmd: "osk-pin"}
	case "osk-unpin":
		a.stateCh <- stateEvent{kind: "command", cmd: "osk-unpin"}

	// New daemon service commands
	case "reload-keybinds":
		if a.hyprKeybindsSvc != nil {
			a.hyprKeybindsSvc.Reload()
		}
	case "easyeffects-toggle":
		if a.easyEffectsSvc != nil {
			if a.easyEffectsSvc.IsActive() {
				a.easyEffectsSvc.Disable()
			} else {
				a.easyEffectsSvc.Enable()
			}
		}
	case "easyeffects-enable":
		if a.easyEffectsSvc != nil {
			a.easyEffectsSvc.Enable()
		}
	case "easyeffects-disable":
		if a.easyEffectsSvc != nil {
			a.easyEffectsSvc.Disable()
		}
	case "hyprsunset-gamma":
		if a.hyprsunsetSvc != nil && len(fields) >= 2 {
			if gamma, err := strconv.Atoi(fields[1]); err == nil {
				a.hyprsunsetSvc.SetGamma(gamma)
			}
		}
	case "hyprsunset-enable":
		if a.hyprsunsetSvc != nil {
			a.hyprsunsetSvc.EnableTemperature()
		}
	case "hyprsunset-disable":
		if a.hyprsunsetSvc != nil {
			a.hyprsunsetSvc.DisableTemperature()
		}
	case "hyprsunset-toggle":
		if a.hyprsunsetSvc != nil {
			active := true
			if len(fields) >= 2 {
				active = fields[1] != "false" && fields[1] != "0"
			}
			a.hyprsunsetSvc.ToggleTemperature(active)
		}
	case "wifi-enable":
		if a.networkSvc != nil {
			a.networkSvc.EnableWifi(context.Background(), true)
		}
	case "wifi-disable":
		if a.networkSvc != nil {
			a.networkSvc.EnableWifi(context.Background(), false)
		}
	case "wifi-toggle":
		if a.networkSvc != nil {
			a.networkSvc.ToggleWifi(context.Background())
		}
	case "wifi-rescan":
		if a.networkSvc != nil {
			go a.networkSvc.RescanWifi(context.Background())
		}
	case "wifi-connect":
		if a.networkSvc != nil && len(fields) >= 2 {
			go func() {
				err := a.networkSvc.ConnectWifi(context.Background(), strings.Join(fields[1:], " "))
				if err != nil {
					log.Printf("[app] wifi-connect: %v", err)
					a.socketServer.Emitter().Emit(map[string]any{
						"event": "network_connect_result",
						"data": map[string]any{
							"success":        false,
							"askingPassword": true,
							"ssid":           strings.Join(fields[1:], " "),
						},
					})
				}
			}()
		}
	case "wifi-disconnect":
		if a.networkSvc != nil && len(fields) >= 2 {
			go a.networkSvc.DisconnectWifi(context.Background(), strings.Join(fields[1:], " "))
		}
	case "wifi-change-password":
		if a.networkSvc != nil && len(fields) >= 3 {
			go a.networkSvc.ChangePassword(context.Background(), fields[1], fields[2])
		}
	case "brightness-set":
		if a.brightnessSvc != nil && len(fields) >= 3 {
			screen := fields[1]
			value, err := strconv.ParseFloat(fields[2], 64)
			if err == nil {
				a.brightnessSvc.SetBrightness(screen, value)
			}
		}
	case "brightness-increment":
		if a.brightnessSvc != nil && len(fields) >= 3 {
			screen := fields[1]
			delta, err := strconv.ParseFloat(fields[2], 64)
			if err == nil {
				a.brightnessSvc.IncrementBrightness(screen, delta)
			}
		}
	case "brightness-get":
		if a.brightnessSvc != nil && len(fields) >= 2 {
			value := a.brightnessSvc.GetBrightness(fields[1])
			a.socketServer.Emitter().Emit(map[string]any{
				"event": "brightness_value",
				"data": map[string]any{
					"screen":     fields[1],
					"brightness": value,
				},
			})
		}
	case "warp-connect":
		if a.warpSvc != nil {
			go a.warpSvc.Connect()
		}
	case "warp-disconnect":
		if a.warpSvc != nil {
			go a.warpSvc.Disconnect()
		}
	case "warp-toggle":
		if a.warpSvc != nil {
			go a.warpSvc.Toggle()
		}
	case "warp-register":
		if a.warpSvc != nil {
			go a.warpSvc.Register()
		}
	case "gamemode-enable":
		if a.gamemodeSvc != nil {
			go a.gamemodeSvc.Enable()
		}
	case "gamemode-disable":
		if a.gamemodeSvc != nil {
			go a.gamemodeSvc.Disable()
		}
	case "gamemode-toggle":
		if a.gamemodeSvc != nil {
			go a.gamemodeSvc.Toggle()
		}
	case "conflict-check":
		if a.conflictSvc != nil {
			go a.handleConflictCheck()
		}
	case "fprintd-check":
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
	case "fps-set":
		if len(fields) >= 2 {
			fpsValue := fields[1]
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
	case "hyprconfig-get":
		if len(fields) >= 2 {
			key := strings.Join(fields[1:], " ")
			out, err := exec.Command("hyprctl", "getoption", "-j", key).Output()
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
		}
	case "hyprconfig-set":
		if len(fields) >= 3 {
			key := fields[1]
			value := strings.Join(fields[2:], " ")
			exec.Command("hyprctl", "keyword", key, value).Run()
		}
	case "hyprconfig-reset":
		if len(fields) >= 2 {
			key := strings.Join(fields[1:], " ")
			exec.Command("hyprctl", "keyword", key, "undef").Run()
		}
	case "record":
		go a.handleRecord(fields[1:])
	case "recognize-music":
		go a.handleRecognizeMusic(fields[1:])
	case "keyring-check":
		go a.handleKeyringCheck()
	case "keyring-lookup":
		go a.handleKeyringLookup()
	case "keyring-unlock":
		if len(fields) >= 2 {
			go a.handleKeyringUnlock(fields[1])
		}
	case "capslock-check":
		if a.hyprlandSvc != nil {
			go a.handleCapslockCheck()
		}
	case "battery-status":
		go a.handleBatteryStatus()
	case "restore-video-wallpaper":
		go a.handleRestoreVideoWallpaper()
	case "nvim-apply-colors":
		go a.handleNvimApplyColors()
	case "apply-vscode-color":
		go a.handleApplyVscodeColor()
	case "apply-kvantum-theme":
		go a.handleApplyKvantumTheme()
	case "apply-terminal-colors":
		go a.handleApplyTerminalColors()
	case "hyprconfig-edit":
		if len(fields) < 3 {
			return
		}
		go a.handleHyprconfigEdit(fields[1:])
	}
}

// dispatchQsLock sends a lock command to Quickshell via IPC.
// This is more reliable than the socket-based DaemonSocket for triggering
// the QML lock screen.
func (a *App) dispatchQsLock() {
	binary := a.cfg.QuickshellCfg.Binary
	configDir := a.cfg.QuickshellCfg.ConfigDir
	if err := exec.Command(binary, "-c", configDir, "ipc", "call", "lock", "activate").Run(); err != nil {
		log.Printf("[app] qs ipc lock failed: %v", err)
	}
}

func (a *App) handleWorkspaceAction(args []string) {
	if len(args) < 2 || a.hyprlandSvc == nil {
		return
	}
	dispatcher := args[0]
	target := args[1]

	if strings.ContainsAny(target, "+-") {
		if err := a.hyprlandSvc.Dispatch(dispatcher, target); err != nil {
			log.Printf("[app] workspace-action dispatch: %v", err)
		}
		return
	}

	if id, err := strconv.Atoi(target); err == nil {
		currID, err := a.hyprlandSvc.ActiveWorkspaceID()
		if err != nil {
			log.Printf("[app] workspace-action get active: %v", err)
			return
		}
		targetWS := ((currID-1)/10)*10 + id
		if err := a.hyprlandSvc.Dispatch(dispatcher, strconv.Itoa(targetWS)); err != nil {
			log.Printf("[app] workspace-action dispatch: %v", err)
		}
		return
	}

	if err := a.hyprlandSvc.Dispatch(dispatcher, target); err != nil {
		log.Printf("[app] workspace-action dispatch: %v", err)
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
		if _, err := a.hyprlandSvc.QuerySocket("keyword cursor:zoom_factor 1.0"); err != nil {
			log.Printf("[app] zoom reset: %v", err)
		}
		return
	}

	data, err := a.hyprlandSvc.QuerySocket("j/getoption cursor:zoom_factor")
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

	cmd := fmt.Sprintf("keyword cursor:zoom_factor %f", newZoom)
	if _, err := a.hyprlandSvc.QuerySocket(cmd); err != nil {
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
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
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
}

func (a *App) handleCapslockCheck() {
	data, err := a.hyprlandSvc.QuerySocket("devices")
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
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	scriptPath := filepath.Join(configDir, "hypr", "custom", "scripts", "__restore_video_wallpaper.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		return
	}
	exec.Command("bash", scriptPath).Run()
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
			if monitor, err := a.hyprlandSvc.QuerySocket("j/monitors"); err == nil {
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

func (a *App) handleRecognizeMusic(args []string) {
	if _, err := exec.LookPath("songrec"); err != nil {
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "music_recognition_error",
			"data":  map[string]any{"error": "songrec not installed"},
		})
		return
	}

	interval := "2"
	source := "monitor"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-i":
			if i+1 < len(args) {
				interval = args[i+1]
				i++
			}
		case "-t":
			if i+1 < len(args) {
				i++
			}
		case "-s":
			if i+1 < len(args) {
				source = args[i+1]
				i++
			}
		}
	}

	var audioDevice string
	if source == "monitor" {
		out, err := exec.Command("pactl", "get-default-sink").Output()
		if err != nil {
			return
		}
		audioDevice = strings.TrimSpace(string(out)) + ".monitor"
	} else {
		out, err := exec.Command("pactl", "info").Output()
		if err != nil {
			return
		}
		for line := range strings.SplitSeq(string(out), "\n") {
			if after, ok := strings.CutPrefix(line, "Default Source:"); ok {
				audioDevice = strings.TrimSpace(after)
				break
			}
		}
	}

	if audioDevice == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "songrec", "listen",
		"--audio-device", audioDevice,
		"--request-interval", interval,
		"--json", "--disable-mpris")
	out, err := cmd.Output()
	if err != nil {
		return
	}

	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.Contains(line, `"matches": [`) {
			a.socketServer.Emitter().Emit(map[string]any{
				"event": "music_recognition_result",
				"data":  json.RawMessage(line),
			})
			return
		}
	}
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
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		stateDir = filepath.Join(os.Getenv("HOME"), ".local/state")
	}
	scssFile := filepath.Join(stateDir, "quickshell", "user", "generated", "material_colors.scss")
	nvimFile := filepath.Join(stateDir, "quickshell", "user", "generated", "nvim_colors.json")

	data, err := os.ReadFile(scssFile)
	if err != nil {
		return
	}

	colors := make(map[string]any)
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, ";")
		if !strings.HasPrefix(line, "$") {
			continue
		}
		line = line[1:]
		before, after, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(before)
		val := strings.TrimSpace(after)
		switch val {
		case "True":
			colors[key] = true
		case "False":
			colors[key] = false
		default:
			colors[key] = val
		}
	}

	jsonOut, err := json.MarshalIndent(colors, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(nvimFile), 0755)
	os.WriteFile(nvimFile, jsonOut, 0644)

	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		commData, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(commData)) == "nvim" {
			pid, err := strconv.Atoi(entry.Name())
			if err == nil {
				syscall.Kill(pid, syscall.SIGUSR1)
			}
		}
	}
}

func (a *App) handleApplyVscodeColor() {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		stateDir = filepath.Join(os.Getenv("HOME"), ".local/state")
	}
	colorFile := filepath.Join(stateDir, "quickshell", "user", "generated", "color.txt")
	newColor, err := os.ReadFile(colorFile)
	if err != nil {
		return
	}
	newColorStr := strings.TrimSpace(string(newColor))
	if newColorStr == "" {
		return
	}

	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}

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
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}

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

	// Read SCSS colors
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		stateDir = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	scssFile := filepath.Join(stateDir, "quickshell", "user", "generated", "material_colors.scss")
	scssData, err := os.ReadFile(scssFile)
	if err != nil {
		return
	}
	colors := parseSCSSColors(string(scssData))

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
		if hexVal, ok := colors[scssName]; ok {
			re := regexp.MustCompile(regexp.QuoteMeta(kvKey) + `=\s*#[0-9a-fA-F]+`)
			content = re.ReplaceAllString(content, kvKey+"="+hexVal)
		}
	}
	os.WriteFile(dstConfig, []byte(content), 0644)
}

func parseSCSSColors(scss string) map[string]string {
	colors := make(map[string]string)
	for line := range strings.SplitSeq(scss, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, ";")
		if !strings.HasPrefix(line, "$") {
			continue
		}
		line = line[1:]
		before, after, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(before)
		val := strings.TrimSpace(after)
		colors[key] = val
	}
	return colors
}

func (a *App) findScriptsDir(configDir string) string {
	// Try installed path first
	exe, err := os.Executable()
	if err == nil {
		shareDir := filepath.Join(filepath.Dir(exe), "..", "share", "snry-shell", "configs", "quickshell", "ii", "scripts")
		if _, err := os.Stat(shareDir); err == nil {
			return shareDir
		}
	}
	// Fallback to source config
	return filepath.Join(configDir, "quickshell", "ii", "scripts")
}

func (a *App) handleApplyTerminalColors() {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		stateDir = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}

	genDir := filepath.Join(stateDir, "quickshell", "user", "generated")
	scssFile := filepath.Join(genDir, "material_colors.scss")
	scssData, err := os.ReadFile(scssFile)
	if err != nil {
		return
	}
	colors := parseSCSSColors(string(scssData))

	// Check if terminal theming is enabled
	configFile := filepath.Join(configDir, "snry-shell", "config.json")
	enableTerminal := true // default
	if cfgData, err := os.ReadFile(configFile); err == nil {
		var cfg map[string]any
		if json.Unmarshal(cfgData, &cfg) == nil {
			if appear, ok := cfg["appearance"].(map[string]any); ok {
				if wt, ok := appear["wallpaperTheming"].(map[string]any); ok {
					if et, ok := wt["enableTerminal"].(bool); ok {
						enableTerminal = et
					}
				}
			}
		}
	}

	if !enableTerminal {
		return
	}

	os.MkdirAll(filepath.Join(genDir, "terminal"), 0755)
	termAlpha := "100"

	// Get the scripts directory
	scriptDir := a.findScriptsDir(configDir)

	// Apply Ghostty theme
	ghosttyTemplate := filepath.Join(scriptDir, "colors", "terminal", "ghostty-theme.conf")
	if data, err := os.ReadFile(ghosttyTemplate); err == nil {
		content := string(data)
		for name, hexVal := range colors {
			content = strings.ReplaceAll(content, name+" #", strings.TrimPrefix(hexVal, "#"))
		}
		os.WriteFile(filepath.Join(genDir, "terminal", "ghostty-theme.conf"), []byte(content), 0644)
		// Signal ghostty
		if out, err := exec.Command("pidof", "ghostty").Output(); err == nil {
			for pidStr := range strings.FieldsSeq(strings.TrimSpace(string(out))) {
				if pid, err := strconv.Atoi(pidStr); err == nil {
					syscall.Kill(pid, syscall.SIGUSR1)
				}
			}
		}
	}

	// Apply generic terminal escape sequences
	seqTemplate := filepath.Join(scriptDir, "colors", "terminal", "sequences.txt")
	if data, err := os.ReadFile(seqTemplate); err == nil {
		content := string(data)
		for name, hexVal := range colors {
			content = strings.ReplaceAll(content, name+" #", strings.TrimPrefix(hexVal, "#"))
		}
		content = strings.ReplaceAll(content, "$alpha", termAlpha)
		os.WriteFile(filepath.Join(genDir, "terminal", "sequences.txt"), []byte(content), 0644)

		// Send to /dev/pts
		seqPath := filepath.Join(genDir, "terminal", "sequences.txt")
		seqData, _ := os.ReadFile(seqPath)
		entries, _ := os.ReadDir("/dev/pts")
		for _, entry := range entries {
			if !entry.IsDir() {
				matched, _ := regexp.MatchString(`^\d+$`, entry.Name())
				if matched {
					if f, err := os.OpenFile(filepath.Join("/dev/pts", entry.Name()), os.O_WRONLY, 0); err == nil {
						f.Write(seqData)
						f.Close()
					}
				}
			}
		}
	}

	// Also apply nvim and vscode colors
	a.handleNvimApplyColors()
	a.handleApplyVscodeColor()
}

func (a *App) handleHyprconfigEdit(args []string) {
	filePath := ""
	var setArgs [][2]string
	var resetKeys []string

	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	defaultFile := filepath.Join(configDir, "hypr", "hyprland.conf")

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
