# Smart Config Sync Engine

## Overview

The smart config sync engine (`internal/syncengine/`) handles all config file deployment for snry-shell. It replaces simple file copying with a three-way merge system that preserves user customizations across updates.

## Package Layout

```
internal/syncengine/
├── engine.go          # SyncEngine orchestrator — Run(), syncFile(), decide()
├── manifest.go        # JSON manifest with SHA256 checksums, atomic load/save
├── categorize.go      # Priority-ordered rule matching (file path → strategy)
├── template.go        # Safe string substitution (not text/template)
├── conflict.go        # .orig/.new backup files + JSON-lines conflict log
├── hyprparse/
│   ├── parser.go      # Line-oriented Hyprland config parser
│   └── merge.go       # Section-aware three-way merge
├── kvparse/
│   ├── parser.go      # Generic key=value INI parser with sections
│   └── merge.go       # Three-way key-value merge
└── sectionparse/
    ├── parser.go      # Marker-block (# >>> snry-shell >>>) parser
    └── merge.go       # Managed-block merge via KV merge
```

## Data Flow

```
  Upstream (repo configs/)           Deployed (XDG paths)
         │                                  │
         │  ┌──────────────────────┐        │
         └──▶│   SyncEngine.Run()   │◀──────┘
             │                      │
             │ 1. Load manifest    │
             │ 2. For each file:   │
             │    a. Categorize    │
             │    b. Read 3 copies │
             │    c. Compute SHA256│
             │    d. Decide        │
             │    e. Merge/overwrite│
             │    f. Atomic write  │
             │    g. Update manifest│
             │ 3. Save manifest   │
             └──────────────────────┘
```

## Core Types

### SyncStep

```go
type SyncStep struct {
    UpstreamPath string       // repo source file
    DeployPath   string       // destination on disk
    RelPath      string       // manifest key + categorization path
    Strategy     SyncStrategy // optional override (auto-detected if empty)
}
```

### SyncDecision

The three-checksum comparison result:

| Decision | Condition | Action |
|----------|-----------|--------|
| `DecisionNew` | File doesn't exist on disk | Deploy upstream |
| `DecisionNoop` | Both current and upstream match original | Skip |
| `DecisionUpdate` | User unchanged, upstream changed | Take upstream |
| `DecisionKeep` | User changed, upstream unchanged | Keep user's |
| `DecisionConflict` | Both changed differently | Attempt merge, fallback to user's version |

### FileEntry (manifest)

```go
type FileEntry struct {
    RelPath        string `json:"relPath"`
    Strategy       string `json:"strategy"`
    OriginalSHA256 string `json:"originalSha256"`
    CurrentSHA256  string `json:"currentSha256"`
    UpstreamSHA256 string `json:"upstreamSha256"`
    DeployPath     string `json:"deployPath"`
    UpstreamPath   string `json:"upstreamPath"`
    LastSynced     string `json:"lastSynced"`
    Conflict       bool   `json:"conflict"`
}
```

The manifest lives at `$XDG_CONFIG_HOME/snry-shell/sync-manifest.json`.

## Merge Strategies

### overwrite

Always replace with upstream version. Used for binary assets (SVGs, PNGs, fonts) and quickshell QML files.

### merge-hyprland

Section-aware three-way merge for Hyprland config files. Uses `hyprparse` to parse into an AST with sections, key-values, binds, and exec directives. The merge algorithm:

1. Walk sections present in upstream, merge key-values within each using `threeWayMergeKV`
2. Preserve user sections not in upstream (if they were user-added)
3. Merge top-level binds/execs as sets (union of additions)
4. Preserve user comments and formatting

### merge-kv

Key-value level three-way merge for INI/TOML-style configs. Uses `kvparse` to parse `key = value` pairs (with optional `[section]` headers). The merge operates on key-value maps:

- Keys added by upstream → included
- Keys deleted by user → removed (user intent wins)
- Keys changed by only one side → take that side's value
- Same key changed by both → keep user's version, report conflict

### merge-section

For bashrc-style files with managed marker blocks:

```bash
# >>> snry-shell >>>
... managed content ...
# <<< snry-shell <<<
```

Only the content between markers is merged (using KV merge). Surrounding user content is preserved untouched. On first deploy, content is wrapped in markers.

### skip-if-exists

Only deploy if the file doesn't exist on disk. Used for `monitors.conf` and `workspaces.conf` — user-specific hardware/layout configs that should never be overwritten.

### template

Files containing `{{.User}}`, `{{.Home}}`, etc. have variables substituted first using safe string replacement (NOT Go `text/template`) to avoid mangling matugen's `{{colors.primary.hex}}` syntax. Unknown `{{...}}` patterns are left untouched. After rendering, the underlying strategy applies.

## Categorization Rules

Priority-ordered, first match wins:

| Priority | Pattern | Strategy |
|----------|---------|----------|
| 1 | `hypr/hyprland/*.conf` | `merge-hyprland` |
| 2 | `hypr/hyprland.conf` | `merge-hyprland` |
| 3 | `hypr/hyprlock.conf` | `merge-kv` |
| 4 | `hypr/hypridle.conf` | `merge-kv` |
| 5 | `fuzzel/*.ini` | `merge-kv` |
| 6 | `hypr/monitors.conf` | `skip-if-exists` |
| 7 | `hypr/workspaces.conf` | `skip-if-exists` |
| 8 | `bash/bashrc` | `merge-section` |
| 9 | `bash/bash_profile` | `merge-section` |
| 10 | `bash/zprofile` | `merge-section` |
| 11 | `hypr/hyprland/scripts/*` | `overwrite` |
| 12 | `**/*.svg`, `**/*.png`, `**/*.ttf`, `**/*.otf` | `overwrite` |
| 13 | `fontconfig/**`, `Kvantum/**`, `quickshell/**` | `overwrite` |
| 14 | `hypr/custom/*` | `skip-if-exists` |
| 15 | `matugen/templates/*` | `template` |
| 16 | `**/*.conf`, `**/*.ini` | `merge-kv` |
| 17 | `**` (default) | `overwrite` |

## Conflict Resolution

When both the user and upstream changed the same file:

1. Attempt three-way merge using the file's strategy
2. If merge succeeds (no overlapping key changes) → write merged result
3. If merge fails (same key changed by both):
   - Keep user's current version on disk
   - Write `.orig` file (user's version) alongside deployed file
   - Write `.new` file (upstream version) alongside deployed file
   - Append conflict entry to `snry-shell/conflicts.jsonl`
   - Mark `conflict: true` in manifest entry

## Template Variables

| Variable | Resolved to |
|----------|------------|
| `{{.User}}` | Current username |
| `{{.Home}}` | User home directory |
| `{{.ConfigDir}}` | `$XDG_CONFIG_HOME` |
| `{{.DataDir}}` | `$XDG_DATA_HOME` |
| `{{.StateDir}}` | `$XDG_STATE_HOME` |
| `{{.BinDir}}` | `$HOME/.local/bin` |
| `{{.CacheDir}}` | `$XDG_CACHE_HOME` |
| `{{.RuntimeDir}}` | `$XDG_RUNTIME_DIR` |
| `{{.VenvPath}}` | Python venv path |
| `{{.Fontset}}` | Active fontset name |

## Integration with Manager

The sync engine is wired into `internal/manager/files.go` through two helpers:

```go
func smartSyncSteps(cfg, srcDir, dstDir, relPrefix string) []syncengine.SyncStep
func runSmartSync(cfg Config, steps []syncengine.SyncStep) error
```

Each sync function (`syncQuickshell`, `syncHyprland`, `syncBash`, `syncFontconfig`, `syncMiscConfigs`) builds `SyncStep` lists and calls `runSmartSync`. The `FilesSteps` function includes an `ensure-sync-manifest` step that creates the manifest file if it doesn't exist.

### Files synced through the engine

| Function | Source | Destination |
|----------|--------|-------------|
| `syncQuickshell` | `configs/quickshell/` | `~/.config/quickshell/` |
| `syncHyprland` | `configs/hypr/` (hyprland/, .conf files, custom/) | `~/.config/hypr/` |
| `syncBash` | `configs/bash/` + dotfiles | `~/.config/bash/` + `~/` |
| `syncFontconfig` | `configs/fontconfig/` (or fontset) | `~/.config/fontconfig/` |
| `syncMiscConfigs` | fuzzel, wlogout, foot, ghostty, Kvantum, matugen, mpv, kde-material-you-colors, zshrc.d, xdg-desktop-portal + individual files (starship.toml, darklyrc, dolphinrc, kdeglobals, konsolerc, *-flags.conf) + konsole profile | `~/.config/` + `~/.local/share/konsole/` |

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Manifest missing | Create empty manifest |
| Manifest corrupt | Rename to `.bak`, create empty manifest |
| Upstream file missing | Report error for that file, continue |
| Parse error | Fall back to overwrite strategy |
| Merge conflict | Keep user's version, write backups, log conflict |
| Write error | Report error, continue with remaining files |
| Template error | Skip substitution, use file as-is |

All file writes are atomic (write to temp file, rename). The manifest is saved after each file sync for crash safety.
