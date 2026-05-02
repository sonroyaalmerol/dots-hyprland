<div align="center">
    <h1>snry Shell</h1>
    <h3>A Hyprland desktop shell built with Quickshell and Ansible</h3>
</div>

<div align="center">

![](https://img.shields.io/github/last-commit/sonroyaalmerol/dots-hyprland?&style=for-the-badge&color=8ad7eb&logo=git&logoColor=D9E0EE&labelColor=1E202B)
![](https://img.shields.io/github/stars/sonroyaalmerol/dots-hyprland?style=for-the-badge&logo=andela&color=86dbd7&logoColor=D9E0EE&labelColor=1E202B)
![](https://img.shields.io/github/repo-size/sonroyaalmerol/dots-hyprland?color=86dbce&label=SIZE&logo=protondrive&style=for-the-badge&logoColor=D9E0EE&labelColor=1E202B)

</div>

<div align="center">
    <img src="assets/snry-shell.svg" alt="snry-shell logo" width="400">
</div>

<div align="center">
    <h2>• overview •</h2>
</div>

**snry Shell** is a desktop shell for [Hyprland](https://hyprland.org/), built with [Quickshell](https://quickshell.outfoxxed.me/) and deployed via [Ansible](https://www.ansible.com/). It provides a polished, Material-themed desktop experience with live window previews, AI integration, screen translation, and more — all configurable through a GUI settings app.

<div align="center">
    <h2>• features •</h2>
</div>

- **Overview** — live window previews with full app grid
- **AI sidebar** — integrated AI assistant panel
- **Screen translation** — on-screen OCR + translate
- **Anti-flashbang** — automatic brightness management
- **Material theming** — wallpaper-driven color generation via matugen
- **Settings app** — GUI for shell configuration
- **Dual panel families** — "ii" (default) and "Waffle" styles, switchable live with `Super+Alt+W`
- **Transparent install** — Ansible-driven, every command shown before execution

<div align="center">
    <h2>• installation •</h2>
</div>

### AUR (Arch Linux)

```bash
paru -S snry-shell-qs
snry-shell
```

### Manual

```bash
git clone https://github.com/sonroyaalmerol/dots-hyprland.git
cd dots-hyprland
ansible-playbook setup.yml
```

### CLI commands

| Command | Purpose |
|---------|---------|
| `snry-shell` | Install / update shell |
| `snry-shell uninstall` | Remove shell configuration |
| `snry-shell diagnose` | Run diagnostics |
| `snry-shell checkdeps` | Check dependency status |

<div align="center">
    <h2>• keybinds •</h2>
</div>

| Keybind | Action |
|---------|--------|
| `Super` + `/` | Show keybind list |
| `Super` + `Enter` | Open terminal |
| `Super` + `Alt` + `W` | Switch panel family (ii ↔ Waffle) |

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
| [Ansible](https://github.com/ansible/ansible) | Configuration management and deployment |
| [Go](https://go.dev/) (snry-daemon) | System-level daemon for IPC and automation |
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

[![Stargazers over time](https://starchart.cc/sonroyaalmerol/dots-hyprland.svg?variant=adaptive)](https://starchart.cc/sonroyaalmerol/dots-hyprland)
