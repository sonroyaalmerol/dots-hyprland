-- You can make apps auto-start here
-- Relevant Hyprland wiki section: https://wiki.hyprland.org/Configuring/Keywords/#executing

hl.on("hyprland.start", function()
	hl.exec_cmd("/usr/lib/pam_kwallet_init")
	hl.exec_cmd("xrandr --output DP-1 --primary")
	hl.exec_cmd("hyprpm reload -n")
end)
