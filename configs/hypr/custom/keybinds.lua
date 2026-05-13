-- See https://wiki.hyprland.org/Configuring/Binds/

local mainMod = "SUPER"
local subMod = "SUPER_SHIFT"
local tetMod = "SUPER_CTRL"

hl.unbind(mainMod .. " + Super_L")
hl.unbind(mainMod .. " + Super_R")
hl.unbind(mainMod .. " + T")
hl.unbind("Ctrl + Alt + T")
hl.unbind(mainMod .. " + E")
hl.unbind(mainMod .. " + W")
hl.unbind("Ctrl + SUPER + Shift + Alt + W")
hl.unbind(mainMod .. " + X")
hl.unbind("Ctrl + SUPER + V")
hl.unbind(mainMod .. " + L")
hl.unbind(mainMod .. " + D")

hl.bind(mainMod .. " + D", hl.dsp.global("quickshell:searchToggleRelease"))

hl.bind(tetMod .. " + Slash", hl.dsp.exec_cmd("xdg-open ~/.config/snry-shell/config.json"), { description = "Edit shell config" })
hl.bind("Ctrl + SUPER + Alt + Slash", hl.dsp.exec_cmd("xdg-open ~/.config/hypr/custom/keybinds.lua"), { description = "Edit extra keybinds" })

hl.bind(mainMod .. " + L", hl.dsp.exec_cmd(os.getenv("HOME") .. "/.local/bin/snry-daemon send lock"), { description = "Lock" })

hl.bind(mainMod .. " + Q", hl.dsp.window.close())
hl.bind(subMod .. " + Q", hl.dsp.window.kill())
hl.bind(subMod .. " + E", hl.dsp.exit())
hl.bind(mainMod .. " + T", hl.dsp.exec_cmd("dolphin"))
hl.bind(mainMod .. " + V", hl.dsp.window.float())
hl.bind(mainMod .. " + mouse_down", hl.dsp.focus({ workspace = "e+1" }))
hl.bind(mainMod .. " + mouse_up", hl.dsp.focus({ workspace = "e-1" }))
hl.bind(mainMod .. " + W", hl.dsp.exec_cmd("swappnext"))
hl.bind(mainMod .. " + M", hl.dsp.window.fullscreen({ mode = "maximized" }))
hl.bind(subMod .. " + M", hl.dsp.window.fullscreen({ mode = "fullscreen" }))
hl.bind(mainMod .. " + F", hl.dsp.window.float())
hl.bind(mainMod .. " + 0", hl.dsp.focus({ workspace = 10 }))
hl.bind(subMod .. " + 0", hl.dsp.window.move({ workspace = 10 }))

hl.bind(subMod .. " + 1", hl.dsp.window.move({ workspace = 1 }))
hl.bind(subMod .. " + 2", hl.dsp.window.move({ workspace = 2 }))
hl.bind(subMod .. " + 3", hl.dsp.window.move({ workspace = 3 }))
hl.bind(subMod .. " + 4", hl.dsp.window.move({ workspace = 4 }))
hl.bind(subMod .. " + 5", hl.dsp.window.move({ workspace = 5 }))
hl.bind(subMod .. " + 6", hl.dsp.window.move({ workspace = 6 }))
hl.bind(subMod .. " + 7", hl.dsp.window.move({ workspace = 7 }))
hl.bind(subMod .. " + 8", hl.dsp.window.move({ workspace = 8 }))
hl.bind(subMod .. " + 9", hl.dsp.window.move({ workspace = 9 }))
hl.bind(subMod .. " + 0", hl.dsp.window.move({ workspace = 10 }))
-- Add stuff here
-- Use #! to add an extra column on the cheatsheet
-- Use ##! to add a section in that column
