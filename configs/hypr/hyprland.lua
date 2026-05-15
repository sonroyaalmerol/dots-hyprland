-- Managed by snry-shell. Do not edit files in hyprland/ — they are auto-restored.
-- Add your overrides in custom/ — they will never be touched by the daemon.

-- Layer 1: Base configuration (managed)
require("hyprland.variables")
require("hyprland.env")
require("hyprland.execs")
require("hyprland.general")
require("hyprland.rules")
require("hyprland.colors")
require("hyprland.keybinds")

-- Layer 2: Auto-generated configuration (do not remove)
pcall(require, "hyprland.monitors")
pcall(require, "hyprland.workspaces")

-- Layer 3: Shell overrides (managed by snry-shell)
pcall(require, "hyprland.shellOverrides.main")

-- Layer 4: User overrides — loaded LAST so they override everything above.
-- These files in custom/ will never be touched by the daemon.
pcall(require, "custom.env")
pcall(require, "custom.variables")
pcall(require, "custom.execs")
pcall(require, "custom.general")
pcall(require, "custom.rules")
pcall(require, "custom.keybinds")
