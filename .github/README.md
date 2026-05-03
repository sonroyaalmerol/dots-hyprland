<div align="center">
    <h1>snry Shell</h1>
    <h3>A Hyprland desktop shell built with Quickshell and Go</h3>
</div>

<div align="center">

![](https://img.shields.io/github/last-commit/sonroyaalmerol/snry-shell?&style=for-the-badge&color=8ad7eb&logo=git&logoColor=D9E0EE&labelColor=1E202B)
![](https://img.shields.io/github/stars/sonroyaalmerol/snry-shell?style=for-the-badge&logo=andela&color=86dbd7&logoColor=D9E0EE&labelColor=1E202B)
![](https://img.shields.io/github/repo-size/sonroyaalmerol/snry-shell?color=86dbce&label=SIZE&logo=protondrive&style=for-the-badge&logoColor=D9E0EE&labelColor=1E202B)

</div>

<div align="center">
    <img src="assets/snry-shell.svg" alt="snry-shell logo" width="400">
</div>

<div align="center">
    <h2>• overview •</h2>
</div>

**snry Shell** is a desktop shell for [Hyprland](https://hyprland.org/), built with [Quickshell](https://quickshell.outfoxxed.me/) and managed by a native Go daemon. It provides a polished, Material-themed desktop experience with live window previews, AI integration, screen translation, smart config sync, and more — all configurable through a GUI settings app.

<div align="center">
    <h2>• features •</h2>
</div>

- **Overview** — live window previews with full app grid
- **AI sidebar** — integrated AI assistant panel
- **Screen translation** — on-screen OCR + translate
- **Anti-flashbang** — automatic brightness management
- **Material theming** — wallpaper-driven color generation via matugen
- **Settings app** — GUI for shell configuration
- **Smart config sync** — three-way merge engine preserves user customizations across updates
- **Hands-off install** — AUR package handles everything automatically
- **Zero Ansible** — all setup, config deployment, and diagnostics handled by snry-daemon (Go)

<div align="center">
    <h2>• installation •</h2>
</div>

### AUR (Arch Linux)

```bash
paru -S snry-shell-qs
```

That's it. The `post_install` hook runs system setup as root and deploys configs as your user automatically.

### Manual

```bash
git clone https://github.com/sonroyaalmerol/snry-shell.git
cd snry-shell
go build -o ~/.local/bin/snry-daemon ./cmd/snry-daemon
snry-daemon setup
```

### CLI commands

| Command | Purpose |
|---------|---------|
| `snry-daemon setup` | Full install: deps + config sync + system setup |
| `snry-daemon files` | Smart-sync config files (three-way merge) |
| `snry-daemon deps` | Install packages only |
| `snry-daemon setups` | System setup (groups, systemd, PAM) |
| `snry-daemon diagnose` | Run diagnostics |
| `snry-daemon checkdeps` | Check dependency status |
| `snry-daemon autoscale` | Auto-set monitor scale |
| `snry-daemon uninstall` | Remove shell configuration |
| `snry-daemon daemon` | Start runtime daemon (socket server) |

<div align="center">
    <h2>• smart config sync •</h2>
</div>

snry-daemon includes a smart config sync engine that handles updates intelligently:

| Strategy | For files | Behavior |
|----------|-----------|----------|
| `overwrite` | SVGs, PNGs, fonts, quickshell QML | Always replace with upstream |
| `merge-hyprland` | `hypr/hyprland/*.conf` | Section-aware merge (key-values + binds) |
| `merge-kv` | hyprlock.conf, fuzzel.ini, `*.conf`/`*.ini` | Key-value level three-way merge |
| `merge-section` | bashrc, bash_profile | Merge only between `# >>> snry-shell >>>` markers |
| `skip-if-exists` | monitors.conf, workspaces.conf | Only deploy on first install |
| `template` | Files with `{{.User}}` etc. | Render variables, then merge |

See [`docs/SMART_SYNC.md`](docs/SMART_SYNC.md) for the full design.

<div align="center">
    <h2>• keybinds •</h2>
</div>

| Keybind | Action |
|---------|--------|
| `Super` + `/` | Show keybind list |
| `Super` + `Enter` | Open terminal |

> Full keybind list is available in-app via `Super+/`.

<div align="center">
    <h2>• screenshots •</h2>
</div>

[Showcase video](https://www.youtube.com/watch?v=RPwovTInagE)

| AI, settings app                                                                                                                     | Some widgets                                                                                                                         |
| :----------------------------------------------------------------------------------------------------------------------------------- | :----------------------------------------------------------------------------------------------------------------------------------- |
| <img width="1920" height="1080" alt="image" src="https://github.com/user-attachments/assets/5d4e7d07-d0b4-4406-a4c9-ed7ba90e3fe4" /> | <img width="1920" height="1080" alt="image" src="https://github.com/user-attachments/assets/6a32395f-9437-4192-8faf-2951a9e84cbe" /> |
| Window management                                                                                                                    | Material theming                                                                                                                     |
| <img width="1920" height="1080" alt="image" src="https://github.com/user-attachments/assets/c51bed8b-3670-4d4c-9074-873be224fb8e" /> | <img width="1920" height="1080" alt="image" src="https://github.com/user-attachments/assets/98703a66-0743-439f-a721-cef7afa6ab95" /> |

<div align="center">
    <h2>• tech stack •</h2>
</div>

| Software | Purpose |
|----------|---------|
| [Hyprland](https://github.com/hyprwm/hyprland) | Wayland compositor — window management and rendering |
| [Quickshell](https://quickshell.outfoxxed.me/) | QtQuick-based widget system — bar, panels, overlays |
| [Go](https://go.dev/) (snry-daemon) | Central manager: install, config sync, diagnostics, runtime daemon |
| [Python](https://www.python.org/) | Scripts, translation tools |

<div align="center">
    <h2>• contributing •</h2>
</div>

Contributions are welcome! Please see [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines.

<div align="center">
    <h2>• credits •</h2>
</div>

This project was originally forked from [@end-4](https://github.com/end-4)'s [dots-hyprland](https://github.com/end-4/dots-hyprland) and has since diverged into an independent project.

Thanks to:
- [@clsty](https://github.com/clsty) — install scripting and packaging
- [@midn8hustlr](https://github.com/midn8hustlr) — color generation system improvements
- [@outfoxxed](https://github.com/outfoxxed) — Quickshell development and support
- Quickshell community: [Soramane](https://github.com/caelestia-dots/shell/), [FridayFaerie](https://github.com/FridayFaerie/quickshell), [nydragon](https://github.com/nydragon/nysh)

<div align="center">
    <h2>• stargazers •</h2>
</div>

[![Stargazers over time](https://starchart.cc/sonroyaalmerol/snry-shell.svg?variant=adaptive)](https://starchart.cc/sonroyaalmerol/snry-shell)
