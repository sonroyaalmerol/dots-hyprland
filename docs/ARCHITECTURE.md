# snry-daemon Architecture

snry-daemon is the central manager for the snry-shell desktop environment. It handles
installation, configuration, runtime services, and system management — replacing the
previous Ansible-based setup.

## Directory Structure

```
snry-shell-qs/
├── cmd/
│   └── snry-daemon/           # CLI entrypoint
│       └── main.go
├── internal/
│   ├── app/                   # Application orchestration (daemon mode)
│   │   ├── app.go
│   │   └── commands.go
│   ├── daemon/                # Runtime services (existing daemon features)
│   │   ├── brightness/
│   │   ├── cliphist/
│   │   ├── hyprland/
│   │   ├── idle/
│   │   │   ├── bus.go
│   │   │   ├── dbusutil/
│   │   │   ├── idle.go
│   │   │   ├── protocol/
│   │   │   └── screensaver.go
│   │   ├── inputmethod/
│   │   ├── lock/
│   │   ├── lockscreen/
│   │   ├── powersave/
│   │   ├── quickshell/
│   │   ├── resources/
│   │   ├── socket/
│   │   ├── tabletmode/
│   │   ├── uinput/
│   │   ├── updates/
│   │   └── weather/
│   ├── manager/               # Installation & config management (replaces Ansible)
│   │   ├── manager.go         # Manager interface + orchestrator
│   │   ├── config.go          # TOML config loading (replaces group_vars/all.yml)
│   │   ├── filesync.go        # File sync engine (replaces ansible synchronize)
│   │   ├── diagnose.go        # System diagnostics
│   │   ├── autoscale.go       # Monitor autoscale
│   │   ├── checkdeps.go       # Dependency checking
│   │   ├── steps.go           # Step runner with progress reporting
│   │   ├── arch/              # Arch Linux implementation
│   │   │   ├── deps.go        # Package installation via paru
│   │   │   ├── microtex.go    # MicroTeX AUR build
│   │   │   └── checkdeps.go   # AUR package existence check
│   │   ├── fedora/            # Fedora implementation
│   │   │   └── deps.go        # Package installation via dnf + COPR
│   │   └── setup.go           # System setup (groups, systemd, PAM, etc.)
│   ├── platform/              # OS detection + shared platform utilities
│   │   ├── detect.go          # Distro detection
│   │   └── exec.go            # Privileged command execution helpers
│   └── xdg/                   # XDG base directory resolution
│       └── xdg.go
├── frontend/                  # Quickshell QML/UI (moved from files/.config/quickshell/)
│   └── ii/
│       ├── shell.qml
│       ├── modules/
│       ├── services/
│       └── ...
├── data/                      # Package manifests (kept, read by manager)
│   ├── arch/
│   │   ├── packages.conf
│   │   └── microtex-git/
│   │       └── PKGBUILD
│   ├── fedora/
│   │   ├── feddeps.toml
│   │   └── SPECS/
│   └── python/
│       ├── requirements.in
│       └── requirements.txt
├── configs/                   # Config templates (moved from files/ + files-extra/)
│   ├── quickshell/            # → synced to $XDG_CONFIG_HOME/quickshell/
│   ├── hypr/                  # → synced to $XDG_CONFIG_HOME/hypr/
│   ├── bash/                  # → synced to $XDG_CONFIG_HOME/bash/
│   ├── fontconfig/            # → synced to $XDG_CONFIG_HOME/fontconfig/
│   ├── fuzzel/
│   ├── wlogout/
│   ├── kvantum/
│   ├── starship.toml
│   ├── konsole/
│   ├── portal/
│   ├── hyprland-entries/      # hyprland.conf, hyprlock.conf, etc.
│   ├── fedora/
│   ├── fontsets/
│   ├── swaylock/
│   ├── fcitx5/
│   ├── zshrc.d/
│   └── icons/
├── scripts/                   # Shell scripts used by quickshell modules
│   └── osk-watcher/
├── go.mod
├── go.sum
├── PKGBUILD
├── LICENSE
└── docs/
```

## CLI Subcommands

```
snry-daemon              # Start daemon (default, no subcommand)
snry-daemon daemon       # Explicit daemon start
snry-daemon setup        # Full installation (deps + files + setups)
snry-daemon deps         # Install packages only
snry-daemon files        # Sync config files only
snry-daemon setups       # System setup only (groups, systemd, PAM)
snry-daemon diagnose     # Collect system diagnostics
snry-daemon checkdeps    # Check missing packages
snry-daemon autoscale    # Auto-set monitor scale
snry-daemon uninstall    # Remove installed files and revert changes
snry-daemon send <cmd>   # Send command to running daemon
```

## Manager Architecture

### Interface Design

```go
// internal/manager/manager.go

// PackageManager handles OS-specific package installation.
type PackageManager interface {
    // InstallPackages installs the listed packages.
    InstallPackages(ctx context.Context, packages []string) error
    // InstallBuildDeps installs build dependencies.
    InstallBuildDeps(ctx context.Context) error
    // UpdateSystem performs a full system upgrade.
    UpdateSystem(ctx context.Context) error
    // CheckPackages returns packages not available in any repo.
    CheckPackages(ctx context.Context, packages []string) ([]string, error)
    // Distro returns the detected distro family.
    Distro() distro.Family
}

// Step represents a single setup operation with progress reporting.
type Step struct {
    Name     string
    Fn       func(ctx context.Context) error
    Optional bool
}
```

### Privilege Escalation

The manager runs as the current user. Privileged operations use `sudo` internally:

```go
// internal/platform/exec.go

func SudoCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
    cmd := exec.CommandContext(ctx, "sudo", append([]string{name}, args...)...)
    cmd.Stdin = os.Stdin // allows password prompt
    return cmd
}
```

Sudo credentials are cached per-invocation (sudo's default `timestamp_timeout`).
The setup flow prompts for sudo once at the beginning for system-level operations.

### File Sync Engine

Replaces `ansible.builtin.synchronize` with a Go-native rsync wrapper:

```go
// internal/manager/filesync.go

type SyncOptions struct {
    Src      string
    Dst      string
    Delete   bool   // --delete
    Excludes []string
}

func SyncDirectory(ctx context.Context, opts SyncOptions) error {
    // Uses rsync if available, falls back to filepath.Walk + io.Copy
}
```

### Config Loading

Replaces `group_vars/all.yml` with a Go-native config:

```go
// internal/manager/config.go

type Config struct {
    // Skip flags
    SkipSysUpdate   bool
    SkipQuickshell  bool
    SkipHyprland    bool
    SkipBash        bool
    SkipFontconfig  bool
    SkipMiscConf    bool
    SkipBackup      bool
    Force           bool

    // Fontset override
    FontsetDirName string

    // Paths (resolved from XDG env vars)
    XDG  xdg.Paths
    Home string
}
```

### Step Runner

Each setup phase is broken into steps with progress reporting:

```go
// internal/manager/steps.go

type StepResult struct {
    Name    string
    Skipped bool
    Err     error
}

type ProgressFunc func(step string, current, total int)

func RunSteps(ctx context.Context, steps []Step, progress ProgressFunc) []StepResult {
    results := make([]StepResult, 0, len(steps))
    for i, step := range steps {
        select {
        case <-ctx.Done():
            return results
        default:
        }
        progress(step.Name, i+1, len(steps))
        err := step.Fn(ctx)
        if err != nil && !step.Optional {
            return results
        }
        results = append(results, StepResult{Name: step.Name, Err: err})
    }
    return results
}
```

## Memory Management Patterns

1. **Context propagation**: Every operation accepts `context.Context` as first param.
   Cancellation stops in-flight operations and cleans up resources.

2. **No goroutine leaks**: Every goroutine has a `select { case <-ctx.Done(): return }`
   exit path. Use `errgroup` for concurrent operations.

3. **Resource cleanup**: Use `defer` for file handles, network connections, temp files.
   Temp directories use `t.TempDir()` in tests.

4. **No mutable globals**: All state is in structs, injected via constructors.

5. **Proper error wrapping**: Use `fmt.Errorf("operation: %w", err)`.
   Errors are returned, not logged and returned.

6. **sync.Pool for buffers**: Reuse `bytes.Buffer` in hot paths (socket server).

7. **Preallocate slices**: Always `make([]T, 0, n)` when size is known.

## Error Handling

- Return errors, do not panic.
- Wrap with context: `fmt.Errorf("install packages: %w", err)`
- Sentinel errors for expected conditions:
  ```go
  var ErrUnsupportedDistro = errors.New("unsupported distribution")
  var ErrDaemonRunning     = errors.New("daemon already running")
  ```
- Handle errors once: either log+degrade or wrap+return, never both.
- Use `errors.Join` for multi-operation failures.

## Migration Path

1. **Phase 1**: Implement manager package alongside existing Ansible. Both coexist.
2. **Phase 2**: Switch `snry-shell` wrapper to call `snry-daemon setup` instead of Ansible.
3. **Phase 3**: Remove Ansible files (playbooks, roles, ansible.cfg, requirements.yml).
4. **Phase 4**: Update PKGBUILD to remove `ansible-core` dependency.

## PKGBUILD Changes

```bash
# Old depends
depends=('ansible-core' 'git' 'rsync' 'python' 'uv' 'sudo' 'findutils' 'which')

# New depends
depends=('git' 'rsync' 'python' 'uv' 'sudo' 'findutils')
makedepends=('go')

# snry-shell wrapper becomes:
install -Dm755 /dev/stdin "$pkgdir/usr/bin/snry-shell" <<'SCRIPT'
#!/bin/bash
exec /usr/bin/snry-daemon setup "$@"
SCRIPT
```
