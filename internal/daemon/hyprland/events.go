package hyprland

import (
	"strconv"
	"strings"
)

// handleSocket2Event parses a socket2 event line and mutates the in-memory cache.
// All events are self-contained — no socket1 IPC is needed except for:
//   - "configreloaded" → full re-fetch
//   - "fullscreen" → windows re-fetch (event doesn't carry the window address)
func (s *Service) handleSocket2Event(eventName, data string) {
	switch eventName {

	// ── Workspaces ──────────────────────────────────────────────────────────

	case "workspacev2":
		// data: "ID,NAME"  (e.g. "3,Terminal")
		s.handleWorkspaceV2(data)

	case "focusedmonv2":
		// data: "MONNAME,WORKSPACEID"  (e.g. "DP-1,3")
		s.handleFocusedMonV2(data)

	case "createworkspacev2":
		// data: "ID,NAME"  (e.g. "5,Web")
		s.handleCreateWorkspaceV2(data)

	case "destroyworkspacev2":
		// data: "ID,NAME"  (e.g. "5,Web")
		s.handleDestroyWorkspaceV2(data)

	case "moveworkspacev2":
		// data: "WORKSPACEID,WORKSPACENAME,MONNAME"
		s.handleMoveWorkspaceV2(data)

	case "renameworkspace":
		// data: "WORKSPACEID,NEWNAME"
		s.handleRenameWorkspace(data)

	case "activespecialv2":
		// data: "WORKSPACEID,WORKSPACENAME,MONNAME"
		s.handleActiveSpecialV2(data)

	// ── Windows ─────────────────────────────────────────────────────────────

	case "openwindow":
		// data: "ADDR,WORKSPACENAME,CLASS,TITLE"
		s.handleOpenWindow(data)

	case "closewindow":
		// data: "ADDR"
		s.handleCloseWindow(data)

	case "activewindowv2":
		// data: "ADDR" (empty means no active window)
		s.handleActiveWindowV2(data)

	case "movewindowv2":
		// data: "ADDR,WORKSPACEID,WORKSPACENAME"
		s.handleMoveWindowV2(data)

	case "windowtitlev2":
		// data: "ADDR,TITLE"
		s.handleWindowTitleV2(data)

	case "changefloatingmode":
		// data: "ADDR,0/1"
		s.handleChangeFloatingMode(data)

	case "fullscreen":
		// data: "0"/"1" (no address — patch active window)
		s.handleFullscreen(data)

	case "pin":
		// data: "ADDR,0/1"
		s.handlePin(data)

	case "minimized":
		// data: "ADDR,0/1"
		s.handleMinimized(data)

	case "urgent":
		// data: "ADDR"
		s.handleUrgent(data)

	// ── Monitors ────────────────────────────────────────────────────────────

	case "monitoraddedv2":
		// data: "ID,NAME,DESCRIPTION"
		s.handleMonitorAddedV2(data)

	case "monitorremovedv2":
		// data: "ID,NAME,DESCRIPTION"
		s.handleMonitorRemovedV2(data)

	// ── Layers ──────────────────────────────────────────────────────────────

	case "openlayer":
		// data: "NAMESPACE"
		s.handleOpenLayer(data)

	case "closelayer":
		// data: "NAMESPACE"
		s.handleCloseLayer(data)

	// ── Config ──────────────────────────────────────────────────────────────

	case "configreloaded":
		s.needsFullFetch = true

	// ── Ignored (not used by QML currently) ─────────────────────────────────
	//
	// workspace, focusedmon, activewindow, moveworkspace, createworkspace,
	// destroyworkspace, monitoradded, monitorremoved, windowtitle,
	// submap, screencast, screencastv2, togglegroup, moveintogroup,
	// moveoutofgroup, ignoregrouplock, lockgroups, kill, bell, activelayout
	//
	// These are either legacy v1 events (we only track v2) or events
	// that the QML layer doesn't consume.
	default:
		return
	}
}

// ── Workspace handlers ────────────────────────────────────────────────────────

func (s *Service) handleWorkspaceV2(data string) {
	id, name := split2(data)
	if id == "" {
		return
	}
	idn, _ := strconv.Atoi(id)

	s.mu.Lock()
	s.activeWorkspace = map[string]any{"id": idn, "name": name}
	// Ensure this workspace exists in the list.
	s.upsertWorkspace(idn, name)
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleFocusedMonV2(data string) {
	monName, wsIDStr := split2(data)
	if monName == "" {
		return
	}
	wsID, _ := strconv.Atoi(wsIDStr)

	s.mu.Lock()
	// Update the monitor's active workspace.
	for i, m := range s.monitors {
		if name, _ := m["name"].(string); name == monName {
			s.monitors[i]["activeWorkspace"] = map[string]any{"id": wsID}
			break
		}
	}
	// Also update the global active workspace.
	if wsIDStr != "" {
		s.activeWorkspace = map[string]any{"id": wsID}
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleCreateWorkspaceV2(data string) {
	id, name := split2(data)
	if id == "" {
		return
	}
	idn, _ := strconv.Atoi(id)

	s.mu.Lock()
	s.upsertWorkspace(idn, name)
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleDestroyWorkspaceV2(data string) {
	id, _ := split2(data)
	if id == "" {
		return
	}
	idn, _ := strconv.Atoi(id)

	s.mu.Lock()
	for i, ws := range s.workspaces {
		if wid, _ := ws["id"].(float64); int(wid) == idn {
			s.workspaces = append(s.workspaces[:i], s.workspaces[i+1:]...)
			break
		}
	}
	// Clean up name mapping for this ID.
	for name, wid := range s.wsNameToID {
		if wid == idn {
			delete(s.wsNameToID, name)
			break
		}
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleMoveWorkspaceV2(data string) {
	// data: "WORKSPACEID,WORKSPACENAME,MONNAME"
	parts := splitN(data, 3)
	if len(parts) < 3 {
		return
	}
	wsID, _ := strconv.Atoi(parts[0])
	wsName := parts[1]
	monName := parts[2]

	s.mu.Lock()
	// Update the workspace entry.
	s.upsertWorkspace(wsID, wsName)
	// Reassign workspace to the new monitor.
	for i, m := range s.monitors {
		if name, _ := m["name"].(string); name == monName {
			s.monitors[i]["activeWorkspace"] = map[string]any{"id": wsID}
		} else {
			// Clear this workspace from other monitors if it was there.
			if aw, ok := m["activeWorkspace"].(map[string]any); ok {
				if awid, _ := aw["id"].(float64); int(awid) == wsID {
					delete(s.monitors[i], "activeWorkspace")
				}
			}
		}
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleRenameWorkspace(data string) {
	idStr, newName := split2(data)
	if idStr == "" {
		return
	}
	id, _ := strconv.Atoi(idStr)

	s.mu.Lock()
	for i, ws := range s.workspaces {
		if wid, _ := ws["id"].(float64); int(wid) == id {
			oldName, _ := ws["name"].(string)
			s.workspaces[i]["name"] = newName
			// Update name→ID mapping.
			delete(s.wsNameToID, oldName)
			s.wsNameToID[newName] = id
			break
		}
	}
	// Also update activeWorkspace name if it matches.
	if aw, ok := s.activeWorkspace["id"].(float64); ok && int(aw) == id {
		s.activeWorkspace["name"] = newName
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleActiveSpecialV2(data string) {
	// data: "WORKSPACEID,WORKSPACENAME,MONNAME"
	parts := splitN(data, 3)
	if len(parts) < 3 {
		return
	}
	wsID, _ := strconv.Atoi(parts[0])
	wsName := parts[1]
	monName := parts[2]

	s.mu.Lock()
	if wsID == 0 && wsName == "" {
		// Special workspace closed on this monitor.
		for i, m := range s.monitors {
			if name, _ := m["name"].(string); name == monName {
				delete(s.monitors[i], "specialWorkspace")
				break
			}
		}
	} else {
		s.upsertWorkspace(wsID, wsName)
		for i, m := range s.monitors {
			if name, _ := m["name"].(string); name == monName {
				s.monitors[i]["specialWorkspace"] = map[string]any{"id": wsID, "name": wsName}
				break
			}
		}
	}
	s.mu.Unlock()
	s.emit()
}

// ── Window handlers ───────────────────────────────────────────────────────────

func (s *Service) handleOpenWindow(data string) {
	// data: "ADDR,WORKSPACENAME,CLASS,TITLE"
	parts := splitN(data, 4)
	if len(parts) < 4 {
		return
	}
	addr := parts[0]
	wsName := parts[1]
	class := parts[2]
	title := parts[3]

	s.mu.Lock()
	wsID := s.lookupWorkspaceID(wsName)

	win := map[string]any{
		"address": addr,
		"class":   class,
		"title":   title,
	}
	if wsID > 0 {
		win["workspace"] = map[string]any{"id": wsID, "name": wsName}
	}
	s.windows = append(s.windows, win)
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleCloseWindow(data string) {
	// data: "ADDR"
	addr := data

	s.mu.Lock()
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == addr {
			s.windows = append(s.windows[:i], s.windows[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleActiveWindowV2(data string) {
	// data: "ADDR" or empty
	s.mu.Lock()
	if data == "" {
		delete(s.activeWorkspace, "activeWindow")
	} else {
		if s.activeWorkspace == nil {
			s.activeWorkspace = make(map[string]any)
		}
		s.activeWorkspace["activeWindow"] = data
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleMoveWindowV2(data string) {
	// data: "ADDR,WORKSPACEID,WORKSPACENAME"
	parts := splitN(data, 3)
	if len(parts) < 3 {
		return
	}
	addr := parts[0]
	wsID, _ := strconv.Atoi(parts[1])
	wsName := parts[2]

	s.mu.Lock()
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == addr {
			s.windows[i]["workspace"] = map[string]any{"id": wsID, "name": wsName}
			break
		}
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleWindowTitleV2(data string) {
	// data: "ADDR,TITLE"
	addr, title := split2(data)
	if addr == "" {
		return
	}

	s.mu.Lock()
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == addr {
			s.windows[i]["title"] = title
			break
		}
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleChangeFloatingMode(data string) {
	addr, floatingStr := split2(data)
	if addr == "" {
		return
	}
	floating := floatingStr == "1"

	s.mu.Lock()
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == addr {
			s.windows[i]["floating"] = floating
			break
		}
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleFullscreen(data string) {
	// data: "0"/"1" — no window address. Patch the active window.
	isFS := data == "1"

	s.mu.Lock()
	activeAddr, _ := s.activeWorkspace["activeWindow"].(string)
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == activeAddr {
			s.windows[i]["fullscreen"] = isFS
			break
		}
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handlePin(data string) {
	addr, pinStr := split2(data)
	if addr == "" {
		return
	}
	pinned := pinStr == "1"

	s.mu.Lock()
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == addr {
			s.windows[i]["pinned"] = pinned
			break
		}
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleMinimized(data string) {
	addr, minimizedStr := split2(data)
	if addr == "" {
		return
	}
	minimized := minimizedStr == "1"

	s.mu.Lock()
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == addr {
			s.windows[i]["minimized"] = minimized
			break
		}
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleUrgent(data string) {
	// data: "ADDR" — set urgent flag on window.
	addr := data

	s.mu.Lock()
	for i, w := range s.windows {
		if a, _ := w["address"].(string); a == addr {
			s.windows[i]["urgent"] = true
			break
		}
	}
	s.mu.Unlock()
	s.emit()
}

// ── Monitor handlers ──────────────────────────────────────────────────────────

func (s *Service) handleMonitorAddedV2(data string) {
	parts := splitN(data, 3)
	if len(parts) < 2 {
		return
	}
	id, _ := strconv.Atoi(parts[0])
	name := parts[1]
	desc := ""
	if len(parts) >= 3 {
		desc = parts[2]
	}

	s.mu.Lock()
	// Check if already present (avoid duplicates on reconnect + add).
	found := false
	for _, m := range s.monitors {
		if mn, _ := m["name"].(string); mn == name {
			found = true
			break
		}
	}
	if !found {
		s.monitors = append(s.monitors, map[string]any{
			"id":          id,
			"name":        name,
			"description": desc,
		})
		// Monitor added without full data (no resolution/scale/position).
		// Mark for a targeted refresh to fill in the missing fields.
		s.needsMonitorDetails = true
	}
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleMonitorRemovedV2(data string) {
	parts := splitN(data, 3)
	if len(parts) < 2 {
		return
	}
	name := parts[1]

	s.mu.Lock()
	for i, m := range s.monitors {
		if mn, _ := m["name"].(string); mn == name {
			s.monitors = append(s.monitors[:i], s.monitors[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	s.emit()
}

// ── Layer handlers ────────────────────────────────────────────────────────────

func (s *Service) handleOpenLayer(data string) {
	// data: "NAMESPACE"
	ns := data

	s.mu.Lock()
	if s.layers == nil {
		s.layers = make(map[string]any)
	}
	s.layers[ns] = true
	s.mu.Unlock()
	s.emit()
}

func (s *Service) handleCloseLayer(data string) {
	// data: "NAMESPACE"
	ns := data

	s.mu.Lock()
	if s.layers != nil {
		delete(s.layers, ns)
	}
	s.mu.Unlock()
	s.emit()
}

// ── Cache helpers ─────────────────────────────────────────────────────────────

// upsertWorkspace adds or updates a workspace entry in the list and the name map.
// Must be called with s.mu held.
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
	s.workspaces = append(s.workspaces, map[string]any{
		"id":   id,
		"name": name,
	})
	s.wsNameToID[name] = id
}

// lookupWorkspaceID resolves a workspace name to its numeric ID.
// Must be called with s.mu held.
func (s *Service) lookupWorkspaceID(name string) int {
	if id, ok := s.wsNameToID[name]; ok {
		return id
	}
	// Fallback: try parsing the name as a number (default ws names are numeric).
	if id, err := strconv.Atoi(name); err == nil {
		return id
	}
	return 0
}

// ── CSV splitting helpers (zero-alloc for common cases) ───────────────────────

// split2 splits data by the first comma. Returns both parts.
func split2(data string) (string, string) {
	if before, after, ok := strings.Cut(data, ","); ok {
		return before, after
	}
	return data, ""
}

// splitN splits data into at most n comma-separated parts.
func splitN(data string, n int) []string {
	parts := make([]string, 0, n)
	rest := data
	for i := 0; i < n-1 && rest != ""; i++ {
		if idx := strings.IndexByte(rest, ','); idx >= 0 {
			parts = append(parts, rest[:idx])
			rest = rest[idx+1:]
		} else {
			parts = append(parts, rest)
			rest = ""
		}
	}
	if rest != "" || len(parts) < n {
		parts = append(parts, rest)
	}
	return parts
}
