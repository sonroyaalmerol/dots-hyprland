-- snry-shell system entry point
-- This file lives at /usr/share/snry-shell/configs/hypr/hyprland.lua
-- Hyprland sets package.path to this file's parent directory.
-- Layer 1 resolves to system files (read-only).
-- Layers 2-3 resolve to ~/.config/hypr/ (user-writable).

-- Layer 1: System defaults (from /usr/share/snry-shell/configs/hypr/)
require("hyprland.variables")
require("hyprland.env")
require("hyprland.execs")
require("hyprland.general")
require("hyprland.rules")
require("hyprland.colors")
require("hyprland.keybinds")

-- Layer 2-3: User config directory
local configDir = os.getenv("XDG_CONFIG_HOME") or (os.getenv("HOME") .. "/.config")
local userHyprDir = configDir .. "/hypr"

-- Prepend user hypr dir to package.path so require() finds user files first.
package.path = userHyprDir .. "/?.lua;" .. userHyprDir .. "/?/init.lua;" .. package.path

-- Layer 2: Auto-generated configuration (monitors, workspaces)
pcall(require, "monitors")
pcall(require, "workspaces")

-- Layer 3: User overrides — loaded LAST so they override everything above.
-- Place your overrides in ~/.config/hypr/snry-override.lua
-- You can require() additional files from there if needed.
pcall(require, "snry-override")
