#!/usr/bin/env bash
sleep 5

for _ in 1 2 3 4 5; do
  if pidof hyprlock >/dev/null 2>&1 || pidof qs >/dev/null 2>&1 || pidof quickshell >/dev/null 2>&1; then
    hyprctl dispatch dpms off && log "dpms off OK" || log "dpms off failed"
    exit 0
  fi
  sleep 1
done
log "Locker never detected; skipping"
