-- Default variables
-- Copy these to ~/.config/hypr/custom/variables.lua to make changes in a dotfiles-update-friendly manner

-- Apps (try each in order until one is found)
terminal = os.getenv("HOME") .. "/.local/bin/snry-daemon send launch ghostty foot alacritty wezterm konsole kgx uxterm xterm"
fileManager = os.getenv("HOME") .. "/.local/bin/snry-daemon send launch dolphin nautilus nemo thunar 'ghostty -e bash -c yazi'"
browser = os.getenv("HOME") .. "/.local/bin/snry-daemon send launch google-chrome-stable zen-browser firefox brave chromium microsoft-edge-stable opera librewolf"
codeEditor = os.getenv("HOME") .. "/.local/bin/snry-daemon send launch antigravity code codium cursor zed zedit zeditor kate gnome-text-editor emacs 'command -v nvim && ghostty -e nvim' 'command -v micro && ghostty -e micro'"
officeSoftware = os.getenv("HOME") .. "/.local/bin/snry-daemon send launch wps onlyoffice-desktopeditors libreoffice"
textEditor = os.getenv("HOME") .. "/.local/bin/snry-daemon send launch kate gnome-text-editor emacs"
volumeMixer = os.getenv("HOME") .. "/.local/bin/snry-daemon send launch pavucontrol-qt pavucontrol"
settingsApp = os.getenv("HOME") .. "/.local/bin/snry-daemon send launch 'XDG_CURRENT_DESKTOP=gnome qs -p ~/.config/quickshell/ii/settings.qml' 'XDG_CURRENT_DESKTOP=gnome systemsettings' 'XDG_CURRENT_DESKTOP=gnome gnome-control-center' 'XDG_CURRENT_DESKTOP=gnome better-control'"
taskManager = os.getenv("HOME") .. "/.local/bin/snry-daemon send launch gnome-system-monitor 'plasma-systemmonitor --page-name Processes' 'command -v btop && ghostty -e btop'"

-- The folder within ~/.config/quickshell containing the config
qsConfig = "ii"

-- Leave blank/false to load default config. Set to true to skip.
dontLoadDefaultExecs = false
dontLoadDefaultGeneral = false
dontLoadDefaultRules = false
dontLoadDefaultColors = false
dontLoadDefaultKeybinds = false
