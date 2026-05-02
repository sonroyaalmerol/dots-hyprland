# Contributing to snry Shell

Thanks for your interest in contributing to snry Shell! This document covers code style, setup instructions, and the contribution workflow.

## Pull Requests

- **One feature or fix per PR.** Don't bundle unrelated changes together.
- Don't include personal config changes or altered defaults in your PR.
- We'll happily accept features we don't personally use — just make them configurable and optionally loaded (off by default if impractical for daily use).
- Before starting something big, ask first. If you've already built it for yourself, submit it anyway — no harm in that.

## Translations

See `files/.config/quickshell/ii/translations/tools` for the translation management tool suite. Refer to `translations/tools/README.md` for full documentation.

## Code Style

### Dynamic loading

- If a component isn't always needed (especially when guarded by a config toggle), wrap it in a `Loader`.
  - Declare positioning properties (like `anchors`) in the `Loader`, not the `sourceComponent`.
  - For components that don't affect parent layout, use `FadeLoader` and its `shown` property instead of toggling `active` and `visible`.

### Practical concerns

- Don't add anything that drains significant resources for a minor visual effect. snry Shell must remain practical for daily driving.
- If a feature is flashy but impractical, make it configurable and **disabled by default** (e.g., a constantly rotating background clock).

### Formatting

- Use **spaces** for indentation.
- Space around operators and keywords: `if (condition) { ... } else { ... }` — not `if(condition){...}else{...}`.
- Group spacing: space properties and children into meaningful blocks, but don't use 2+ blank lines in a row.
- Keep nesting shallow where possible:
  - Prefer early returns: `if (!condition) return; doStuff();`
  - Use `component` declarations to extract reusable pieces without leaving the file.

## Development Setup

These instructions assume an Arch(-based) Linux system.

### Full install

_Safest — gives you a working environment matching production._

```bash
paru -S snry-shell-qs    # or: ansible-playbook setup.yml
# (use a new user account if you don't want to overwrite your existing config)
```

Make your changes, push to a fork, and open a PR.

### Partial shell

_Most shell features work, but not all._

```bash
paru -S hyprland quickshell-git
cp -r files/.config/quickshell ~/
```

### Quickshell LSP setup

```bash
touch ~/.config/quickshell/ii/.qmlls.ini
```

**VS Code:** Install the official "Qt Qml" extension and set its custom executable path to `/usr/bin/qmlls6`.

### Python

If your changes involve Python packages or scripts, use the `uv` virtual environment as described in `data/python/README.md`.

## Running

1. Launch Hyprland (not the uwsm-managed variant).
2. Open `~/.config/quickshell/ii` in your editor.
3. Start the shell in a terminal for live logs:
   ```bash
   pkill qs; qs -c ii
   ```
4. Edit files in the opened folder — changes reload live.
