package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
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
