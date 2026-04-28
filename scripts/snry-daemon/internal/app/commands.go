package app

import (
	"context"
	"strconv"
	"strings"
)

const maxKey = 248

func dispatchCommand(a *App, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}
	switch fields[0] {
	case "press":
		if len(fields) != 2 {
			return
		}
		code, err := strconv.ParseUint(fields[1], 10, 16)
		if err != nil || code > maxKey {
			return
		}
		a.uinput.Press(uint16(code))
	case "release":
		if len(fields) != 2 {
			return
		}
		code, err := strconv.ParseUint(fields[1], 10, 16)
		if err != nil || code > maxKey {
			return
		}
		a.uinput.Release(uint16(code))
	case "releaseall":
		a.uinput.ReleaseAll()
	case "combo":
		if len(fields) < 2 {
			return
		}
		var codes []uint16
		for i := 1; i < len(fields); i++ {
			code, err := strconv.ParseUint(fields[i], 10, 16)
			if err != nil || code > maxKey {
				return
			}
			codes = append(codes, uint16(code))
		}
		a.uinput.Combo(codes)
	case "auth":
		if len(fields) < 2 || a.lockscreenSvc == nil {
			return
		}
		password := strings.Join(fields[1:], " ")
		go a.lockscreenSvc.Authenticate(password)
	case "lock":
		if a.lockscreenSvc != nil {
			a.lockscreenSvc.Lock()
		} else if a.idleSvc != nil {
			a.idleSvc.Lock()
		}
	case "lock-startup":
		if a.lockscreenSvc != nil {
			a.lockscreenSvc.LockWithAutoUnlock()
		} else if a.idleSvc != nil {
			a.idleSvc.Lock()
		}
	case "unlock":
		if a.lockscreenSvc != nil {
			a.lockscreenSvc.Unlock()
		} else if a.idleSvc != nil {
			a.idleSvc.Unlock()
		}
	case "auto-unlock":
		if a.lockscreenSvc != nil {
			a.lockscreenSvc.TryAutoUnlock()
		} else if a.idleSvc != nil {
			a.idleSvc.Unlock()
		}
	case "power-button", "lid-close":
		if a.idleSvc != nil {
			a.idleSvc.SuppressDisplayOn(true)
		}
		if a.lockscreenSvc != nil {
			a.lockscreenSvc.Lock()
		}
		if a.idleSvc != nil {
			a.idleSvc.SetDisplay(false)
		}
	case "resources":
		if a.resourcesSvc != nil {
			a.resourcesSvc.EmitSnapshot(a.socketServer.Emitter().Emit)
		}
	case "weather-refresh":
		if a.weatherSvc != nil {
			go a.weatherSvc.RefreshNow(context.Background())
		}
	case "cliphist-list":
		if a.cliphistSvc != nil {
			go a.cliphistSvc.EmitList(context.Background())
		}
	case "cliphist-delete":
		if a.cliphistSvc != nil && len(fields) >= 2 {
			entry := strings.Join(fields[1:], " ")
			go a.cliphistSvc.DeleteEntry(context.Background(), entry)
		}
	case "cliphist-wipe":
		if a.cliphistSvc != nil {
			go a.cliphistSvc.Wipe(context.Background())
		}
	}
}
