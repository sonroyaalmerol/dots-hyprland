-- This file sources other modules in `hyprland` and `custom` folders
-- You wanna add your stuff in files in `custom`

-- Variables (must be loaded first as other modules reference them)
require("hyprland.variables")

-- Environment variables
require("hyprland.env")

-- Main config
if not dontLoadDefaultExecs then require("hyprland.execs") end
if not dontLoadDefaultGeneral then require("hyprland.general") end
if not dontLoadDefaultRules then require("hyprland.rules") end
if not dontLoadDefaultColors then require("hyprland.colors") end
if not dontLoadDefaultKeybinds then require("hyprland.keybinds") end

-- Custom overrides (optional, may not exist)
pcall(require, "custom.env")
pcall(require, "custom.variables")
pcall(require, "custom.execs")
pcall(require, "custom.general")
pcall(require, "custom.rules")
pcall(require, "custom.keybinds")

-- nwg-displays support
pcall(require, "workspaces")
pcall(require, "monitors")

-- Shell overrides
pcall(require, "hyprland.shellOverrides.main")
