package app

import (
	"context"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/brightness"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/cliphist"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/hyprland"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/idle"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/idle/dbusutil"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/inputmethod"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/lockscreen"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/powersave"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/quickshell"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/resources"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/socket"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/tabletmode"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/uinput"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/updates"
	"github.com/sonroyaalmerol/dots-hyprland/scripts/snry-daemon/internal/weather"
)

type Config struct {
	SocketPath       string
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
}

func DefaultConfig() Config {
	return Config{
		SocketPath:       os.Getenv("XDG_RUNTIME_DIR") + "/snry-daemon.sock",
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
	}
}

type App struct {
	cfg Config

	uinput        *uinput.Keyboard
	socketServer  *socket.Server
	idleSvc       *idle.Service
	lockscreenSvc *lockscreen.Service
	powersaveSvc  *powersave.Service
	resourcesSvc  *resources.Service
	hyprlandSvc   *hyprland.Service
	qsSvc         *quickshell.Service
	weatherSvc    *weather.Service
	updatesSvc    *updates.Service
	cliphistSvc   *cliphist.Service
	brightnessSvc *brightness.Service
}

func New(cfg Config) *App {
	return &App{cfg: cfg}
}

func (a *App) HyprlandSvc() *hyprland.Service   { return a.hyprlandSvc }
func (a *App) ResourcesSvc() *resources.Service { return a.resourcesSvc }

func (a *App) Run(ctx context.Context) error {
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
	}

	if a.powersaveSvc != nil {
		a.powersaveSvc.SetLockedProvider(a.lockscreenSvc.IsLocked)
	}

	a.resourcesSvc = resources.New(a.cfg.ResourcesCfg, a.socketServer.Emitter().Emit)
	a.hyprlandSvc = hyprland.New(a.cfg.HyprlandCfg, a.socketServer.Emitter().Emit)
	a.qsSvc = quickshell.New(a.cfg.QuickshellCfg)

	// Wire suspend check for weather and updates
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

	var wg sync.WaitGroup

	wg.Go(func() { a.runSocketServer(ctx) })
	wg.Go(func() { a.runIdle(ctx) })
	wg.Go(func() { a.runPowersaveTicker(ctx) })
	wg.Go(func() { a.runTabletMode(ctx) })
	wg.Go(func() { a.runResources(ctx) })
	wg.Go(func() { a.runHyprland(ctx) })
	wg.Go(func() { a.runInputMethod(ctx) })
	wg.Go(func() { a.runQuickshell(ctx) })
	wg.Go(func() { a.runWeather(ctx) })
	wg.Go(func() { a.runUpdates(ctx) })
	wg.Go(func() { a.runCliphist(ctx) })
	wg.Go(func() { a.runBrightness(ctx) })
	wg.Go(func() {
		time.Sleep(3 * time.Second)
		cleanup := a.setupHyprlandSystemBinds()
		defer cleanup()
	})

	<-ctx.Done()
	log.Printf("shutting down...")
	a.qsSvc.Stop()
	a.uinput.Close()
	wg.Wait()
	return nil
}

func (a *App) runSocketServer(ctx context.Context) {
	snapshots := make([]socket.SnapshotProvider, 0, 6)
	if a.hyprlandSvc != nil {
		snapshots = append(snapshots, a.hyprlandSvc)
	}
	if a.resourcesSvc != nil {
		snapshots = append(snapshots, a.resourcesSvc)
	}
	if a.weatherSvc != nil {
		snapshots = append(snapshots, a.weatherSvc)
	}
	if a.updatesSvc != nil {
		snapshots = append(snapshots, a.updatesSvc)
	}
	if a.brightnessSvc != nil {
		snapshots = append(snapshots, a.brightnessSvc)
	}
	if err := a.socketServer.Run(ctx, a, snapshots); err != nil {
		log.Printf("socket server: %v", err)
	}
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
		a.socketServer.Emitter().Emit(map[string]any{
			"event":  "tablet_mode",
			"active": tablet,
		})
	})
	tm.Run(ctx)
}

func (a *App) runResources(ctx context.Context) {
	if a.resourcesSvc == nil {
		return
	}
	if err := a.resourcesSvc.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("resources: %v", err)
	}
}

func (a *App) runHyprland(ctx context.Context) {
	if a.hyprlandSvc == nil {
		return
	}
	if err := a.hyprlandSvc.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("hyprland: %v", err)
	}
}

func (a *App) runInputMethod(ctx context.Context) {
	im, err := inputmethod.New(func(active bool) {
		a.socketServer.Emitter().Emit(map[string]any{
			"event":  "text_focus",
			"active": active,
		})
	})
	if err != nil {
		log.Printf("inputmethod: %v", err)
	}
	if im != nil {
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

func (a *App) runWeather(ctx context.Context) {
	if a.weatherSvc == nil {
		return
	}
	if err := a.weatherSvc.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("weather: %v", err)
	}
}

func (a *App) runUpdates(ctx context.Context) {
	if a.updatesSvc == nil {
		return
	}
	if err := a.updatesSvc.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("updates: %v", err)
	}
}

func (a *App) runCliphist(ctx context.Context) {
	if a.cliphistSvc == nil {
		return
	}
	if err := a.cliphistSvc.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("cliphist: %v", err)
	}
}

func (a *App) runBrightness(ctx context.Context) {
	if a.brightnessSvc == nil {
		return
	}
	if err := a.brightnessSvc.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("brightness: %v", err)
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

func (a *App) onLockscreenEvent(t lockscreen.EventType, data any) {
	switch t {
	case lockscreen.EventLockState:
		locked := data.(bool)
		if a.idleSvc != nil {
			a.idleSvc.NotifyLockChanged()
		}
		a.socketServer.Emitter().Emit(map[string]any{
			"event": "lock_state",
			"data": map[string]any{
				"locked": locked,
			},
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
			"data": map[string]any{
				"remainingSeconds": data.(int),
			},
		})
	}
}

func (a *App) onPowerState(suspended bool) {
	a.socketServer.Emitter().Emit(map[string]any{
		"event": "power_state",
		"data":  map[string]bool{"suspended": suspended},
	})
}

func (a *App) DispatchCommand(line string) {
	dispatchCommand(a, line)
}
