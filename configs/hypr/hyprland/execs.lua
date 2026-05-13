-- Autostart applications

hl.on("hyprland.start", function()
	-- Bar, wallpaper
	hl.exec_cmd(os.getenv("HOME") .. "/.local/bin/snry-daemon send start-geoclue")
	-- QuickShell video wallpaper restore
	hl.exec_cmd(os.getenv("HOME") .. "/.config/hypr/custom/scripts/__restore_video_wallpaper.sh")

	-- Core components
	hl.exec_cmd("gnome-keyring-daemon --start --components=secrets")
	hl.exec_cmd(os.getenv("HOME") .. "/.local/bin/snry-daemon")

	-- Touch gesture plugin (hyprgrass)
	hl.exec_cmd("hyprctl plugin load ~/.local/lib/hyprland/plugins/libhyprgrass.so")
	hl.exec_cmd("dbus-update-activation-environment --all")
	hl.exec_cmd("sleep 1 && dbus-update-activation-environment --systemd WAYLAND_DISPLAY XDG_CURRENT_DESKTOP")

	-- Audio
	hl.exec_cmd("easyeffects --hide-window --service-mode")

	-- Clipboard: history
	hl.exec_cmd("wl-paste --type text --watch bash -c 'cliphist store && qs -c " .. qsConfig .. " ipc call cliphistService update'")
	hl.exec_cmd("wl-paste --type image --watch bash -c 'cliphist store && qs -c " .. qsConfig .. " ipc call cliphistService update'")

	-- Cursor
	hl.exec_cmd("hyprctl setcursor Bibata-Modern-Classic 24")
end)
