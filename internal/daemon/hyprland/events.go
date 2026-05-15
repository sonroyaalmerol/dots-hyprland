package hyprland

import (
	"strconv"
	"strings"
)

// handleSocket2Event parses a socket2 event line and mutates the in-memory cache.
// Zero heap allocation on the parse path — all CSV fields are sliced substrings
// of the original event data, and all cache maps are mutated in-place.
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
		s.mu.Unlock()
		s.emit()

	case "focusedmonv2":
		monName, wsIDStr := split2(data)
		if monName == "" {
			return
		}
		wsID, _ := strconv.Atoi(wsIDStr)
		s.mu.Lock()
		s.putMonitorActiveWS(monName, wsID)
		s.putActiveWorkspaceMonitor(wsID, monName)
		s.mu.Unlock()
		s.emit()

	case "createworkspacev2":
		id, name := split2(data)
		if id == "" {
			return
		}
		idn, _ := strconv.Atoi(id)
		s.mu.Lock()
		s.upsertWorkspace(idn, name)
		s.mu.Unlock()
		s.emit()

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
		s.mu.Unlock()
		s.emit()

	case "moveworkspacev2":
		// "WORKSPACEID,WORKSPACENAME,MONNAME"
		id, rest := split2(data)
		wsName, monName := split2(rest)
		if id == "" || monName == "" {
			return
		}
		wsID, _ := strconv.Atoi(id)
		s.mu.Lock()
		s.upsertWorkspace(wsID, wsName)
		s.putMonitorActiveWS(monName, wsID)
		// Clear this workspace from other monitors.
		for i, m := range s.monitors {
			if name, _ := m["name"].(string); name != monName {
				if aw, ok := m["activeWorkspace"].(map[string]any); ok {
					if awid, _ := aw["id"].(float64); int(awid) == wsID {
						delete(s.monitors[i], "activeWorkspace")
					}
				}
			}
		}
		s.mu.Unlock()
		s.emit()

	case "renameworkspace":
		idStr, newName := split2(data)
		if idStr == "" {
			return
		}
		id, _ := strconv.Atoi(idStr)
		s.mu.Lock()
		s.renameWorkspaceInPlace(id, newName)
		s.mu.Unlock()
		s.emit()

	case "activespecialv2":
		// "WORKSPACEID,WORKSPACENAME,MONNAME"
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
		s.mu.Unlock()
		s.emit()

	// ── Windows ─────────────────────────────────────────────────────────────

	case "openwindow":
		// "ADDR,WORKSPACENAME,CLASS,TITLE" — up to 3 cuts
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
		s.mu.Unlock()
		s.emit()

	case "closewindow", "kill":
		s.mu.Lock()
		s.removeWindow(data)
		s.mu.Unlock()
		s.emit()

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
		s.mu.Unlock()
		s.emit()

	case "movewindowv2":
		// "ADDR,WORKSPACEID,WORKSPACENAME"
		addr, rest := split2(data)
		wsIDStr, wsName := split2(rest)
		if addr == "" {
			return
		}
		wsID, _ := strconv.Atoi(wsIDStr)
		s.mu.Lock()
		s.putWindowWorkspace(addr, wsID, wsName)
		s.mu.Unlock()
		s.emit()

	case "windowtitlev2":
		addr, title := split2(data)
		if addr == "" {
			return
		}
		s.mu.Lock()
		s.putWindowField(addr, "title", title)
		s.mu.Unlock()
		s.emit()

	case "changefloatingmode":
		addr, val := split2(data)
		if addr == "" {
			return
		}
		s.mu.Lock()
		s.putWindowField(addr, "floating", val == "1")
		s.mu.Unlock()
		s.emit()

	case "fullscreen":
		s.mu.Lock()
		if aw, ok := s.activeWorkspace["activeWindow"].(string); ok {
			s.putWindowField(aw, "fullscreen", data == "1")
		}
		s.mu.Unlock()
		s.emit()

	case "pin":
		addr, val := split2(data)
		if addr == "" {
			return
		}
		s.mu.Lock()
		s.putWindowField(addr, "pinned", val == "1")
		s.mu.Unlock()
		s.emit()

	case "minimized":
		addr, val := split2(data)
		if addr == "" {
			return
		}
		s.mu.Lock()
		s.putWindowField(addr, "minimized", val == "1")
		s.mu.Unlock()
		s.emit()

	case "urgent":
		s.mu.Lock()
		s.putWindowField(data, "urgent", true)
		s.mu.Unlock()
		s.emit()

	// ── Monitors ────────────────────────────────────────────────────────────

	case "monitoraddedv2":
		// "ID,NAME,DESCRIPTION"
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
			s.needsMonitorDetails = true
		}
		s.mu.Unlock()
		s.emit()

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
		s.emit()

	// ── Layers ──────────────────────────────────────────────────────────────

	case "openlayer":
		s.mu.Lock()
		if s.layers == nil {
			s.layers = make(map[string]any)
		}
		s.layers[data] = true
		s.mu.Unlock()
		s.emit()

	case "closelayer":
		s.mu.Lock()
		if s.layers != nil {
			delete(s.layers, data)
		}
		s.mu.Unlock()
		s.emit()

	// ── Config ──────────────────────────────────────────────────────────────

	case "configreloaded":
		s.needsFullFetch = true
	}
}

// ── In-place cache mutators (zero map allocations) ────────────────────────────

// putActiveWorkspace sets id/name on the activeWorkspace map, preserving the
// existing monitor field (workspacev2 fires on same-monitor switches where
// focusedmonv2 does NOT fire). Also updates the per-monitor activeWorkspace.id
// so the bar workspace highlight stays in sync.
func (s *Service) putActiveWorkspace(id int, name string) {
	if s.activeWorkspace == nil {
		s.activeWorkspace = make(map[string]any, 4)
	}
	// Preserve monitor from current value.
	mon, _ := s.activeWorkspace["monitor"].(string)
	// Wipe old keys except monitor.
	for k := range s.activeWorkspace {
		if k != "monitor" {
			delete(s.activeWorkspace, k)
		}
	}
	s.activeWorkspace["id"] = id
	s.activeWorkspace["name"] = name
	if mon != "" {
		s.activeWorkspace["monitor"] = mon
		s.putMonitorActiveWS(mon, id)
	}
}

// putActiveWorkspaceMonitor sets id+monitor on activeWorkspace (used by focusedmonv2).
func (s *Service) putActiveWorkspaceMonitor(id int, monName string) {
	if s.activeWorkspace == nil {
		s.activeWorkspace = make(map[string]any, 4)
	}
	// Wipe all old keys.
	for k := range s.activeWorkspace {
		delete(s.activeWorkspace, k)
	}
	s.activeWorkspace["id"] = id
	s.activeWorkspace["monitor"] = monName
}

// putMonitorActiveWS sets the activeWorkspace field on a specific monitor entry.
func (s *Service) putMonitorActiveWS(monName string, wsID int) {
	for i, m := range s.monitors {
		if name, _ := m["name"].(string); name == monName {
			if aw, ok := m["activeWorkspace"].(map[string]any); ok {
				aw["id"] = wsID
			} else {
				s.monitors[i]["activeWorkspace"] = map[string]any{"id": wsID}
			}
			return
		}
	}
}

// putMonitorField sets a named field on a specific monitor entry.
func (s *Service) putMonitorField(monName, field string, val any) {
	for i, m := range s.monitors {
		if name, _ := m["name"].(string); name == monName {
			s.monitors[i][field] = val
			return
		}
	}
}

// deleteMonitorField removes a named field from a monitor entry.
func (s *Service) deleteMonitorField(monName, field string) {
	for i, m := range s.monitors {
		if name, _ := m["name"].(string); name == monName {
			delete(s.monitors[i], field)
			return
		}
	}
}

// putWindowField sets a named field on the window with the given address.
func (s *Service) putWindowField(addr, field string, val any) {
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == addr {
			s.windows[i][field] = val
			return
		}
	}
}

// putWindowWorkspace sets the workspace field on the window with the given address.
func (s *Service) putWindowWorkspace(addr string, wsID int, wsName string) {
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == addr {
			// Mutate existing workspace map if possible, else allocate.
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

// removeWindow deletes the window with the given address from the list.
func (s *Service) removeWindow(addr string) {
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == addr {
			s.windows = append(s.windows[:i], s.windows[i+1:]...)
			return
		}
	}
}

// removeWorkspace deletes the workspace with the given ID from the list.
func (s *Service) removeWorkspace(id int) {
	for i, ws := range s.workspaces {
		if wid, _ := ws["id"].(float64); int(wid) == id {
			s.workspaces = append(s.workspaces[:i], s.workspaces[i+1:]...)
			return
		}
	}
}

// renameWorkspaceInPlace updates the name of a workspace by ID in the list,
// activeWorkspace, and wsNameToID map.
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

// ── Zero-alloc CSV splitting ──────────────────────────────────────────────────

// split2 splits data at the first comma. Returns two substrings of the original
// data — zero heap allocation (Go string slicing is a view).
func split2(data string) (string, string) {
	if before, after, ok := strings.Cut(data, ","); ok {
		return before, after
	}
	return data, ""
}
