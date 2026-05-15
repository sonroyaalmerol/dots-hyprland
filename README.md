# snry-shell

A desktop shell for [Hyprland](https://hyprland.org/) built on [Quickshell](https://quickshell.outfoxxed.me/) with a Go daemon backend.

<p align="center">
  <img src="https://img.shields.io/github/last-commit/sonroyaalmerol/snry-shell?&style=for-the-badge&color=8ad7eb&logo=git&logoColor=D9E0EE&labelColor=1E202B">
  <img src="https://img.shields.io/github/stars/sonroyaalmerol/snry-shell?style=for-the-badge&logo=andela&color=86dbd7&logoColor=D9E0EE&labelColor=1E202B">
  <img src="https://img.shields.io/github/repo-size/sonroyaalmerol/snry-shell?color=86dbce&label=SIZE&logo=protondrive&style=for-the-badge&logoColor=D9E0EE&labelColor=1E202B">
</p>

## Architecture

**Frontend** (Quickshell/QML) `shell.qml` → `services/DaemonSocket` ↔ Unix socket ↔ **Backend** (`snry-daemon`, Go)

- **Frontend** — QML/Qt6 panels (bar, dock, lock screen, notifications, launcher, overlay widgets, settings). Communicates with the daemon through `DaemonSocket`, a singleton that wraps a Unix domain socket connection. All image processing, color generation, recording, keyring access, and system management are routed through the daemon — no shell scripts or Python at runtime.

- **Backend** — `snry-daemon` is a long-running Go process listening on `$XDG_RUNTIME_DIR/snry-daemon.sock`. It accepts text commands (`switch-wallpaper <path>`, `record --fullscreen --sound`, `brightness-set 50`, etc.) and emits JSON events back to connected clients. It also doubles as a setup/management CLI (`snry-daemon setup`, `snry-daemon deps`, etc.).

## Features

### Panels & UI

- **Bar** — workspaces, system tray, media controls, quick toggles, volume mixer, clock, weather
- **Dock** — optional dock with pinned/running apps
- **Launcher** — app search, web search, calculator, cliphist history, action commands
- **Lock screen** — Hyprlock integration with Material You theming
- **Session screen** — suspend, hibernate, poweroff, reboot, firmware setup
- **Notifications** — notification daemon with popup history
- **Screen corners** — rounded corner overlays for Hyprland
- **On-screen keyboard** — virtual keyboard for tablet mode
- **Overlay widgets** — recorder controls, cheatsheet
- **Settings** — full GUI settings panel (`settings.qml`) plus first-run wizard (`welcome.qml`)
- **Region selector** — screenshot region, OCR, screen recording region selection

### Quick Toggles

Audio, Bluetooth, Dark Mode, EasyEffects, Game Mode, Night Light, Network, Notification, On-Screen Keyboard, Power Profiles, Screen Snip, Tablet Mode, Color Picker, Idle Inhibitor, Mic

### Theming

- **Material You** — automatic color scheme generation from wallpaper via [matugen](https://github.com/InioX/matugen) + custom HCT color space harmonization in Go
- **Terminal colors** — ghostty theme generation, terminal escape sequences
- **KVantum, VS Code, Neovim** — automatic theme application
- **Dark/light mode** — toggle with gsettings propagation
- **14 languages** — de, en, es, fr, he, id, it, ja, pt, ru, tr, uk, vi, zh

### Wallpaper

- Static image and video wallpaper support
- Random wallpaper from local collection or osu! skins
- Automatic Material You color generation on switch
- Contour-based region detection for smart widget placement
- Thumbnail generation (pure Go, no OpenCV)
- Text color detection for overlay readability

### Screen Recording

- Region, fullscreen, and fullscreen-with-sound recording via `wf-recorder`
- Region selection overlay with real-time preview

### Daemon Services

Battery, brightness, clipboard (cliphist), compositor (Hyprland IPC), conflict killer, dark mode, EasyEffects, Game Mode, Hyprland keybinds, Hyprsunset, XKB layout, idle detection, on-screen keyboard, lock screen, network (NM), power saving, resource monitor, session warnings, system info, tablet mode, updates, weather, keyring (gnome-keyring/secret-tool), polkit

## Smart Config Sync

snry-daemon includes a config sync engine that handles updates intelligently. Strategies are defined in [`internal/syncengine/categorize.go`](internal/syncengine/categorize.go):

| Strategy         | For files                                                                 | Behavior                                          |
| ---------------- | ------------------------------------------------------------------------- | ------------------------------------------------- |
| `overwrite`      | SVGs, PNGs, fonts, QML, Lua, fontconfig, Kvantum                          | Always replace with upstream                      |
| `merge-hyprland` | `hypr/hyprland/*.conf`                                                    | Section-aware merge (key-values + binds)          |
| `merge-kv`       | `hyprlock.conf`, `hypridle.conf`, `fuzzel/*.ini`, `**/*.conf`, `**/*.ini` | Key-value level three-way merge                   |
| `merge-section`  | `bash/bashrc`, `bash/bash_profile`, `bash/zprofile`                       | Merge only between `# >>> snry-shell >>>` markers |
| `skip-if-exists` | `hypr/custom/*.lua`, `monitors.conf`, `workspaces.conf`                   | Only deploy on first install                      |
| `template`       | `matugen/templates/*`                                                     | Render `{{.User}}`, `{{.Home}}`, etc., then merge |

Template variables: `{{.User}}`, `{{.Home}}`, `{{.ConfigDir}}`, `{{.DataDir}}`, `{{.StateDir}}`, `{{.BinDir}}`, `{{.CacheDir}}`, `{{.RuntimeDir}}`, `{{.VenvPath}}`, `{{.Fontset}}`

## Keybinds

Full keybind list is available in-app via `Super + /`.

| Keybind                   | Action                          |
| ------------------------- | ------------------------------- |
| **Shell**                 |                                 |
| `Super`                   | Toggle launcher / search        |
| `Super + Tab`             | Toggle overview                 |
| `Super + V`               | Clipboard history               |
| `Super + Period`          | Emoji picker                    |
| `Super + N`               | Toggle right sidebar            |
| `Super + /`               | Toggle cheatsheet               |
| `Super + K`               | Toggle on-screen keyboard       |
| `Super + M`               | Toggle media controls           |
| `Super + G`               | Toggle overlay                  |
| `Super + J`               | Toggle bar                      |
| `Ctrl + Super + T`        | Wallpaper selector              |
| `Ctrl + Super + Alt + T`  | Random wallpaper                |
| `Ctrl + Super + R`        | Restart widgets                 |
| `Ctrl + Super + P`        | Cycle panel family              |
| `Ctrl + Alt + Delete`     | Session menu                    |
| **Utilities**             |                                 |
| `Super + Shift + S`       | Screen snip (screenshot region) |
| `Super + Shift + A`       | Google Lens (region search)     |
| `Super + Shift + X`       | OCR → clipboard                 |
| `Super + Shift + T`       | Translate screen content        |
| `Super + Shift + C`       | Color picker → clipboard        |
| `Super + Shift + R`       | Record region (no sound)        |
| `Super + Shift + Alt + R` | Record fullscreen with sound    |
| `Print`                   | Screenshot → clipboard          |
| **Window**                |                                 |
| `Super + Q`               | Close window                    |
| `Super + ←/→/↑/↓`         | Focus in direction              |
| `Super + Shift + ←/→/↑/↓` | Move window in direction        |
| `Super + Alt + Space`     | Float/Tile toggle               |
| `Super + F`               | Fullscreen                      |
| `Super + P`               | Pin window                      |
| `Super + Alt + 1-10`      | Send to workspace 1-10          |
| **Workspace**             |                                 |
| `Super + 1-10`            | Focus workspace 1-10            |
| `Ctrl + Super + ←/→`      | Focus workspace left/right      |
| `Super + S`               | Toggle scratchpad               |
| `Super + Alt + S`         | Send to scratchpad              |
| **Session**               |                                 |
| `Super + L`               | Lock                            |
| `Super + Shift + L`       | Suspend                         |
| **Screen**                |                                 |
| `Super + -/+`             | Zoom out/in                     |
| **Media**                 |                                 |
| `Super + Shift + P`       | Play/pause                      |
| `Super + Shift + N`       | Next track                      |
| `Super + Shift + B`       | Previous track                  |
| **Apps**                  |                                 |
| `Super + Return`          | Terminal                        |
| `Super + E`               | File manager                    |
| `Super + W`               | Browser                         |
| `Super + C`               | Code editor                     |
| `Super + X`               | Text editor                     |
| `Super + I`               | Settings app                    |
| `Ctrl + Super + V`        | Volume mixer                    |
| `Ctrl + Shift + Escape`   | Task manager                    |

## Requirements

### Runtime

- [Quickshell](https://quickshell.outfoxxed.me/) (QML shell host)
- [Hyprland](https://hyprland.org/) (Wayland compositor)
- Go 1.26+ (build only)

### Optional

See `PKGBUILD` for the full dependency list. Key packages:

| Category | Packages                                        |
| -------- | ----------------------------------------------- |
| Audio    | pipewire, wireplumber, playerctl, cava          |
| Display  | brightnessctl, ddcutil, hyprsunset              |
| Capture  | slurp, grim, wf-recorder, swappy, tesseract     |
| Network  | networkmanager, plasma-nm                       |
| Desktop  | fuzzel, wlogout, hyprlock, hypridle, hyprpicker |
| Theming  | matugen, ghostty, kvantum                       |
| Keyring  | gnome-keyring, libsecret                        |

## Installation

### Arch Linux (AUR)

```sh
paru -S snry-shell-qs
snry-daemon setup
```

### From Source

```sh
git clone https://github.com/sonroyaalmerol/snry-shell.git
cd snry-shell
go build -o snry-daemon ./cmd/snry-daemon
./snry-daemon setup
```

## Usage

### CLI

| Command                  | Purpose                                         |
| ------------------------ | ----------------------------------------------- |
| `snry-daemon`            | Start daemon (default)                          |
| `snry-daemon daemon`     | Start daemon explicitly                         |
| `snry-daemon setup`      | Full install: deps + config sync + system setup |
| `snry-daemon deps`       | Install packages only                           |
| `snry-daemon files`      | Smart-sync config files (three-way merge)       |
| `snry-daemon setups`     | System setup (groups, systemd, PAM)             |
| `snry-daemon diagnose`   | Run diagnostics                                 |
| `snry-daemon checkdeps`  | Check dependency status                         |
| `snry-daemon autoscale`  | Auto-set monitor scale                          |
| `snry-daemon uninstall`  | Remove shell configuration                      |
| `snry-daemon send <cmd>` | Send command to running daemon                  |

### Sending Commands

```sh
# Switch wallpaper and regenerate colors
snry-daemon send "switch-wallpaper /path/to/image.jpg"

# Start fullscreen recording with audio
snry-daemon send "record --fullscreen --sound"

# Set brightness
snry-daemon send "brightness-set 80"

# Toggle game mode
snry-daemon send "gamemode-toggle"

# Get system resource usage
snry-daemon send "resources"
```

### Configuration

User config lives at `~/.config/snry-shell/config.json`. The settings GUI is available via `Super + I` or the sidebar settings button.

## Project Structure

```
cmd/snry-daemon/         Entrypoint (daemon + CLI)
configs/                 Dotfiles synced by snry-daemon setup
  bash/                  Shell configuration
  ghostty/               Terminal config
  hypr/                  Hyprland, hyprlock, hypridle configs
  matugen/               Material You color templates
  starship.toml          Prompt theme
  ...
data/                    OS-specific package lists (arch, fedora)
frontend/ii/
  assets/                Icons and images
  modules/
    common/              Shared widgets, config, utilities
    ii/                  Main UI panels (bar, dock, lock, etc.)
    settings/            Settings panels
  panelFamilies/         Panel loading logic
  scripts/               Static data (cava config, terminal scheme base)
  services/              QML service singletons (DaemonSocket, Audio, etc.)
  translations/          i18n JSON files (14 languages)
internal/
  daemon/
    app/                 Command dispatcher, daemon lifecycle
    socket/              Unix domain socket server with JSON events
    brightness/          Backlight control (brightnessctl, ddcutil)
    cliphist/            Clipboard history
    compositor/          Hyprland IPC
    conflict/            Tray/notification conflict killer
    darkmode/            Dark mode toggle (gsettings)
    easyeffects/         EasyEffects D-Bus control
    gamemode/            Feral GameMode D-Bus
    hyprland/            Monitor/workspace/window management
    hyprkeybinds/        Keybind parser
    hyprsunset/          Night light gamma control
    hyprxkb/             XKB layout tracking
    idle/                Idle inhibition (ext-idle-notify-v1)
    inputmethod/         On-screen keyboard (virtual-keyboard-v1)
    lock/                Screen locking
    lockscreen/          Lock state monitoring
    network/             NetworkManager D-Bus
    powersave/           Power profile control (UPower)
    quickshell/          Quickshell IPC integration
    resources/           CPU/memory monitoring
    session/             Loginctl session tracking
    sysinfo/             System information collection
    tabletmode/          Tablet mode detection
    updates/             Package update checking
    weather/             Weather data via wttr.in + geoclue
  image/                 Pure Go image processing (no CGo)
    image.go             Decoding, resizing, cropping
    regions.go           Contour-based region detection (Sobel + flood fill)
    leastbusy.go         Least-busy region detection (integral image variance)
    textcolor.go         Text color detection (corner sampling)
  wallpaper/             Color science & wallpaper management
    hct.go               HCT color space (CAM16 + HctSolver)
    scheme.go            Light/dark scheme detection
    terminal.go          Ghostty theme + terminal sequence generation
    wallpaper.go         Static/video wallpaper management
  manager/               Setup, deps, file sync, diagnostics
  syncengine/            Hyprland config parser + three-way merge engine
  xdg/                   XDG directory helpers
  lualint/               Config validation
  platform/              OS detection
```

## Credits

Originally forked from [@end-4](https://github.com/end-4)'s [dots-hyprland](https://github.com/end-4/dots-hyprland) and has since diverged into an independent project.

Thanks to:

- [@clsty](https://github.com/clsty) — install scripting and packaging
- [@midn8hustlr](https://github.com/midn8hustlr) — color generation system improvements
- [@outfoxxed](https://github.com/outfoxxed) — Quickshell development and support
- Quickshell community: [Soramane](https://github.com/caelestia-dots/shell/), [FridayFaerie](https://github.com/FridayFaerie/quickshell), [nydragon](https://github.com/nydragon/nysh)

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](.github/CONTRIBUTING.md) for guidelines.

## License

[GPLv3](LICENSE)
