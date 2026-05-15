-- Default variables
-- Override in ~/.config/hypr/snry-override.lua

-- Apps (try each in order until one is found)
terminal = "snry send launch ghostty foot alacritty wezterm konsole kgx uxterm xterm"
fileManager = "snry send launch dolphin nautilus nemo thunar 'ghostty -e bash -c yazi'"
browser = "snry send launch google-chrome-stable zen-browser firefox brave chromium microsoft-edge-stable opera librewolf"
codeEditor = "snry send launch antigravity code codium cursor zed zedit zeditor kate gnome-text-editor emacs 'command -v nvim && ghostty -e nvim' 'command -v micro && ghostty -e micro'"
officeSoftware = "snry send launch wps onlyoffice-desktopeditors libreoffice"
textEditor = "snry send launch kate gnome-text-editor emacs"
volumeMixer = "snry send launch pavucontrol-qt pavucontrol"
settingsApp = "snry send launch 'XDG_CURRENT_DESKTOP=gnome qs -p /usr/share/snry-shell/frontend/ii/settings.qml' 'XDG_CURRENT_DESKTOP=gnome systemsettings' 'XDG_CURRENT_DESKTOP=gnome gnome-control-center' 'XDG_CURRENT_DESKTOP=gnome better-control'"
taskManager = "snry send launch gnome-system-monitor 'plasma-systemmonitor --page-name Processes' 'command -v btop && ghostty -e btop'"

-- Quickshell config path for IPC and script references
qsConfig = "/usr/share/snry-shell/frontend/ii"

-- Leave blank/false to load default config. Set to true to skip.
dontLoadDefaultExecs = false
dontLoadDefaultGeneral = false
dontLoadDefaultRules = false
dontLoadDefaultColors = false
dontLoadDefaultKeybinds = false
