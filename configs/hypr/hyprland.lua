-- Managed by snry-shell. Do not edit files in hyprland/ — they are auto-restored.
-- Add your overrides in custom/ — they will never be touched by the daemon.

require("hyprland.variables")
require("hyprland.env")
require("hyprland.execs")
require("hyprland.general")
require("hyprland.rules")
require("hyprland.colors")
require("hyprland.keybinds")

-- User overrides — these will never be managed by snry-shell
pcall(require, "custom.env")
pcall(require, "custom.variables")
pcall(require, "custom.execs")
pcall(require, "custom.general")
pcall(require, "custom.rules")
pcall(require, "custom.keybinds")

-- Auto-generated (do not remove)
pcall(require, "hyprland.monitors")
pcall(require, "hyprland.workspaces")

-- Shell overrides (managed)
pcall(require, "hyprland.shellOverrides.main")
