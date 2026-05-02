# snry-daemon Architecture

snry-daemon is the central manager for the snry-shell desktop environment. It handles
installation, configuration, runtime services, and system management — all implemented
natively in Go, with no external automation tool dependencies.

## Directory Structure

```
snry-shell-qs/
├── cmd/
│   └── snry-daemon/           # CLI entrypoint
│       └── main.go
├── internal/
│   ├── daemon/                # Application orchestration + runtime services
│   │   ├── app/               # Daemon mode app + command dispatch
│   │   │   ├── app.go
│   │   │   └── commands.go
│   │   ├── brightness/
│   │   ├── cliphist/
│   │   ├── hyprland/
│   │   ├── idle/
│   │   ├── inputmethod/
│   │   ├── lock/
│   │   ├── lockscreen/
│   │   ├── powersave/
│   │   ├── quickshell/
│   │   ├── resources/
│   │   ├── socket/            # Unix socket server for runtime commands
│   │   ├── tabletmode/
│   │   ├── uinput/
│   │   ├── updates/
│   │   └── weather/
│   ├── manager/               # Installation & config management
│   │   ├── manager.go         # Manager interface + orchestrator
│   │   ├── config.go          # Config struct with XDG path resolution
│   │   ├── filesync.go        # Low-level file ops (CopyFile, EnsureDir, etc.)
│   │   ├── files.go           # Smart sync steps for all config categories
│   │   ├── diagnose.go        # System diagnostics
│   │   ├── autoscale.go       # Monitor autoscale
│   │   ├── checkdeps.go       # Dependency checking
│   │   ├── steps.go           # Step runner with progress reporting
│   │   ├── setup.go           # System setup (groups, systemd, PAM, etc.)
│   │   ├── uninstall.go       # Clean removal of all deployed files
│   │   ├── arch/              # Arch Linux implementation
│   │   │   └── deps.go        # Package installation via paru
│   │   └── fedora/            # Fedora implementation
│   │       └── deps.go        # Package installation via dnf + COPR
│   ├── syncengine/            # Smart config sync engine
│   │   ├── engine.go          # SyncEngine orchestrator
│   │   ├── manifest.go        # JSON manifest with SHA256 checksum tracking
│   │   ├── categorize.go      # File → strategy rule matching
│   │   ├── template.go        # Safe variable substitution
│   │   ├── conflict.go        # Conflict logging (.orig/.new + JSONL)
│   │   ├── hyprparse/         # Hyprland config parser + section-aware merge
│   │   ├── kvparse/           # INI/key-value parser + three-way merge
│   │   └── sectionparse/      # Marker-block parser + section merge
│   ├── platform/              # OS detection + root-aware helpers
│   │   └── detect.go
│   └── xdg/                   # XDG base directory resolution
│       └── xdg.go
├── frontend/                  # Quickshell QML/UI
│   └── ii/
│       ├── shell.qml
│       ├── modules/
│       ├── services/
│       │   └── DaemonSocket.qml   # Socket client for daemon IPC
│       └── ...
├── data/                      # Package manifests
│   ├── arch/
│   │   ├── packages.conf
│   │   └── microtex-git/
│   ├── fedora/
│   │   ├── feddeps.toml
│   │   └── SPECS/
│   └── python/
│       ├── requirements.in
│       └── requirements.txt
├── configs/                   # Config templates (synced to XDG paths)
│   ├── quickshell/            # → $XDG_CONFIG_HOME/quickshell/
│   ├── hypr/                  # → $XDG_CONFIG_HOME/hypr/
│   ├── bash/                  # → $XDG_CONFIG_HOME/bash/ + ~/dotfiles
│   ├── fontconfig/            # → $XDG_CONFIG_HOME/fontconfig/
│   ├── fuzzel/                # → $XDG_CONFIG_HOME/fuzzel/
│   ├── wlogout/               # → $XDG_CONFIG_HOME/wlogout/
│   ├── foot/                  # → $XDG_CONFIG_HOME/foot/
│   ├── ghostty/               # → $XDG_CONFIG_HOME/ghostty/
│   ├── Kvantum/               # → $XDG_CONFIG_HOME/Kvantum/
│   ├── matugen/               # → $XDG_CONFIG_HOME/matugen/
│   ├── mpv/                   # → $XDG_CONFIG_HOME/mpv/
│   ├── kde-material-you-colors/
│   ├── zshrc.d/
│   ├── xdg-desktop-portal/
│   ├── systemd/user/          # → $XDG_CONFIG_HOME/systemd/user/
│   ├── starship.toml, darklyrc, dolphinrc, kdeglobals, konsolerc
│   ├── *-flags.conf           # chrome/code/thorium flags
│   └── extra/fontsets/        # Alternate font configurations
├── go.mod
├── go.sum
├── PKGBUILD
├── snry-shell-qs.install      # AUR post_install/post_upgrade hooks
├── LICENSE
└── docs/
    ├── ARCHITECTURE.md
    └── SMART_SYNC.md
```

## CLI Subcommands

```
snry-daemon              # Start daemon (default, no subcommand)
snry-daemon daemon       # Explicit daemon start
snry-daemon setup        # Full installation (deps + files + setups)
snry-daemon deps         # Install packages only
snry-daemon files        # Smart-sync config files only
snry-daemon setups       # System setup only (groups, systemd, PAM)
snry-daemon diagnose     # Collect system diagnostics
snry-daemon checkdeps    # Check missing packages
snry-daemon autoscale    # Auto-set monitor scale
snry-daemon uninstall    # Remove installed files and revert changes
snry-daemon send <cmd>   # Send command to running daemon via Unix socket
```

### Socket Commands (runtime daemon)

The daemon listens on `$XDG_RUNTIME_DIR/snry-daemon.sock` for line-based text commands:

| Command | Response |
|---------|----------|
| `brightness-up` | Increase display brightness |
| `brightness-down` | Decrease display brightness |
| `autoscale` | Auto-set monitor scale factor |
| `checkdeps` | Check for missing packages |
| `diagnose` | Return system diagnostics |
| `reload-hyprland` | Reload Hyprland config |

## Manager Architecture

### Interface Design

```go
// internal/manager/manager.go

type PackageManager interface {
    InstallPackages(ctx context.Context, packages []string) error
    InstallBuildDeps(ctx context.Context) error
    UpdateSystem(ctx context.Context) error
    CheckPackages(ctx context.Context, packages []string) ([]string, error)
    Distro() distro.Family
}

type Step struct {
    Name     string
    Fn       func(ctx context.Context) error
    Optional bool
}
```

### Privilege Escalation

The manager runs as the current user. Privileged operations use `sudo` internally.
When running as root (e.g. from AUR `post_install`), sudo is skipped automatically:

```go
// internal/platform/detect.go

func IsRoot() bool
func RealUser() (string, error)  // resolves real user even when running as root
func HomeDir() string            // resolves real $HOME from /etc/passwd when root

// SudoCmd/SudoCmd skip sudo prefix when already root
func RunSudo(ctx context.Context, args ...string) error
```

### Smart Config Sync Engine

All config file deployment uses the smart sync engine (`internal/syncengine/`).
It replaces simple file copying with a three-way merge system that preserves
user customizations across updates.

**Sync flow for each file:**
1. Load SHA256 manifest (tracks what was previously deployed)
2. Categorize file → determine merge strategy
3. Compare three checksums: original (manifest), current (disk), upstream (repo)
4. Decide: noop / update / keep / conflict / new
5. Apply strategy-specific merge algorithm
6. Write result atomically (temp file + rename)
7. Update manifest with new checksums

**Merge strategies:**

| Strategy | For files | Behavior |
|----------|-----------|----------|
| `overwrite` | SVGs, PNGs, fonts, quickshell QML | Always replace |
| `merge-hyprland` | `hypr/hyprland/*.conf` | Section-aware merge with key-value + bind/exec set merging |
| `merge-kv` | hyprlock.conf, hypridle.conf, fuzzel.ini | Key-value level three-way merge |
| `merge-section` | bashrc, bash_profile, zprofile | Only merge content between `# >>> snry-shell >>>` markers |
| `skip-if-exists` | monitors.conf, workspaces.conf | Only deploy on first install |
| `template` | Files with `{{.User}}` etc. | Render variables first, then apply underlying strategy |

See `docs/SMART_SYNC.md` for full design details.

### Config

```go
// internal/manager/config.go

type Config struct {
    SkipSysUpdate   bool
    SkipQuickshell  bool
    SkipHyprland    bool
    SkipBash        bool
    SkipFontconfig  bool
    SkipMiscConf    bool
    SkipBackup      bool
    Force           bool

    FontsetDirName string

    XDG  xdg.Paths
    Home string
    RepoRoot string
}
```

### Step Runner

```go
// internal/manager/steps.go

type StepResult struct {
    Name    string
    Skipped bool
    Err     error
}

type ProgressFunc func(step string, current, total int)

func RunSteps(ctx context.Context, steps []Step, progress ProgressFunc) []StepResult
```

Steps run sequentially. Optional steps log errors but don't halt. Context
cancellation stops the pipeline immediately.

## Memory Management Patterns

1. **Context propagation**: Every operation accepts `context.Context` as first param.
2. **No goroutine leaks**: Every goroutine has a `select { case <-ctx.Done(): return }` exit path.
3. **Resource cleanup**: `defer` for file handles, temp files. `t.TempDir()` in tests.
4. **No mutable globals**: All state in structs, injected via constructors.
5. **Proper error wrapping**: `fmt.Errorf("operation: %w", err)`.
6. **Preallocate slices**: `make([]T, 0, n)` when size is known.
7. **Atomic writes**: All file writes use temp-file-then-rename pattern.

## Error Handling

- Return errors, do not panic.
- Wrap with context: `fmt.Errorf("install packages: %w", err)`
- Sentinel errors for expected conditions:
  ```go
  var ErrUnsupportedDistro = errors.New("unsupported distribution")
  ```
- Handle errors once: either log+degrade or wrap+return, never both.
- Use `errors.Join` for multi-operation failures.

## AUR Installation (Hands-Off)

The `snry-shell-qs.install` script splits `post_install` into two phases:

1. **System setup (root)**: Runs `snry-daemon setups` as root — creates user groups,
   enables systemd services, configures PAM, sets up udev rules.
2. **Config sync (user)**: Detects the first normal user via `logname` or `/etc/passwd` scan,
   then runs `snry-daemon files` as that user — deploys all config files through the
   smart sync engine.

`post_upgrade` runs `snry-daemon files` to sync any updated configs.

The PKGBUILD `depends` array includes all ~70 required packages so pacman handles
them before the install script runs.

## PKGBUILD

```bash
makedepends=('go')
depends=(git rsync python uv sudo findutils hyprland quickshell ...
         # all ~70 packages from packages.conf
        )

# Binary symlink for keybind compatibility:
# /usr/bin/snry-daemon → installed by makepkg
# ~/.local/bin/snry-daemon → symlinked to /usr/bin/ for Hyprland keybinds
```
