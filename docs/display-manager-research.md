# Display Manager Research for snry-daemon

## How existing DMs work

### Architecture (greetd as reference model)

All Linux display managers follow the same core pattern:

```
systemd (pid 1)
  └─ greetd.service (root, WantedBy=graphical.target)
       ├─ VT management: opens /dev/tty1, sets KD_GRAPHICS mode
       ├─ PAM session: pam_start → pam_authenticate → pam_open_session
       ├─ UID switch: fork+setuid to authenticated user
       ├─ exec compositor/desktop as that user
       └─ register session with logind via D-Bus
```

**Key operations that require root:**

1. **VT management** — `open(/dev/tty1)`, `ioctl(KDSETMODE, KD_GRAPHICS)`, `ioctl(VT_ACTIVATE)` — needs `CAP_SYS_TTY_CONFIG` or root
2. **PAM authentication** — `pam_start()`, `pam_authenticate()`, `pam_open_session()` — needs root to read `/etc/shadow`
3. **Session registration** — `sd_pid_notify()` with `READY=1` to logind — needs root or `CAP_SYSLOG`
4. **UID switch** — `fork()` + `setuid()` — needs root to switch to arbitrary user

### greetd's approach (Rust, ~3k LOC)

greetd splits into two processes:

1. **greetd server** (root) — manages VT, accepts greeter IPC, spawns session workers
2. **session worker** (`greetd --session-worker`) — child process that does PAM auth + UID switch

The greeter talks to greetd over a Unix socket (`GREETD_SOCK`) using a simple JSON IPC protocol:

- `CreateSession` → PAM conversation begins
- `PostAuthMessageResponse` → answer PAM questions (password, etc.)
- `StartSession` → launch the compositor

### Canonical's authd approach (Go)

authd uses a **hybrid**: a tiny C PAM module (`go-exec/module.c`) that `exec`s a Go binary, which then talks to the authd daemon over a private D-Bus connection. This avoids CGO for the main Go code while still using libpam (via the C shim).

## PAM in Go

| Library                         | CGO?                            | Notes                                                                                                                                                                                  |
| ------------------------------- | ------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `github.com/msteinert/pam` v2   | **Yes** (`#cgo LDFLAGS: -lpam`) | Most mature, wraps libpam via CGO. Used by authd.                                                                                                                                      |
| `github.com/squat/pam`          | **Yes** (`-buildmode=c-shared`) | For writing PAM _modules_ in Go, not for apps. Wrong direction.                                                                                                                        |
| `github.com/netauth/pam-helper` | **No** (pure Go)                | Uses `pam_exec.so` — pipes stdin/stdout to a Go binary. Not a library, but an architecture pattern.                                                                                    |
| Pure Go reimplementation        | No                              | No one has done this. PAM is an API spec around `/etc/pam.d/` config + pluggable shared objects. Reimplementing means reimplementing libpam + all module loading. Massive undertaking. |

**Conclusion: There is no pure-Go PAM library.** All Go PAM bindings use CGO.

## Options for snry-daemon

### Option 1: pam-exec helper (pure Go, no CGO) ⭐ RECOMMENDED

Use `pam_exec.so` (ships with every Linux distro's PAM) to call a small Go binary for authentication:

```
/etc/pam.d/snry-dm:
  auth requisite pam_exec.so expose_authtok /usr/bin/snry-pam-auth
  account requisite pam_exec.so /usr/bin/snry-pam-auth
```

`snry-pam-auth` is a tiny Go binary that:

1. Reads username from `PAM_USER` env var
2. Reads password from stdin (via `expose_authtok`)
3. Verifies against `/etc/shadow` (or via `passwd` check)
4. Exits 0 on success, non-zero on failure

**Pros:** Pure Go, no CGO, works everywhere PAM exists
**Cons:** Less flexible than direct PAM (can't do PAM conversations for 2FA, etc.)

But for our use case (auto-login to Hyprland), we don't need full PAM conversation support. We just need `pam_open_session()` for proper logind session registration.

### Option 2: CGO PAM binding (msteinert/pam v2)

Use `github.com/msteinert/pam/v2` for direct PAM access:

```go
tx, _ := pam.StartFunc("snry-dm", username, func(style pam.Style, msg string) (string, error) {
    switch style {
    case pam.PromptEchoOff: return getPassword(), nil
    default: return "", nil
    }
})
tx.Authenticate(0)
tx.OpenSession(0)
```

**Pros:** Full PAM API, proper session registration, supports 2FA/etc
**Cons:** Requires CGO, adds `libpam` dependency, cross-compilation complexity

### Option 3: greetd-compatible (use greetd as the DM, snry-daemon as greeter)

Don't reinvent the DM. Instead:

1. Install `greetd` as the system service (root, runs on tty1)
2. `snry-daemon` acts as a **greetd greeter** via the IPC protocol
3. After auth, greetd launches `snry-daemon daemon` as the user
4. snry-daemon then launches Hyprland

**Pros:** Zero root code in snry-daemon, proper PAM via greetd, battle-tested DM behavior
**Cons:** Hard dependency on greetd, less control over the auth flow

### Option 4: Full DM (root service) with CGO PAM

Build the complete DM as described — root system service that does VT management, PAM auth, UID switch, compositor launch.

**Pros:** Complete control, no external dependencies
**Cons:** Most complex, requires CGO, root-running Go code, biggest security surface

## VT Management (required for all DM approaches)

Regardless of PAM strategy, a DM needs to manage VTs. This is pure Go via syscalls:

```go
import "golang.org/x/sys/unix"

// Open and configure a VT
fd, _ := unix.Open("/dev/tty1", unix.O_RDWR|unix.O_NOCTTY, 0)
// Set graphics mode (hide text console)
unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.KDSETMODE, unix.KD_GRAPHICS)
// Switch to this VT
unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.VT_ACTIVATE, 1)
// Later: restore text mode
unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.KDSETMODE, unix.KD_TEXT)
```

This needs `CAP_SYS_TTY_CONFIG` or root.

## Session Registration with logind

After PAM auth + UID switch, the session must be registered with logind:

```go
// Via D-Bus (already using godbus in snry-daemon)
// org.freedesktop.login1.Manager.CreateSession(...)
// This is what pam_systemd does internally during pam_open_session
```

Or simply rely on `pam_open_session()` which calls `pam_systemd` which registers with logind.

## Recommended Architecture

```
┌─────────────────────────────────────────────────┐
│  snry-dm.service (root, system service)         │
│  WantedBy=graphical.target                      │
│                                                  │
│  1. Open /dev/tty1, set KD_GRAPHICS             │
│  2. Start greeter (snry-daemon greeter-mode)    │
│  3. Wait for auth over Unix socket               │
│  4. PAM auth (via msteinert/pam or pam_exec)    │
│  5. pam_open_session → registers with logind    │
│  6. fork + setuid to authenticated user          │
│  7. exec snry-daemon daemon                      │
│  8. Wait for session end, then restart greeter   │
│                                                  │
│  compositor.Launch() is called by the daemon     │
│  after the DM has already started it, so it     │
│  becomes a no-op if HYPRLAND_INSTANCE_SIGNATURE  │
│  is already set.                                 │
└─────────────────────────────────────────────────┘
```

This is essentially the greetd pattern, but implemented in Go within snry-daemon itself.

## Minimal Viable DM (Phase 1)

For the first iteration, keep it simple:

1. **System service** (`snry-dm.service`) runs as root
2. **VT management** — open tty1, set graphics mode (pure Go, golang.org/x/sys/unix)
3. **Auto-login only** — skip PAM auth for now, directly `setuid` to the configured user and exec `snry-daemon daemon`
4. **Session registration** — use `pam_open_session` via CGO binding OR via `systemd-run --user` to register with logind
5. **Greeter** — later phase; can use Quickshell UI

This gives us the compositor-launcher DM behavior immediately, with PAM auth added in phase 2.

## Phase 2: PAM Authentication

Add PAM authentication with `github.com/msteinert/pam/v2` (CGO):

```go
func authenticate(username, password string) error {
    tx, err := pam.StartFunc("snry-dm", username, func(style pam.Style, msg string) (string, error) {
        if style == pam.PromptEchoOff {
            return password, nil
        }
        return "", nil
    })
    if err != nil {
        return err
    }
    defer tx.End()

    if err := tx.Authenticate(0); err != nil {
        return err
    }
    if err := tx.AcctMgmt(0); err != nil {
        return err
    }
    if err := tx.OpenSession(0); err != nil {
        return err
    }
    return nil
}
```

## Key Dependencies

| Dependency                                                           | Purpose                     | Pure Go?                 |
| -------------------------------------------------------------------- | --------------------------- | ------------------------ |
| `golang.org/x/sys/unix`                                              | VT management, ioctl        | Yes                      |
| `github.com/godbus/dbus/v5`                                          | logind session registration | Yes (already in project) |
| `github.com/msteinert/pam/v2`                                        | PAM auth + session          | **No (CGO)**             |
| `github.com/sonroyaalmerol/snry-shell-qs/internal/daemon/compositor` | Hyprland launch             | Yes (already done)       |

## Files to Create

1. `internal/daemon/displaymanager/` — DM core (VT mgmt, PAM, uid switch, session lifecycle)
2. `cmd/snry-dm/main.go` — root binary entry point (system service)
3. `configs/systemd/system/snry-dm.service` — systemd unit file
4. `configs/pam.d/snry-dm` — PAM service config

## Comparison: greetd vs snry-dm

| Aspect               | greetd                              | snry-dm (proposed)              |
| -------------------- | ----------------------------------- | ------------------------------- |
| Language             | Rust                                | Go                              |
| PAM                  | pam-sys (FFI)                       | msteinert/pam (CGO) or pam_exec |
| Greeter              | External (tuigreet, gtkgreet, etc.) | Built-in (Quickshell UI)        |
| Compositor launch    | Via greeter IPC                     | Direct (compositor.Launch)      |
| VT management        | nix crate                           | golang.org/x/sys/unix           |
| Session registration | pam_open_session → pam_systemd      | Same                            |
| Auto-login           | Config file                         | Config file or default          |
| IPC protocol         | JSON over Unix socket               | Unix socket (already have one)  |
