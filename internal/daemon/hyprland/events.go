package hyprland

import (
	"maps"
	"strconv"
	"strings"
)

// handleSocket2Event parses a socket2 event line and mutates the in-memory cache.
// After mutation, each handler calls emitDelta() with a targeted payload — never
// the full snapshot.  The frontend applies surgical QML property updates, so a
// single window title change sends ~80 bytes instead of the full state.
//
// Convention: snapshot under lock → unlock → emit.
func (s *Service) handleSocket2Event(eventName, data string) {
	switch eventName {

	// ── Workspaces ──────────────────────────────────────────────────────────

	case "workspacev2":
		id, name := split2(data)
		if id == "" {
			return
		}
		idn, _ := strconv.Atoi(id)
		s.mu.Lock()
		s.putActiveWorkspace(idn, name)
		s.upsertWorkspace(idn, name)
		snap := s.snapshotActiveWorkspace()
		s.mu.Unlock()
		s.emitDelta("hypr_active_workspace", map[string]any{
			"activeWorkspace": snap,
		})

	case "focusedmonv2":
		monName, wsIDStr := split2(data)
		if monName == "" {
			return
		}
		wsID, _ := strconv.Atoi(wsIDStr)
		s.mu.Lock()
		wsName := s.workspaceNameByID(wsID)
		s.putMonitorActiveWS(monName, wsID, wsName)
		s.putActiveWorkspaceFull(wsID, wsName, monName)
		snapAW := s.snapshotActiveWorkspace()
		snapMon := s.snapshotMonitorByName(monName)
		s.mu.Unlock()
		s.emitDelta("hypr_active_workspace", map[string]any{
			"activeWorkspace": snapAW,
		})
		if snapMon != nil {
			s.emitDelta("hypr_monitor_update", map[string]any{
				"monitor": snapMon,
			})
		}

	case "focusedmon":
		monName, wsName := split2(data)
		if monName == "" {
			return
		}
		wsID := s.lookupWorkspaceID(wsName)
		if wsID == 0 {
			return
		}
		s.mu.Lock()
		s.putMonitorActiveWS(monName, wsID, wsName)
		s.putActiveWorkspaceFull(wsID, wsName, monName)
		snapAW := s.snapshotActiveWorkspace()
		snapMon := s.snapshotMonitorByName(monName)
		s.mu.Unlock()
		s.emitDelta("hypr_active_workspace", map[string]any{
			"activeWorkspace": snapAW,
		})
		if snapMon != nil {
			s.emitDelta("hypr_monitor_update", map[string]any{
				"monitor": snapMon,
			})
		}

	case "createworkspacev2":
		id, name := split2(data)
		if id == "" {
			return
		}
		idn, _ := strconv.Atoi(id)
		s.mu.Lock()
		s.upsertWorkspace(idn, name)
		s.mu.Unlock()
		s.emitDelta("hypr_workspace_add", map[string]any{
			"workspace": map[string]any{"id": idn, "name": name},
		})

	case "destroyworkspacev2":
		id, _ := split2(data)
		if id == "" {
			return
		}
		idn, _ := strconv.Atoi(id)
		s.mu.Lock()
		s.removeWorkspace(idn)
		for name, wid := range s.wsNameToID {
			if wid == idn {
				delete(s.wsNameToID, name)
				break
			}
		}
		var affectedMonitors []map[string]any
		for i, m := range s.monitors {
			if aw, ok := m["activeWorkspace"].(map[string]any); ok {
				if awid, _ := aw["id"].(float64); int(awid) == idn {
					delete(s.monitors[i], "activeWorkspace")
					affectedMonitors = append(affectedMonitors, s.snapshotMonitorByName(m["name"].(string)))
				}
			}
		}
		s.mu.Unlock()
		s.emitDelta("hypr_workspace_remove", map[string]any{"id": idn})
		for _, monSnap := range affectedMonitors {
			s.emitDelta("hypr_monitor_update", map[string]any{"monitor": monSnap})
		}

	case "moveworkspacev2":
		id, rest := split2(data)
		wsName, monName := split2(rest)
		if id == "" || monName == "" {
			return
		}
		wsID, _ := strconv.Atoi(id)
		s.mu.Lock()
		s.upsertWorkspace(wsID, wsName)
		s.putMonitorActiveWS(monName, wsID, wsName)
		var clearedMonSnaps []map[string]any
		for i, m := range s.monitors {
			if name, _ := m["name"].(string); name != monName {
				if aw, ok := m["activeWorkspace"].(map[string]any); ok {
					if awid, _ := aw["id"].(float64); int(awid) == wsID {
						delete(s.monitors[i], "activeWorkspace")
						clearedMonSnaps = append(clearedMonSnaps, s.snapshotMonitorByName(name))
					}
				}
			}
		}
		newMonID := 0
		for _, m := range s.monitors {
			if mn, _ := m["name"].(string); mn == monName {
				if mid, ok := m["id"].(float64); ok {
					newMonID = int(mid)
				}
				break
			}
		}
		var windowDeltas []map[string]any
		if newMonID > 0 {
			for _, w := range s.windows {
				if ws, ok := w["workspace"].(map[string]any); ok {
					if wid, _ := ws["id"].(float64); int(wid) == wsID {
						w["monitor"] = newMonID
						if addr, _ := w["address"].(string); addr != "" {
							windowDeltas = append(windowDeltas, map[string]any{
								"address": addr, "updates": map[string]any{"monitor": newMonID},
							})
						}
					}
				}
			}
		}
		targetMonSnap := s.snapshotMonitorByName(monName)
		s.mu.Unlock()
		s.emitDelta("hypr_workspace_update", map[string]any{
			"workspace": map[string]any{"id": wsID, "name": wsName, "monitorName": monName},
		})
		if targetMonSnap != nil {
			s.emitDelta("hypr_monitor_update", map[string]any{"monitor": targetMonSnap})
		}
		for _, monSnap := range clearedMonSnaps {
			s.emitDelta("hypr_monitor_update", map[string]any{"monitor": monSnap})
		}
		for _, wd := range windowDeltas {
			s.emitDelta("hypr_window_update", wd)
		}

	case "renameworkspace":
		idStr, newName := split2(data)
		if idStr == "" {
			return
		}
		id, _ := strconv.Atoi(idStr)
		s.mu.Lock()
		s.renameWorkspaceInPlace(id, newName)
		s.mu.Unlock()
		s.emitDelta("hypr_workspace_update", map[string]any{
			"workspace": map[string]any{"id": id, "name": newName},
		})

	case "activespecialv2":
		id, rest := split2(data)
		wsName, monName := split2(rest)
		if monName == "" {
			return
		}
		wsID, _ := strconv.Atoi(id)
		s.mu.Lock()
		if wsID == 0 && wsName == "" {
			s.deleteMonitorField(monName, "specialWorkspace")
		} else {
			s.upsertWorkspace(wsID, wsName)
			s.putMonitorField(monName, "specialWorkspace", map[string]any{"id": wsID, "name": wsName})
		}
		monSnap := s.snapshotMonitorByName(monName)
		s.mu.Unlock()
		if monSnap != nil {
			s.emitDelta("hypr_monitor_update", map[string]any{"monitor": monSnap})
		}

	// ── Windows ─────────────────────────────────────────────────────────────

	case "openwindow":
		addr, rest := split2(data)
		wsName, rest := split2(rest)
		class, title := split2(rest)
		if addr == "" {
			return
		}
		s.mu.Lock()
		wsID := s.lookupWorkspaceID(wsName)
		win := map[string]any{"address": addr, "class": class, "title": title}
		if wsID > 0 {
			win["workspace"] = map[string]any{"id": wsID, "name": wsName}
		}
		s.windows = append(s.windows, win)
		s.needsWindowFetch = true
		// Snapshot the stub for immediate emission.
		winSnap := make(map[string]any, len(win))
		maps.Copy(winSnap, win)
		if ws, ok := win["workspace"].(map[string]any); ok {
			wsCp := make(map[string]any, len(ws))
			maps.Copy(wsCp, ws)
			winSnap["workspace"] = wsCp
		}
		s.mu.Unlock()
		s.emitDelta("hypr_window_add", map[string]any{"window": winSnap})

	case "closewindow", "kill":
		s.mu.Lock()
		s.removeWindow(data)
		s.mu.Unlock()
		s.emitDelta("hypr_window_remove", map[string]any{"address": data})

	case "activewindowv2":
		s.mu.Lock()
		if s.activeWorkspace == nil {
			s.activeWorkspace = make(map[string]any, 4)
		}
		if data == "" {
			delete(s.activeWorkspace, "activeWindow")
		} else {
			s.activeWorkspace["activeWindow"] = data
		}
		snap := s.snapshotActiveWorkspace()
		s.mu.Unlock()
		s.emitDelta("hypr_active_workspace", map[string]any{
			"activeWorkspace": snap,
		})

	case "movewindowv2":
		addr, rest := split2(data)
		wsIDStr, wsName := split2(rest)
		if addr == "" {
			return
		}
		wsID, _ := strconv.Atoi(wsIDStr)
		s.mu.Lock()
		s.putWindowWorkspace(addr, wsID, wsName)
		monID := 0
		for _, m := range s.monitors {
			if aw, ok := m["activeWorkspace"].(map[string]any); ok {
				if awid, _ := aw["id"].(float64); int(awid) == wsID {
					if mid, ok := m["id"].(float64); ok {
						monID = int(mid)
					}
					break
				}
			}
		}
		if monID > 0 {
			s.putWindowField(addr, "monitor", monID)
		}
		s.mu.Unlock()
		updates := map[string]any{
			"workspace": map[string]any{"id": wsID, "name": wsName},
		}
		if monID > 0 {
			updates["monitor"] = monID
		}
		s.emitDelta("hypr_window_update", map[string]any{
			"address": addr, "updates": updates,
		})

	case "movewindow":
		addr, wsName := split2(data)
		if addr == "" {
			return
		}
		wsID := s.lookupWorkspaceID(wsName)
		if wsID == 0 {
			return
		}
		s.mu.Lock()
		s.putWindowWorkspace(addr, wsID, wsName)
		monID := 0
		for _, m := range s.monitors {
			if aw, ok := m["activeWorkspace"].(map[string]any); ok {
				if awid, _ := aw["id"].(float64); int(awid) == wsID {
					if mid, ok := m["id"].(float64); ok {
						monID = int(mid)
					}
					break
				}
			}
		}
		if monID > 0 {
			s.putWindowField(addr, "monitor", monID)
		}
		s.mu.Unlock()
		updates := map[string]any{
			"workspace": map[string]any{"id": wsID, "name": wsName},
		}
		if monID > 0 {
			updates["monitor"] = monID
		}
		s.emitDelta("hypr_window_update", map[string]any{
			"address": addr, "updates": updates,
		})

	case "windowtitlev2":
		addr, title := split2(data)
		if addr == "" {
			return
		}
		s.mu.Lock()
		s.putWindowField(addr, "title", title)
		s.mu.Unlock()
		s.emitDelta("hypr_window_update", map[string]any{
			"address": addr, "updates": map[string]any{"title": title},
		})

	case "windowtitle":
		s.mu.Lock()
		aw, _ := s.activeWorkspace["activeWindow"].(string)
		if aw != "" {
			s.putWindowField(aw, "title", data)
		}
		s.mu.Unlock()
		if aw != "" {
			s.emitDelta("hypr_window_update", map[string]any{
				"address": aw, "updates": map[string]any{"title": data},
			})
		}

	case "changefloatingmode":
		addr, val := split2(data)
		if addr == "" {
			return
		}
		floating := val == "1"
		s.mu.Lock()
		s.putWindowField(addr, "floating", floating)
		s.mu.Unlock()
		s.emitDelta("hypr_window_update", map[string]any{
			"address": addr, "updates": map[string]any{"floating": floating},
		})

	case "fullscreen":
		isFS := data == "1"
		s.mu.Lock()
		aw, _ := s.activeWorkspace["activeWindow"].(string)
		if aw != "" {
			s.putWindowField(aw, "fullscreen", isFS)
		}
		s.mu.Unlock()
		if aw != "" {
			s.emitDelta("hypr_window_update", map[string]any{
				"address": aw, "updates": map[string]any{"fullscreen": isFS},
			})
		}

	case "pin":
		addr, val := split2(data)
		if addr == "" {
			return
		}
		pinned := val == "1"
		s.mu.Lock()
		s.putWindowField(addr, "pinned", pinned)
		s.mu.Unlock()
		s.emitDelta("hypr_window_update", map[string]any{
			"address": addr, "updates": map[string]any{"pinned": pinned},
		})

	case "minimized":
		addr, val := split2(data)
		if addr == "" {
			return
		}
		minimized := val == "1"
		s.mu.Lock()
		s.putWindowField(addr, "minimized", minimized)
		s.mu.Unlock()
		s.emitDelta("hypr_window_update", map[string]any{
			"address": addr, "updates": map[string]any{"minimized": minimized},
		})

	case "urgent":
		s.mu.Lock()
		s.putWindowField(data, "urgent", true)
		s.mu.Unlock()
		s.emitDelta("hypr_window_update", map[string]any{
			"address": data, "updates": map[string]any{"urgent": true},
		})

	// ── Monitors ────────────────────────────────────────────────────────────

	case "monitoraddedv2":
		id, rest := split2(data)
		name, desc := split2(rest)
		if name == "" {
			return
		}
		monID, _ := strconv.Atoi(id)
		s.mu.Lock()
		found := false
		for _, m := range s.monitors {
			if mn, _ := m["name"].(string); mn == name {
				found = true
				break
			}
		}
		if !found {
			s.monitors = append(s.monitors, map[string]any{
				"id": monID, "name": name, "description": desc,
			})
		}
		s.needsMonitorDetails = true
		s.mu.Unlock()
		s.emitDelta("hypr_monitor_add", map[string]any{
			"monitor": map[string]any{"id": monID, "name": name, "description": desc},
		})

	case "monitorremovedv2":
		_, rest := split2(data)
		name, _ := split2(rest)
		if name == "" {
			return
		}
		s.mu.Lock()
		for i, m := range s.monitors {
			if mn, _ := m["name"].(string); mn == name {
				s.monitors = append(s.monitors[:i], s.monitors[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		s.emitDelta("hypr_monitor_remove", map[string]any{"name": name})

	// ── Layers ──────────────────────────────────────────────────────────────

	case "openlayer":
		s.mu.Lock()
		if s.layers == nil {
			s.layers = make(map[string]any)
		}
		s.layers[data] = true
		s.mu.Unlock()
		s.emitDelta("hypr_layer_update", map[string]any{"name": data, "state": true})

	case "closelayer":
		s.mu.Lock()
		if s.layers != nil {
			delete(s.layers, data)
		}
		s.mu.Unlock()
		s.emitDelta("hypr_layer_update", map[string]any{"name": data, "state": false})

	// ── Config ──────────────────────────────────────────────────────────────

	case "configreloaded":
		s.needsFullFetch = true
	}
}

// ── In-place cache mutators ────────────────────────────────────────────────────

func (s *Service) putActiveWorkspace(id int, name string) {
	if s.activeWorkspace == nil {
		s.activeWorkspace = make(map[string]any, 4)
	}
	mon, _ := s.activeWorkspace["monitor"].(string)
	for k := range s.activeWorkspace {
		if k != "monitor" {
			delete(s.activeWorkspace, k)
		}
	}
	s.activeWorkspace["id"] = id
	s.activeWorkspace["name"] = name
	if mon != "" {
		s.activeWorkspace["monitor"] = mon
		s.putMonitorActiveWS(mon, id, name)
	}
}

func (s *Service) putActiveWorkspaceFull(id int, name, monName string) {
	if s.activeWorkspace == nil {
		s.activeWorkspace = make(map[string]any, 4)
	}
	for k := range s.activeWorkspace {
		delete(s.activeWorkspace, k)
	}
	s.activeWorkspace["id"] = id
	s.activeWorkspace["name"] = name
	s.activeWorkspace["monitor"] = monName
}

func (s *Service) putMonitorActiveWS(monName string, wsID int, wsName string) {
	for i, m := range s.monitors {
		if name, _ := m["name"].(string); name == monName {
			if aw, ok := m["activeWorkspace"].(map[string]any); ok {
				aw["id"] = wsID
				aw["name"] = wsName
			} else {
				s.monitors[i]["activeWorkspace"] = map[string]any{"id": wsID, "name": wsName}
			}
			return
		}
	}
}

func (s *Service) putMonitorField(monName, field string, val any) {
	for i, m := range s.monitors {
		if name, _ := m["name"].(string); name == monName {
			s.monitors[i][field] = val
			return
		}
	}
}

func (s *Service) deleteMonitorField(monName, field string) {
	for i, m := range s.monitors {
		if name, _ := m["name"].(string); name == monName {
			delete(s.monitors[i], field)
			return
		}
	}
}

func (s *Service) putWindowField(addr, field string, val any) {
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == addr {
			s.windows[i][field] = val
			return
		}
	}
}

func (s *Service) putWindowWorkspace(addr string, wsID int, wsName string) {
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == addr {
			if ws, ok := w["workspace"].(map[string]any); ok {
				ws["id"] = wsID
				ws["name"] = wsName
			} else {
				s.windows[i]["workspace"] = map[string]any{"id": wsID, "name": wsName}
			}
			return
		}
	}
}

func (s *Service) removeWindow(addr string) {
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == addr {
			s.windows = append(s.windows[:i], s.windows[i+1:]...)
			return
		}
	}
}

func (s *Service) removeWorkspace(id int) {
	for i, ws := range s.workspaces {
		if wid, _ := ws["id"].(float64); int(wid) == id {
			s.workspaces = append(s.workspaces[:i], s.workspaces[i+1:]...)
			return
		}
	}
}

func (s *Service) renameWorkspaceInPlace(id int, newName string) {
	for i, ws := range s.workspaces {
		if wid, _ := ws["id"].(float64); int(wid) == id {
			oldName, _ := ws["name"].(string)
			s.workspaces[i]["name"] = newName
			delete(s.wsNameToID, oldName)
			s.wsNameToID[newName] = id
			break
		}
	}
	if aw, ok := s.activeWorkspace["id"].(float64); ok && int(aw) == id {
		s.activeWorkspace["name"] = newName
	}
}

// ── Cache helpers ─────────────────────────────────────────────────────────────

func (s *Service) upsertWorkspace(id int, name string) {
	for i, ws := range s.workspaces {
		if wid, _ := ws["id"].(float64); int(wid) == id {
			oldName, _ := ws["name"].(string)
			s.workspaces[i]["name"] = name
			if oldName != name && oldName != "" {
				delete(s.wsNameToID, oldName)
			}
			s.wsNameToID[name] = id
			return
		}
	}
	s.workspaces = append(s.workspaces, map[string]any{"id": id, "name": name})
	s.wsNameToID[name] = id
}

func (s *Service) lookupWorkspaceID(name string) int {
	if id, ok := s.wsNameToID[name]; ok {
		return id
	}
	if id, err := strconv.Atoi(name); err == nil {
		return id
	}
	return 0
}

func (s *Service) workspaceNameByID(id int) string {
	for _, ws := range s.workspaces {
		if wid, _ := ws["id"].(float64); int(wid) == id {
			if name, ok := ws["name"].(string); ok {
				return name
			}
		}
	}
	return ""
}

// ── Snapshot helpers (caller must hold at least a read lock) ───────────────

func (s *Service) snapshotActiveWorkspace() map[string]any {
	if s.activeWorkspace == nil {
		return nil
	}
	cp := make(map[string]any, len(s.activeWorkspace))
	maps.Copy(cp, s.activeWorkspace)
	return cp
}

func (s *Service) snapshotMonitorByName(name string) map[string]any {
	for _, m := range s.monitors {
		if mn, _ := m["name"].(string); mn == name {
			cp := make(map[string]any, len(m))
			maps.Copy(cp, m)
			if aw, ok := m["activeWorkspace"].(map[string]any); ok {
				awCp := make(map[string]any, len(aw))
				maps.Copy(awCp, aw)
				cp["activeWorkspace"] = awCp
			}
			return cp
		}
	}
	return nil
}

func (s *Service) snapshotWindows() []map[string]any {
	out := make([]map[string]any, len(s.windows))
	for i, w := range s.windows {
		out[i] = snapshotWindowMap(w)
	}
	return out
}

func (s *Service) snapshotMonitors() []map[string]any {
	out := make([]map[string]any, len(s.monitors))
	for i, m := range s.monitors {
		out[i] = snapshotMonitorMap(m)
	}
	return out
}

// snapshotWindowMap deep-copies a single window entry.
func snapshotWindowMap(w map[string]any) map[string]any {
	cp := make(map[string]any, len(w))
	maps.Copy(cp, w)
	if ws, ok := w["workspace"].(map[string]any); ok {
		wsCp := make(map[string]any, len(ws))
		maps.Copy(wsCp, ws)
		cp["workspace"] = wsCp
	}
	return cp
}

// snapshotMonitorMap deep-copies a single monitor entry.
func snapshotMonitorMap(m map[string]any) map[string]any {
	cp := make(map[string]any, len(m))
	maps.Copy(cp, m)
	if aw, ok := m["activeWorkspace"].(map[string]any); ok {
		awCp := make(map[string]any, len(aw))
		maps.Copy(awCp, aw)
		cp["activeWorkspace"] = awCp
	}
	return cp
}

// ── Zero-alloc CSV splitting ──────────────────────────────────────────────────

func split2(data string) (string, string) {
	if before, after, ok := strings.Cut(data, ","); ok {
		return before, after
	}
	return data, ""
}
