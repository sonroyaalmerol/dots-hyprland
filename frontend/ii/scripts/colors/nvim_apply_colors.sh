#!/usr/bin/env bash
# Converts material_colors.scss to JSON for neovim consumption
STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/quickshell"
SCSS_FILE="$STATE_DIR/user/generated/material_colors.scss"
NVIM_COLORS_FILE="$STATE_DIR/user/generated/nvim_colors.json"

if [ ! -f "$SCSS_FILE" ]; then
    echo "No material_colors.scss found"
    exit 0
fi

# Parse SCSS vars to JSON using python
python3 -c "
import json, re

colors = {}
with open('$SCSS_FILE') as f:
    for line in f:
        line = line.strip().rstrip(';')
        m = re.match(r'\\\$(\w+):\s*(.+)', line)
        if m:
            key = m.group(1)
            val = m.group(2).strip()
            if val == 'True':
                colors[key] = True
            elif val == 'False':
                colors[key] = False
            else:
                colors[key] = val

with open('$NVIM_COLORS_FILE', 'w') as f:
    json.dump(colors, f, indent=2)
"

# Signal all running neovim instances to reload colors
for pid in $(pgrep -x nvim); do
    kill -USR1 "$pid" 2>/dev/null || true
done
