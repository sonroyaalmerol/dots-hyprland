#!/bin/bash
# Only turn off screen if the session is actually locked
# Uses stamp file created by hypridle lock_cmd and removed by on_unlock_cmd
if [ -f "$XDG_RUNTIME_DIR/snry-locked" ]; then
    hyprctl dispatch dpms off
fi
