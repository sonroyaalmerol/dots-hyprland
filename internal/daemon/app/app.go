package app

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/brightness"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/cliphist"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/conflict"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/darkmode"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/easyeffects"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/gamemode"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/hyprkeybinds"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/hyprland"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/hyprsunset"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/hyprxkb"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/idle"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/idle/dbusutil"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/inputmethod"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/lock"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/lockscreen"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/network"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/powersave"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/quickshell"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/resources"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/session"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/socket"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/sysinfo"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/tabletmode"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/uinput"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/updates"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/warp"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/weather"
	"github.com/sonroyaalmerol/snry-shell-qs/internal/manager"
)

// ── Config ────────────────────────────────────────────────────────────────────

type Config struct {
	SocketPath       string
	LockPath         string
	QuickshellCfg    quickshell.Config
	IdleCfg          idle.Config
	LockscreenCfg    lockscreen.Config
	PowersaveTimeout time.Duration
	ResourcesCfg     resources.Config
	HyprlandCfg      hyprland.Config
	WeatherCfg       weather.Config
	UpdatesCfg       updates.Config
	CliphistCfg      cliphist.Config
	BrightnessCfg    brightness.Config
	SessionCfg       session.Config
	EasyEffectsCfg   easyeffects.Config
	HyprsunsetCfg    hyprsunset.Config
	HyprXkbCfg       hyprxkb.Config
	NetworkCfg       network.Config
	WarpCfg          warp.Config
	GamemodeCfg      gamemode.Config
	DarkmodeCfg      darkmode.Config
}

func DefaultConfig() Config {
	return Config{
		SocketPath:       os.Getenv("XDG_RUNTIME_DIR") + "/snry-daemon.sock",
		LockPath:         os.Getenv("XDG_RUNTIME_DIR") + "/snry-daemon.lock",
		QuickshellCfg:    quickshell.DefaultConfig(),
		IdleCfg:          idle.DefaultConfig(),
		LockscreenCfg:    lockscreen.DefaultConfig(),
		PowersaveTimeout: 30 * time.Second,
		ResourcesCfg:     resources.DefaultConfig(),
		HyprlandCfg:      hyprland.DefaultConfig(),
		WeatherCfg:       weather.DefaultConfig(),
		UpdatesCfg:       updates.DefaultConfig(),
		CliphistCfg:      cliphist.DefaultConfig(),
		BrightnessCfg:    brightness.DefaultConfig(),
		SessionCfg:       session.DefaultConfig(),
		EasyEffectsCfg:   easyeffects.DefaultConfig(),
		HyprsunsetCfg:    hyprsunset.DefaultConfig(),
		HyprXkbCfg:       hyprxkb.DefaultConfig(),
		NetworkCfg:       network.DefaultConfig(),
		WarpCfg:          warp.DefaultConfig(),
		GamemodeCfg:      gamemode.DefaultConfig(),
		DarkmodeCfg:      darkmode.DefaultConfig(),
	}
}

// ── State event ───────────────────────────────────────────────────────────────

// stateEvent is sent to the state loop for serialized processing.
type stateEvent struct {
	kind   string // "tablet", "text_focus", "lock", "command"
	active bool   // for tablet/text_focus/lock
	cmd    string // for command
	arg    string // for command argument
}

// ── App ───────────────────────────────────────────────────────────────────────

type App struct {
	cfg Config

	lockFile        *os.File
	uinput          *uinput.Keyboard
	socketServer    *socket.Server
	idleSvc         *idle.Service
	lockscreenSvc   *lockscreen.Service
	powersaveSvc    *powersave.Service
	resourcesSvc    *resources.Service
	hyprlandSvc     *hyprland.Service
	qsSvc           *quickshell.Service
	weatherSvc      *weather.Service
	updatesSvc      *updates.Service
	cliphistSvc     *cliphist.Service
	brightnessSvc   *brightness.Service
	sysinfoSvc      *sysinfo.Service
	sessionSvc      *session.Service
	easyEffectsSvc  *easyeffects.Service
	hyprKeybindsSvc *hyprkeybinds.Service
	hyprXkbSvc      *hyprxkb.Service
	hyprsunsetSvc   *hyprsunset.Service
	tabletModeMon   *tabletmode.Monitor
	inputMethodW    *inputmethod.Watcher
	networkSvc      *network.Service
	warpSvc         *warp.Service
	gamemodeSvc     *gamemode.Service
	darkmodeSvc     *darkmode.Service
	conflictSvc     *conflict.Service

	// State loop — all state mutations happen in a single goroutine.
	stateCh chan stateEvent

	// OSK hide delay — prevents flash when focus hops between text fields.
	oskHideTimer *time.Timer

	// Last emitted state for dedup.
	lastStateHash   uint64
	lastFocusActive time.Time

	// Canonical state (accessed only from stateLoop goroutine).
	hwTablet     bool
	textFocus    bool
	screenLocked bool
	userMode     int32 // 0=auto, 1=tablet, 2=desktop
	oskVisible   bool
	oskAutoShown bool
	oskDismissed bool
	oskPinned    bool
}

func New(cfg Config) *App {
	return &App{cfg: cfg, stateCh: make(chan stateEvent, 64)}
}

func (a *App) HyprlandSvc() *hyprland.Service   { return a.hyprlandSvc }
func (a *App) ResourcesSvc() *resources.Service { return a.resourcesSvc }

// ── Run ───────────────────────────────────────────────────────────────────────

func (a *App) Run(ctx context.Context) error {
	lockFile, err := lock.Acquire(a.cfg.LockPath)
	if err != nil {
		return fmt.Errorf("singleton lock: %w", err)
	}
	a.lockFile = lockFile

	a.uinput = uinput.NewKeyboard()
	if err := a.uinput.Init(); err != nil {
		log.Printf("uinput: %v (virtual keyboard disabled)", err)
	}

	a.socketServer = socket.New(a.cfg.SocketPath)

	a.lockscreenSvc = lockscreen.New(a.cfg.LockscreenCfg, a.onLockscreenEvent)
	a.powersaveSvc = powersave.New(a.cfg.PowersaveTimeout, a.onPowerState)

	idleConn, err := dbus.SystemBus()
	if err != nil {
		log.Printf("system bus: %v (idle service disabled)", err)
	} else {
		a.idleSvc = idle.New(dbusutil.NewRealConn(idleConn), a.cfg.IdleCfg)
		a.idleSvc.SetLockedProvider(a.lockscreenSvc.IsLocked)
		a.idleSvc.SetOnLock(func() {
			if a.lockscreenSvc != nil {
				a.lockscreenSvc.Lock()
			}
		})
		a.idleSvc.SetOnDisplayChange(func(on bool) {
			if a.powersaveSvc != nil {
				a.powersaveSvc.SetScreenOff(!on)
			}
		})
		a.idleSvc.SetOnLogindUnlock(func() {
			if a.lockscreenSvc != nil {
				a.lockscreenSvc.Unlock()
			}
		})

		a.lockscreenSvc.EmitState()
		if a.lockscreenSvc.TryAutoUnlock() {
			log.Printf("[app] auto-unlocked on startup (keyring available)")
		}
		if a.idleSvc != nil {
			a.idleSvc.NotifyLockChanged()
		}
	}

	if a.powersaveSvc != nil {
		a.powersaveSvc.SetLockedProvider(a.lockscreenSvc.IsLocked)
	}

	a.resourcesSvc = resources.New(a.cfg.ResourcesCfg, a.socketServer.Emitter().Emit)
	a.hyprlandSvc = hyprland.New(a.cfg.HyprlandCfg, a.socketServer.Emitter().Emit)
	a.qsSvc = quickshell.New(a.cfg.QuickshellCfg)

	isSuspended := func() bool {
		return a.powersaveSvc != nil && a.powersaveSvc.IsSuspended()
	}

	weatherCfg := a.cfg.WeatherCfg
	weatherCfg.IsSuspended = isSuspended
	a.weatherSvc = weather.New(weatherCfg, a.socketServer.Emitter().Emit)

	updatesCfg := a.cfg.UpdatesCfg
	updatesCfg.IsSuspended = isSuspended
	a.updatesSvc = updates.New(updatesCfg, a.socketServer.Emitter().Emit)

	a.cliphistSvc = cliphist.New(a.cfg.CliphistCfg, a.socketServer.Emitter().Emit)
	a.brightnessSvc = brightness.New(a.cfg.BrightnessCfg, a.socketServer.Emitter().Emit)

	// New daemon services
	a.sysinfoSvc = sysinfo.New(a.socketServer.Emitter().Emit)
	a.hyprKeybindsSvc = hyprkeybinds.New(a.socketServer.Emitter().Emit)

	if a.hyprlandSvc != nil {
		a.hyprXkbSvc = hyprxkb.New(a.cfg.HyprXkbCfg, a.hyprlandSvc.QuerySocket, a.socketServer.Emitter().Emit)
		a.hyprsunsetSvc = hyprsunset.New(a.cfg.HyprsunsetCfg,
			func(cmd string) ([]byte, error) { return a.hyprlandSvc.QuerySocket(cmd) },
			func(cmd string) error { _, err := a.hyprlandSvc.QuerySocket(cmd); return err },
			a.socketServer.Emitter().Emit,
		)
	}

	a.sessionSvc = session.New(a.cfg.SessionCfg, a.socketServer.Emitter().Emit)
	a.easyEffectsSvc = easyeffects.New(a.cfg.EasyEffectsCfg, a.socketServer.Emitter().Emit)

	a.networkSvc = network.New(a.cfg.NetworkCfg, a.socketServer.Emitter().Emit)

	a.warpSvc = warp.New(a.cfg.WarpCfg, a.socketServer.Emitter().Emit)
	a.gamemodeSvc = gamemode.New(a.cfg.GamemodeCfg, a.socketServer.Emitter().Emit)
	a.darkmodeSvc = darkmode.New(a.cfg.DarkmodeCfg, a.socketServer.Emitter().Emit)
	a.conflictSvc = conflict.New()

	var wg sync.WaitGroup

	wg.Go(func() { a.runStateLoop(ctx) })
	wg.Go(func() { a.runSocketServer(ctx) })
	wg.Go(func() { a.runIdle(ctx) })
	wg.Go(func() { a.runPowersaveTicker(ctx) })
	wg.Go(func() { a.runTabletMode(ctx) })
	wg.Go(func() { a.runInputMethod(ctx) })
	wg.Go(func() { a.runQuickshell(ctx) })
	wg.Go(func() { a.runService(ctx, "resources", a.resourcesSvc) })
	wg.Go(func() { a.runService(ctx, "hyprland", a.hyprlandSvc) })
	wg.Go(func() { a.runService(ctx, "weather", a.weatherSvc) })
	wg.Go(func() { a.runService(ctx, "updates", a.updatesSvc) })
	wg.Go(func() { a.runService(ctx, "cliphist", a.cliphistSvc) })
	wg.Go(func() { a.runService(ctx, "brightness", a.brightnessSvc) })
	wg.Go(func() { a.runService(ctx, "sysinfo", a.sysinfoSvc) })
	wg.Go(func() { a.runService(ctx, "session", a.sessionSvc) })
	wg.Go(func() { a.runService(ctx, "easyeffects", a.easyEffectsSvc) })
	wg.Go(func() { a.runService(ctx, "hyprkeybinds", a.hyprKeybindsSvc) })
	wg.Go(func() { a.runService(ctx, "hyprxkb", a.hyprXkbSvc) })
	wg.Go(func() { a.runService(ctx, "hyprsunset", a.hyprsunsetSvc) })
	wg.Go(func() { a.runService(ctx, "network", a.networkSvc) })
	wg.Go(func() { a.runService(ctx, "warp", a.warpSvc) })
	wg.Go(func() { a.runService(ctx, "gamemode", a.gamemodeSvc) })
	wg.Go(func() { a.runService(ctx, "darkmode", a.darkmodeSvc) })
	wg.Go(func() {
		time.Sleep(3 * time.Second)
		cleanup := a.setupHyprlandSystemBinds()
		defer cleanup()
	})

	<-ctx.Done()
	log.Printf("shutting down...")
	a.qsSvc.Stop()
	a.uinput.Close()
	if a.lockFile != nil {
		a.lockFile.Close()
	}
	wg.Wait()
	return nil
}

// ── Service runner helper ─────────────────────────────────────────────────────

// runService runs a named service if non-nil, logging errors.
func (a *App) runService(ctx context.Context, name string, svc interface{ Run(context.Context) error }) {
	if svc == nil {
		return
	}
	if err := svc.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("%s: %v", name, err)
	}
}

// ── State helpers ─────────────────────────────────────────────────────────────

func modeStr(mode int32) string {
	switch mode {
	case 1:
		return "tablet"
	case 2:
		return "desktop"
	default:
		return "auto"
	}
}

func (a *App) buildStateData() map[string]any {
	return map[string]any{
		"hardware_tablet":       a.hwTablet,
		"text_focus":            a.textFocus,
		"effective_tablet_mode": a.effectiveTablet(),
		"user_mode":             modeStr(a.userMode),
		"osk_visible":           a.oskVisible,
		"osk_dismissed":         a.oskDismissed,
		"osk_pinned":            a.oskPinned,
		"screen_locked":         a.screenLocked,
	}
}

// ── State loop ────────────────────────────────────────────────────────────────

// runStateLoop is the single goroutine that owns all canonical state.
// Every state mutation goes through stateCh to avoid races.
func (a *App) runStateLoop(ctx context.Context) {
	a.loadUserMode()

	for {
		select {
		case <-ctx.Done():
			a.saveUserMode()
			return
		case ev := <-a.stateCh:
			switch ev.kind {
			case "tablet":
				a.onHardwareTablet(ev.active)
			case "text_focus":
				a.onTextFocus(ev.active)
			case "lock":
				a.onScreenLock(ev.active)
			case "osk_hide_timeout":
				a.oskHideTimer = nil
				a.recomputeOsk()
			case "command":
				a.handleStateCommand(ev.cmd, ev.arg)
			}
			a.emitState()
		}
	}
}

func (a *App) effectiveTablet() bool {
	return a.userMode == 1 || (a.userMode == 0 && a.hwTablet)
}

func (a *App) computeOskVisible() bool {
	if a.oskPinned {
		return true
	}
	if a.screenLocked {
		return false
	}
	return a.effectiveTablet() && a.textFocus && !a.oskDismissed
}

func (a *App) recomputeOsk() {
	prev := a.oskVisible
	a.oskVisible = a.computeOskVisible()
	if a.oskVisible && !prev && !a.oskPinned {
		a.oskAutoShown = true
	}
	if !a.oskVisible {
		a.oskAutoShown = false
	}
}

func (a *App) onHardwareTablet(active bool) {
	a.hwTablet = active
	a.recomputeOsk()
}

func (a *App) onTextFocus(active bool) {
	if active {
		a.lastFocusActive = time.Now()
		a.textFocus = true
		a.oskDismissed = false
		// Cancel pending hide — focus returned before delay expired.
		if a.oskHideTimer != nil {
			a.oskHideTimer.Stop()
			a.oskHideTimer = nil
		}
		a.recomputeOsk()
	} else {
		// Suppress rapid deactivate→activate cycles caused by layers
		// stealing focus briefly (OSK panel, overview search, etc.).
		if time.Since(a.lastFocusActive) < 300*time.Millisecond {
			a.textFocus = true // keep reporting focus as active
			return
		}
		a.textFocus = false
		if a.oskHideTimer != nil {
			a.oskHideTimer.Stop()
		}
		a.oskHideTimer = time.AfterFunc(200*time.Millisecond, func() {
			a.stateCh <- stateEvent{kind: "osk_hide_timeout"}
		})
	}
}

func (a *App) onScreenLock(locked bool) {
	a.screenLocked = locked
	a.recomputeOsk()
}

func (a *App) handleStateCommand(cmd, arg string) {
	switch cmd {
	case "set-mode":
		switch arg {
		case "auto":
			a.userMode = 0
		case "tablet":
			a.userMode = 1
		case "desktop":
			a.userMode = 2
		default:
			return
		}
		a.saveUserMode()
		a.reevalOsk()

	case "cycle-mode":
		a.userMode = (a.userMode + 1) % 3
		a.saveUserMode()
		a.reevalOsk()

	case "osk-dismiss":
		a.oskDismissed = true
		a.recomputeOsk()

	case "osk-undismiss":
		a.oskDismissed = false
		a.recomputeOsk()

	case "osk-toggle":
		if a.oskVisible {
			a.oskDismissed = true
		} else {
			a.oskDismissed = false
		}
		a.recomputeOsk()

	case "osk-show":
		a.oskDismissed = false
		a.recomputeOsk()

	case "osk-hide":
		a.oskDismissed = true
		a.recomputeOsk()

	case "osk-pin":
		a.oskPinned = true
		a.recomputeOsk()

	case "osk-unpin":
		a.oskPinned = false
		a.recomputeOsk()
	}
}

func (a *App) reevalOsk() {
	a.recomputeOsk()
}

// ── State emit ────────────────────────────────────────────────────────────────

func (a *App) emitState() {
	data := a.buildStateData()

	// Dedup: skip emission if state hasn't changed.
	h := fnv.New64a()
	fmt.Fprintf(h, "%v%v%v%v%v%v%v%v",
		a.hwTablet, a.textFocus, a.effectiveTablet(), data["user_mode"],
		a.oskVisible, a.oskDismissed, a.oskPinned, a.screenLocked)
	newHash := h.Sum64()
	if newHash == a.lastStateHash {
		return
	}
	a.lastStateHash = newHash

	a.socketServer.Emitter().Emit(map[string]any{
		"event": "state",
		"data":  data,
	})
}

// EmitSnapshot implements socket.SnapshotProvider so new QML clients
// receive the full current state on connect.
func (a *App) EmitSnapshot(emit func(map[string]any)) {
	emit(map[string]any{
		"event": "state",
		"data":  a.buildStateData(),
	})
}

// ── User mode persistence ─────────────────────────────────────────────────────

func (a *App) userModePath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "snry-shell", "tablet-mode")
}

func (a *App) loadUserMode() {
	data, err := os.ReadFile(a.userModePath())
	if err != nil {
		return
	}
	switch strings.TrimSpace(string(data)) {
	case "tablet":
		a.userMode = 1
	case "desktop":
		a.userMode = 2
	default:
		a.userMode = 0
	}
}

func (a *App) saveUserMode() {
	dir := filepath.Dir(a.userModePath())
	_ = os.MkdirAll(dir, 0755)
	_ = os.WriteFile(a.userModePath(), []byte(modeStr(a.userMode)), 0644)
}

// ── Service runners ───────────────────────────────────────────────────────────

func (a *App) runSocketServer(ctx context.Context) {
	if err := a.socketServer.Run(ctx, a, a.socketSnapshots); err != nil {
		log.Printf("socket server: %v", err)
	}
}

func (a *App) socketSnapshots() []socket.SnapshotProvider {
	var snaps []socket.SnapshotProvider
	snap := func(s socket.SnapshotProvider) {
		if s != nil {
			snaps = append(snaps, s)
		}
	}
	snap(a) // App always provides state
	snap(a.lockscreenSvc)
	snap(a.hyprlandSvc)
	snap(a.resourcesSvc)
	snap(a.weatherSvc)
	snap(a.updatesSvc)
	snap(a.brightnessSvc)
	snap(a.sysinfoSvc)
	snap(a.sessionSvc)
	snap(a.easyEffectsSvc)
	snap(a.hyprKeybindsSvc)
	snap(a.hyprXkbSvc)
	snap(a.hyprsunsetSvc)
	snap(a.networkSvc)
	snap(a.warpSvc)
	snap(a.gamemodeSvc)
	snap(a.darkmodeSvc)
	return snaps
}

func (a *App) runIdle(ctx context.Context) {
	if a.idleSvc == nil {
		return
	}
	if err := a.idleSvc.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("idle service: %v", err)
	}
}

func (a *App) runPowersaveTicker(ctx context.Context) {
	if a.powersaveSvc == nil {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.powersaveSvc.Tick()
		}
	}
}

func (a *App) runTabletMode(ctx context.Context) {
	tabletConn, err := dbus.SystemBus()
	var conn *dbus.Conn
	if err != nil {
		log.Printf("system bus: %v (tablet mode logind disabled)", err)
	} else {
		conn = tabletConn
	}
	tm := tabletmode.New(conn, func(tablet bool) {
		a.stateCh <- stateEvent{kind: "tablet", active: tablet}
	})
	a.tabletModeMon = tm
	tm.Run(ctx)
}

func (a *App) runInputMethod(ctx context.Context) {
	im, err := inputmethod.New(func(active bool) {
		a.stateCh <- stateEvent{kind: "text_focus", active: active}
	})
	if err != nil {
		log.Printf("inputmethod: %v", err)
	}
	if im != nil {
		a.inputMethodW = im
		im.Run(ctx)
	} else {
		log.Printf("zwp_input_method_v2 not available, text focus events disabled")
	}
}

func (a *App) runQuickshell(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("quickshell: panic: %v", r)
		}
	}()
	log.Printf("quickshell: service goroutine starting")
	if err := a.qsSvc.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("quickshell: %v", err)
	}
}

func (a *App) setupHyprlandSystemBinds() func() {
	binds := []struct{ key, cmd string }{
		{"XF86PowerOff", "~/.local/bin/snry-daemon send power-button"},
		{"switch:on:Lid Switch", "~/.local/bin/snry-daemon send lid-close"},
	}
	for _, b := range binds {
		val := ", " + b.key + ", exec, " + b.cmd
		out, err := exec.Command("hyprctl", "keyword", "bindl", val).CombinedOutput()
		if err != nil {
			log.Printf("hyprland bindl %s: %v: %s", b.key, err, string(out))
		} else {
			log.Printf("hyprland bindl registered: %s", b.key)
		}
	}
	return func() {
		for _, b := range binds {
			out, err := exec.Command("hyprctl", "keyword", "unbind", ", "+b.key).CombinedOutput()
			if err != nil {
				log.Printf("hyprland unbind %s: %v: %s", b.key, err, string(out))
			}
		}
	}
}

// ── Callbacks ─────────────────────────────────────────────────────────────────

func (a *App) onLockscreenEvent(t lockscreen.EventType, data any) {
	switch t {
	case lockscreen.EventLockState:
		locked := data.(bool)
		if a.idleSvc != nil {
			a.idleSvc.NotifyLockChanged()
		}
		a.stateCh <- stateEvent{kind: "lock", active: locked}
		// Keep legacy lock_state event for lock screen QML.
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "lock_state",
			"data":  map[string]any{"locked": locked},
		})
	case lockscreen.EventAuthResult:
		r := data.(lockscreen.AuthResult)
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "auth_result",
			"data": map[string]any{
				"success":   r.Success,
				"remaining": r.Remaining,
				"lockedOut": r.LockedOut,
				"message":   r.Message,
			},
		})
	case lockscreen.EventLockoutTick:
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "lockout_tick",
			"data":  map[string]any{"remainingSeconds": data.(int)},
		})
	}
}

func (a *App) onPowerState(suspended bool) {
	a.socketServer.Emitter().Emit(map[string]any{
		"event": "power_state",
		"data":  map[string]bool{"suspended": suspended},
	})
}

// ── Administrative commands ───────────────────────────────────────────────────

func (a *App) handleAutoscale() {
	a.socketServer.Emitter().Emit(map[string]any{"event": "autoscale_start"})
	err := manager.Autoscale(context.Background())
	if err != nil {
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "autoscale_error",
			"data":  map[string]string{"error": err.Error()},
		})
		return
	}
	a.socketServer.Emitter().Emit(map[string]any{"event": "autoscale_done"})
}

func (a *App) handleCheckdeps() {
	a.socketServer.Emitter().Emit(map[string]any{"event": "checkdeps_start"})
	cfg := manager.DefaultConfig(a.repoRoot())
	err := manager.CheckDeps(context.Background(), cfg)
	if err != nil {
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "checkdeps_error",
			"data":  map[string]string{"error": err.Error()},
		})
		return
	}
	a.socketServer.Emitter().Emit(map[string]any{"event": "checkdeps_done"})
}

func (a *App) handleDiagnose() {
	a.socketServer.Emitter().Emit(map[string]any{"event": "diagnose_start"})
	cfg := manager.DefaultConfig(a.repoRoot())
	err := manager.Diagnose(context.Background(), cfg)
	if err != nil {
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "diagnose_error",
			"data":  map[string]string{"error": err.Error()},
		})
		return
	}
	a.socketServer.Emitter().Emit(map[string]any{"event": "diagnose_done"})
}

func (a *App) handleConflictCheck() {
	result := a.conflictSvc.Check()
	a.socketServer.Emitter().Emit(map[string]any{
		"event": "conflict_result",
		"data": map[string]any{
			"trays":         result["trays"],
			"notifications": result["notifications"],
		},
	})
}

func (a *App) repoRoot() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	shareDir := filepath.Join(filepath.Dir(exe), "..", "share", "snry-shell")
	if _, err := os.Stat(shareDir); err == nil {
		return shareDir
	}
	return "."
}

func (a *App) DispatchCommand(line string) {
	dispatchCommand(a, line)
}
