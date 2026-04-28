# App Drawer Feature Architecture

## 1. Overview

An Android-style full-screen app drawer overlay that shows all installed applications in a scrollable grid, with a top row of pinned favorite apps. It is a distinct interaction from the existing start menu / search overlay — it's a **pure browsing surface** for visually scanning and launching apps without typing.

---

## 2. New Files

| # | Path | Responsibility |
|---|------|---------------|
| 1 | `ii/modules/common/panels/appDrawer/AppDrawer.qml` | Full-screen PanelWindow overlay (entry point; `Scope` with `Loader` + `PanelWindow` + `HyprlandFocusGrab`, same pattern as `WaffleStartMenu.qml`) |
| 2 | `ii/modules/common/panels/appDrawer/AppDrawerContent.qml` | Content layout: search bar at top, pinned row, then scrollable all-apps grid |
| 3 | `ii/modules/common/panels/appDrawer/AppDrawerGrid.qml` | Reusable grid component that takes a `property list<DesktopEntry> desktopEntries` and renders `AppDrawerButton` items in a `GridLayout` |
| 4 | `ii/modules/common/panels/appDrawer/AppDrawerButton.qml` | Single app button (icon + label), right-click context menu with pin/unpin actions. Reuses `LauncherApps` for pinning. |
| 5 | `ii/modules/common/panels/appDrawer/AppDrawerSearchBar.qml` | Lightweight search/filter bar at top of drawer (optional; filters `AppSearch.list` in-place) |

**Rationale for `modules/common/panels/appDrawer/`**: The app drawer is a panel-level overlay (like lock, overview, cheatsheet), not a waffle-specific or ii-specific visual. Placing it in `common/panels/` allows both families to instantiate it with different styling wrappers if needed, while keeping the core logic shared. It parallels `common/panels/lock/` which is also shared.

---

## 3. Existing Files to Modify

| File | Change |
|------|--------|
| `ii/GlobalStates.qml` | Add `property bool appDrawerOpen: false` (mirrors `searchOpen`, `overviewOpen`, etc.) |
| `ii/modules/common/Config.qml` | Add `property JsonObject appDrawer` under `configOptionsJsonAdapter` with: `property string trigger: "swipe-up"` (values: `"swipe-up"`, `"keybind"`, `"both"`), `property int columns: 6`, `property int pinnedColumns: 6`, `property bool showSearchBar: true`, `property bool enabled: true` |
| `ii/panelFamilies/IllogicalImpulseFamily.qml` | Add `PanelLoader { component: AppDrawer {} }` |
| `ii/panelFamilies/WaffleFamily.qml` | Add `PanelLoader { component: AppDrawer {} }` |

---

## 4. Trigger Mechanism

### Primary: Keybind (GlobalShortcut)

A new `GlobalShortcut` registered inside `AppDrawer.qml`:

```qml
GlobalShortcut {
    name: "appDrawerToggle"
    description: "Toggles the app drawer"
    onPressed: { GlobalStates.appDrawerOpen = !GlobalStates.appDrawerOpen }
}
```

Default keybind suggestion: `Super+A` or a dedicated keybind configured by the user in Hyprland config mapping to `quickshell appDrawer toggle`.

### Secondary: Swipe-up gesture (future)

Touchpad swipe-up gestures require **snry-daemon** changes (see section 9) because QML/Wayland does not expose multi-finger gesture events to layer surfaces. The daemon would listen for `libinput` gesture events and call `quickshell appDrawer toggle` via the existing `IpcHandler` pattern.

### Tertiary: Bar/Dock button

Both the ii bar and waffle bar can include a small launcher button that calls `GlobalStates.appDrawerOpen = !GlobalStates.appDrawerOpen`. This is a trivial addition inside each bar's button row.

---

## 5. Panel Lifecycle (Opening / Closing)

Follows the exact pattern from `WaffleStartMenu.qml`:

```qml
// AppDrawer.qml (Scope)
Connections {
    target: GlobalStates
    function onAppDrawerOpenChanged() {
        if (GlobalStates.appDrawerOpen) panelLoader.active = true;
    }
}

Loader {
    id: panelLoader
    active: GlobalStates.appDrawerOpen
    sourceComponent: PanelWindow {
        exclusiveZone: 0
        WlrLayershell.namespace: "quickshell:appDrawer"
        WlrLayershell.keyboardFocus: WlrKeyboardFocus.OnDemand
        color: "transparent"
        anchors { top: true; bottom: true; left: true; right: true }

        HyprlandFocusGrab {
            active: true
            windows: [panelWindow]
            onCleared: content.close()
        }

        AppDrawerContent {
            id: content
            onClosed: {
                GlobalStates.appDrawerOpen = false;
                panelLoader.active = false;
            }
        }
    }
}
```

An `IpcHandler` with `target: "appDrawer"` provides `toggle()`, `open()`, `close()` for external callers (daemon, CLI, Hyprland bindl).

---

## 6. Pinning Integration

**Reuse `LauncherApps`** (the existing singleton in `ii/services/LauncherApps.qml`) rather than introducing a separate pinned list.

- `LauncherApps.isPinned(appId)` — check if pinned
- `LauncherApps.togglePin(appId)` — pin/unpin
- `LauncherApps.moveToFront/moveLeft/moveRight(appId)` — reorder
- `Config.options.launcher.pinnedApps` — the persisted list

The app drawer's pinned section renders the same data as the waffle start menu's pinned section: `Config.options.launcher.pinnedApps.map(id => DesktopEntries.byId(id))`. This means pinning from either surface (drawer or start menu) affects both. This is desirable — the user's "favorites" are a single canonical set.

If in the future a user wants separate pin lists per surface, we can add `Config.options.appDrawer.pinnedApps` as a separate array. But for V1, reusing `launcher.pinnedApps` avoids config duplication and mental model confusion.

---

## 7. Relationship to Existing Start Menu / Search

| Feature | Start Menu (Waffle) | Search (ii) | App Drawer (new) |
|---------|-------------------|-------------|-------------------|
| Invoked by | Super key (press/release) | Super key (press/release) | Super+A or swipe-up |
| Primary UX | Search-first, with pinned + all-apps tabs | Search-only | Browse-first, no typing required |
| Full-screen | No (panel-sized) | No (panel-sized) | Yes |
| Grid layout | Small grid in tab | N/A | Large full-screen grid |
| Pinned apps | Same list (`launcher.pinnedApps`) | N/A | Same list |

The app drawer is designed to **co-exist** with both. When `GlobalStates.appDrawerOpen` becomes `true`, we should also set `GlobalStates.searchOpen = false` (and vice versa) to prevent overlapping overlays. This is the same mutual exclusion pattern used by other globals — add to `GlobalStates`:

```qml
onAppDrawerOpenChanged: {
    if (appDrawerOpen) {
        searchOpen = false;
        overviewOpen = false;
        // etc.
    }
}
onSearchOpenChanged: {
    if (searchOpen) appDrawerOpen = false;
}
```

---

## 8. Internal Architecture

### `AppDrawerContent.qml`

```
┌─────────────────────────────────────────┐
│  AppDrawerSearchBar                      │  ← optional, filter-by-name
│  [🔍 Search apps...]                     │
├─────────────────────────────────────────┤
│  Pinned row                              │
│  ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐     │  ← AppDrawerGrid
│  │ pin│ │ pin│ │ pin│ │ pin│ │ pin│     │     with pinned entries
│  └────┘ └────┘ └────┘ └────┘ └────┘     │
├─────────────────────────────────────────┤
│  All Apps (scrollable grid)              │
│  ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌─┐│  ← AppDrawerGrid
│  │ app│ │ app│ │ app│ │ app│ │ app│ │ ││     with AppSearch.list
│  └────┘ └────┘ └────┘ └────┘ └────┘ └─┘│
│  ┌────┐ ┌────┐ ┌────┐ ...               │
│  │    │ │    │ │    │                    │
│  └────┘ └────┘ └────┘                    │
└─────────────────────────────────────────┘
```

- **Background**: Semi-transparent blur overlay (same approach as `WaffleBackground.qml` — layer at `WlrLayer.Bottom` with darkened scrim, or a translucent `Item` with `MultiEffect { blur ... }` inside the panel).
- **Search bar**: Filters `AppSearch.list` using `AppSearch.fuzzyQuery()`. When empty, shows all apps. Optional per config.
- **Pinned section**: Uses `Config.options.launcher.pinnedApps` → `DesktopEntries.byId()` lookups.
- **All-apps grid**: Uses `AppSearch.list` (already deduped desktop entries). Alphabetically sorted.
- **AppDrawerButton**: Adapted from `StartAppButton.qml` but with larger icon size (48px vs 34px) for touch-friendly targets.

### Keyboard Navigation

- `Esc` closes the drawer
- Arrow keys move focus between grid cells
- `Enter` launches the focused app
- Typing characters auto-focuses the search bar and filters (same pattern as `StartMenuContent.qml`)

---

## 9. Daemon Changes Required

For **touchpad gesture support** only. The core feature (keybind-triggered) is pure QML.

- **snry-daemon** would need a `libinput` gesture listener (e.g., via `libinput` bindings or `python-evdev`) that detects a 4-finger swipe-up or 3-finger swipe-up and calls `quickshell appDrawer toggle`.
- This is identical to how Hyprland binds work: the daemon sends an IPC message to QuickShell via the `IpcHandler` we register.
- **No QML-side changes are needed** — the `IpcHandler` for `appDrawer` already accepts external open/close/toggle commands.
- Implementing gesture support is a separate feature milestone and should not block V1.

---

## 10. Config Schema

Add inside `configOptionsJsonAdapter` in `Config.qml`:

```qml
property JsonObject appDrawer: JsonObject {
    property bool enabled: true
    property string trigger: "keybind"    // "keybind" | "swipe-up" | "both"
    property int columns: 6
    property int pinnedColumns: 6         // 0 = same as columns
    property bool showSearchBar: true
    property real scrimOpacity: 0.5       // Background dimming
    property real blurRadius: 60           // Background blur (if supported)
}
```

---

## 11. Key Design Decisions & Rationale

1. **Shared module, not family-specific**: The drawer is a fundamental DE interaction (like overview, lock, OSK). Putting it in `common/panels/` means both families get it immediately. Visual differences (fonts, colors) are already handled by `Appearance.qml` / `Looks.qml` which are family-aware singletons.

2. **Reuse `LauncherApps` for pinning**: A single source of truth for "pinned apps" reduces confusion. Users expect one set of favorites. The alternative (separate drawer pinned list) creates UX inconsistency and extra config surface area for little gain.

3. **Full-screen overlay, not a dropdown**: An Android-style drawer covers the entire screen to maximize the grid area and reduce visual clutter from the desktop behind it. This is distinct from the existing start menu which is a compact panel.

4. **Keybind trigger for V1**: Pure QML, no daemon dependency. Gesture support can be layered on later via the IPC mechanism.

5. **Mutual exclusion via `GlobalStates`**: Following the existing pattern where only one overlay is open at a time. Adding `appDrawerOpen` to `GlobalStates` and closing others when it opens keeps the UX clean.

6. **Reuses `AppSearch.list` and `AppSearch.fuzzyQuery()`**: No duplication of desktop entry discovery or search logic. The drawer's optional search bar simply passes queries through the existing fuzzy search service.

7. **`HyprlandFocusGrab` for close-on-click-outside**: Consistent with how all other overlays (start menu, overview, etc.) handle dismissal. Clicking outside or pressing Esc closes the drawer.

8. **`PanelWindow` with `exclusiveZone: 0`**: The drawer is an overlay that does not reserve space for the bar or other panels. It renders at `WlrLayer.Top` (above normal windows, below lock/OSK).