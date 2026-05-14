# Auto start Hyprland on tty1 (fallback for interactive zsh sessions)
# The canonical auto-start is in ~/.profile (shell-agnostic).
if [ -z "${WAYLAND_DISPLAY}" ] && [ "${XDG_VTNR}" -eq 1 ] && [ -z "${HYPRLAND_INSTANCE_SIGNATURE}" ]; then
  mkdir -p ~/.cache
  exec start-hyprland > ~/.cache/hyprland.log 2>&1
fi
