-- Autostart applications

hl.on("hyprland.start", function()
	-- Bar, wallpaper
	hl.exec_cmd(snry send start-geoclue")
	-- QuickShell video wallpaper restore
	hl.exec_cmd(snry send restore-video-wallpaper")

	-- Core components
	hl.exec_cmd("gnome-keyring-daemon --start --components=secrets")
	hl.exec_cmd("/usr/lib/pam_kwallet_init")

	-- Display
	hl.exec_cmd("xrandr --output DP-1 --primary")

	-- Plugins: update headers, reload, then load hyprgrass for touchscreen gestures.
	-- hyprgrass addConfigKeyword/addConfigValue are broken on v0.55+ Lua config
	-- (awaiting upstream update for addConfigValueV2/addLuaFunction support).
	-- Native trackpad gestures are defined in general.lua via hl.gesture().
	hl.exec_cmd("hyprpm update && hyprpm reload -n")

	-- Hyprgrass touch-screen gestures (edge/tap/longpress).
	-- Only functional once hyprgrass adds v0.55+ Lua config support.
	if hl.plugin.touch_gestures ~= nil then
		hl.config({
			plugin = {
				touch_gestures = {
					sensitivity = 4.0,
					workspace_swipe_fingers = 3,
					workspace_swipe_edge = "l",
					long_press_delay = 400,
					resize_on_border_long_press = true,
					edge_margin = 10,
				},
			},
		})
	end
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
