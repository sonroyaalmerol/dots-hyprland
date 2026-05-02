# Smart Config Sync Engine — Design Document

## 1. Architecture Overview

### Package Structure

```
internal/
  syncengine/
    engine.go          # SyncEngine top-level API, orchestration
    manifest.go        # Manifest loading, saving, checksum tracking
    categorize.go      # File categorization rules → SyncStrategy
    strategy.go        # Strategy interface and all implementations
    template.go         # Go text/template rendering with XDG variables
    hyprparse/          # Hyprland config parser (separate sub-package)
      ast.go            # AST node types
      lexer.go          # Lexer/tokenizer
      parser.go         # Parser → AST
      merge.go          # Section-aware three-way merge
    kvparse/
      parser.go         # Generic key=value INI/TOML-style parser
      merge.go          # Three-way merge for key-value configs
    sectionparse/
      parser.go         # Marker-block parser for bashrc/zshrc
      merge.go           # Three-way merge preserving marker blocks
    conflict.go         # Conflict logging, .orig/.new file writing
```

### Data Flow

```
  Upstream (repo configs/)           Deployed (XDG paths)
         │                                  │
         │  ┌──────────────────────┐        │
         │  │   SyncEngine.Run()   │        │
         └──▶│                      │◀──────┘
             │ 1. Load manifest    │
             │ 2. Categorize files │
             │ 3. For each file:   │
             │    a. Read 3 versions│
             │    b. Pick strategy  │
             │    c. Apply merge    │
             │    d. Write result   │
             │    e. Update manifest│
             │ 3. Save manifest    │
             └──────────────────────┘
                    │
               ┌────┴────┐
               │Manifest  │
               │  (JSON)  │
               └─────────┘
```

---

## 2. Data Types and Interfaces

### Core Types

```go
// SyncStrategy determines how a file is synchronized.
type SyncStrategy string

const (
	StrategyOverwrite     SyncStrategy = "overwrite"
	StrategyMergeHyprland SyncStrategy = "merge-hyprland"
	StrategyMergeKV       SyncStrategy = "merge-kv"
	StrategyMergeSection   SyncStrategy = "merge-section"
	StrategyTemplate       SyncStrategy = "template"
	StrategySkipIfExists   SyncStrategy = "skip-if-exists"
)

// SyncDecision is the result of comparing three checksums.
type SyncDecision int

const (
	DecisionNoop      SyncDecision = iota // current==original && upstream==original
	DecisionUpdate                         // current==original && upstream!=original
	DecisionKeep                           // current!=original && upstream==original
	DecisionConflict                       // current!=original && upstream!=original
	DecisionNew                             // file doesn't exist on disk yet
)

// FileEntry tracks one deployed file in the manifest.
type FileEntry struct {
	RelPath        string       `json:"relPath"`        // relative to XDG ConfigHome or Home
	Strategy       SyncStrategy `json:"strategy"`        // categorization
	OriginalSHA256 string       `json:"originalSha256"`  // SHA256 of the version we deployed
	CurrentSHA256  string       `json:"currentSha256"`   // SHA256 currently on disk (at last sync)
	UpstreamSHA256 string       `json:"upstreamSha256"`  // SHA256 of the upstream version (at last sync)
	DeployPath     string       `json:"deployPath"`      // absolute path on disk
	UpstreamPath   string       `json:"upstreamPath"`    // absolute path in repo
	LastSynced     string       `json:"lastSynced"`      // RFC3339 timestamp
	Conflict       bool         `json:"conflict"`        // unresolved conflict marker
}
```

### Strategy Interface

```go
// SyncStep coordinates a single file sync.
type SyncStep struct {
	UpstreamPath string   // repo source
	DeployPath   string   // destination on disk
	RelPath      string   // display/manifest key
	Strategy     SyncStrategy
	Variables    TemplateVars // for template strategy
}

// StrategyHandler processes one file according to its strategy.
type StrategyHandler interface {
	// Merge produces the final file content. orig is the previously deployed
	// version (from the manifest or re-read from .orig), current is what's on
	// disk, upstream is the new repo version.
	Merge(ctx context.Context, step SyncStep, orig, current, upstream []byte) ([]byte, error)
}
```

### Template Variables

```go
type TemplateVars struct {
	User       string
	Home       string
	ConfigDir  string
	DataDir    string
	StateDir   string
	BinDir     string
	CacheDir   string
	RuntimeDir string
	VenvPath   string
	Fontset    string
	Custom     map[string]string
}
```

---

## 3. Manifest Format (JSON Schema)

The manifest lives at `$XDG_CONFIG_HOME/snry-shell/sync-manifest.json`.

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["version", "entries"],
  "properties": {
    "version": { "type": "string", "enum": ["1"] },
    "entries": {
      "type": "object",
      "additionalProperties": {
        "type": "object",
        "required": ["strategy", "originalSha256", "deployPath", "upstreamPath"],
        "properties": {
          "strategy":       { "type": "string" },
          "originalSha256": { "type": "string" },
          "currentSha256":  { "type": "string" },
          "upstreamSha256": { "type": "string" },
          "deployPath":     { "type": "string" },
          "upstreamPath":   { "type": "string" },
          "lastSynced":      { "type": "string", "format": "date-time" },
          "conflict":       { "type": "boolean" }
        }
      }
    }
  }
}
```

Example:

```json
{
  "version": "1",
  "entries": {
    "hypr/hyprland/general.conf": {
      "strategy": "merge-hyprland",
      "originalSha256": "a1b2c3...",
      "currentSha256":  "d4e5f6...",
      "upstreamSha256":  "a1b2c3...",
      "deployPath":     "/home/user/.config/hypr/hyprland/general.conf",
      "upstreamPath":   "/path/to/repo/configs/hypr/hyprland/general.conf",
      "lastSynced":      "2025-10-15T12:00:00Z",
      "conflict": false
    }
  }
}
```

### Checksum Computation

```go
func sha256Of(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
```

---

## 4. File Categorization Rules

Categorization uses a priority-ordered rule list. First match wins.

### Rule Table

| Priority | Pattern (relative to configs/)                          | Strategy           |
|----------|---------------------------------------------------------|--------------------|
| 1        | `hypr/hyprland/*.conf`                                  | `merge-hyprland`  |
| 2        | `hypr/hyprland.conf`                                    | `merge-hyprland`  |
| 3        | `hypr/hyprlock.conf`                                    | `merge-kv`        |
| 4        | `hypr/hypridle.conf`                                    | `merge-kv`        |
| 5        | `fuzzel/*.ini`                                           | `merge-kv`        |
| 6        | `hypr/monitors.conf`                                     | `skip-if-exists`  |
| 7        | `hypr/workspaces.conf`                                  | `skip-if-exists`  |
| 8        | `bash/bashrc`                                           | `merge-section`   |
| 9        | `bash/bash_profile`                                     | `merge-section`   |
| 10       | `bash/zprofile`                                          | `merge-section`   |
| 11       | `hypr/hyprland/scripts/*`                                | `overwrite`       |
| 12       | `**/*.svg`                                               | `overwrite`       |
| 13       | `**/*.png`                                               | `overwrite`       |
| 14       | `**/*.ttf` / `**/*.otf`                                | `overwrite`       |
| 15       | `fontconfig/**`                                          | `overwrite`       |
| 16       | `Kvantum/**`                                             | `overwrite`       |
| 17       | `quickshell/**`                                          | `overwrite`       |
| 18       | `hypr/custom/*`                                          | `skip-if-exists`  |
| 19       | `matugen/templates/*`                                    | `template`        |
| 20       | `**/*.conf` (catch-all)                                  | `merge-kv`        |
| 21       | `**/*.ini` (catch-all)                                   | `merge-kv`        |
| 22       | `**` (default)                                           | `overwrite`       |

### Categorization API

```go
type Categorizer struct {
	Rules []CategorizeRule
}

type CategorizeRule struct {
	Pattern  globPattern // supports ** for recursive
	Strategy SyncStrategy
}

func (c *Categorizer) Categorize(relPath string) SyncStrategy
```

The `Categorizer` is populated from a hardcoded default table. A future version may allow user overrides via `snry-shell/config.json`.

### Template Variable Detection

A file is additionally flagged as `template` if any of these markers appear in the upstream source:

- `{{.User}}`
- `{{.Home}}`
- `{{.ConfigDir}}`
- `{{.DataDir}}`
- `{{.StateDir}}`
- `{{.BinDir}}`
- `{{.CacheDir}}`
- `{{.RuntimeDir}}`
- `{{.VenvPath}}`
- `{{.Fontset}}`

When `StrategyTemplate` applies, the engine first renders the template, then applies the underlying strategy (overwrite, merge-kv, etc.) to the rendered output. The manifest tracks the underlying strategy so re-syncs work correctly.

---

## 5. Sync Strategy Merge Algorithms

### 5.1 Decision Logic (common to all strategies)

```
func decide(origSHA, currentSHA, upstreamSHA string) SyncDecision:
    if currentSHA == "":
        return DecisionNew

    currentUnchanged := (currentSHA == origSHA)
    upstreamUnchanged := (upstreamSHA == origSHA)

    if currentUnchanged && upstreamUnchanged:
        return DecisionNoop
    if currentUnchanged && !upstreamUnchanged:
        return DecisionUpdate
    if !currentUnchanged && upstreamUnchanged:
        return DecisionKeep
    if !currentUnchanged && !upstreamUnchanged:
        return DecisionConflict
```

For `overwrite` strategy, `DecisionConflict` is treated as `DecisionUpdate` (always overwrite).

For `skip-if-exists`, `DecisionNew` triggers deployment; all other decisions result in a no-op (file stays as-is).

### 5.2 Overwrite Strategy

```
Merge(step, orig, current, upstream):
    decision = decide(...)
    if decision == Noop || decision == Keep:
        return current, nil
    # Update, Conflict, New → always replace
    return upstream, nil
```

### 5.3 merge-hyprland Strategy

Parse all three versions into Hyprland ASTs, then perform a section-aware three-way merge.

```
Merge(step, orig, current, upstream):
    decision = decide(...)
    if decision == Noop:   return current, nil
    if decision == Keep:    return current, nil
    if decision == Update:  return upstream, nil

    # DecisionConflict — three-way merge
    origAST      = hyprparse.Parse(orig)
    currentAST   = hyprparse.Parse(current)
    upstreamAST  = hyprparse.Parse(upstream)

    result = &AST{}

    # 1. Walk sections present in upstream
    for each section S in upstreamAST:
        origSection     = origAST.FindSection(S.Name)
        currentSection  = currentAST.FindSection(S.Name)

        if origSection == nil:
            # Section is new in upstream — include as-is
            result.AddSection(S)
            continue

        # Build key-value map for each version
        origKV    = origSection.KeyValueMap()
        currentKV = currentSection.KeyValueMap()
        upKV      = S.KeyValueMap()

        mergedKV = threeWayMergeKV(origKV, currentKV, upKV)
        result.AddSectionWithKV(S.Name, mergedKV, preserveCommentsFrom(currentSection))

    # 2. Sections in current not in upstream
    for each section S in currentAST:
        if upstreamAST.FindSection(S.Name) == nil:
            # Section was removed upstream or added by user
            if origAST.FindSection(S.Name) != nil:
                # Was in original, removed upstream → drop
                continue
            # Added by user → keep
            result.AddSection(S)

    # 3. Top-level key=value entries (outside sections)
    origTopKV    = origAST.TopLevelKV()
    currentTopKV = currentAST.TopLevelKV()
    upTopKV      = upstreamAST.TopLevelKV()
    mergedTopKV  = threeWayMergeKV(origTopKV, currentTopKV, upTopKV)
    result.SetTopLevelKV(mergedTopKV)

    # 4. Top-level bind/exec/exec-once lines (independent, not kv)
    mergedBinds = threeWayMergeLines(
        origAST.TopLevelBinds(),
        currentAST.TopLevelBinds(),
        upstreamAST.TopLevelBinds(),
    )
    result.SetTopLevelBinds(mergedBinds)

    # 5. Source directives — preserve current, append new upstream ones
    result.MergeSources(currentAST.Sources(), upstreamAST.Sources())

    # 6. Comments and blank lines — preserve current's where possible
    result.PreserveFormatting(currentAST)

    return hyprparse.Serialize(result), nil
```

#### threeWayMergeKV (shared utility)

```
threeWayMergeKV(orig, current, upstream map[string]string) map[string]string:
    result = {}
    # All keys from all three versions
    allKeys = union(orig.Keys, current.Keys, upstream.Keys)

    for key in allKeys:
        inOrig = key in orig
        inCurrent = key in current
        inUpstream = key in upstream

        if !inOrig && !inCurrent && inUpstream:
            result[key] = upstream[key]  // new upstream key
        elif !inOrig && inCurrent && !inUpstream:
            result[key] = current[key]    // user-added key
        elif !inOrig && inCurrent && inUpstream:
            if current[key] == upstream[key]:
                result[key] = current[key]  // same addition
            else:
                result[key] = current[key]  // prefer user's version
                // log conflict
        elif inOrig && !inCurrent && !inUpstream:
            // deleted by both → omit
        elif inOrig && !inCurrent && inUpstream:
            // user deleted, upstream changed → delete (user intent wins)
        elif inOrig && inCurrent && !inUpstream:
            // upstream deleted → delete (upstream intent wins)
            // unless current value differs from original
            if current[key] != orig[key]:
                result[key] = current[key]  // user modified deleted key → keep
        elif inOrig && inCurrent && inUpstream:
            if current[key] == orig[key]:
                result[key] = upstream[key]  // user didn't change → take upstream
            elif upstream[key] == orig[key]:
                result[key] = current[key]   // upstream didn't change → keep user
            else:
                # Both changed differently → conflict
                # Write user's version, log conflict marker
                result[key] = current[key]
                // emit conflict entry
        else:
            result[key] = upstream[key]  // safe default

    return result
```

#### threeWayMergeLines (for bind/exec lines)

```
threeWayMergeLines(orig, current, upstream []Line) []Line:
    # Treat each line as an independent entry identified by its content
    # (minus comments). Use set operations.

    origSet     = set(orig)
    currentSet  = set(current)
    upstreamSet = set(upstream)

    result = []

    # Keep all upstream lines
    for line in upstream:
        result.append(line)

    # Add current-only lines (user additions)
    for line in current:
        if line not in origSet and line not in upstreamSet:
            result.append(line)

    # Lines in orig but not in current (user deleted) and not in upstream
    # → already excluded, which is correct

    # Lines in orig and current but not in upstream (upstream deleted)
    # → excluded from result, which is correct

    return result
```

### 5.4 merge-kv Strategy

Used for INI/TOML-style configs (hyprlock.conf, fuzzel.ini, hypridle.conf, etc.).

```
Merge(step, orig, current, upstream):
    decision = decide(...)
    if decision == Noop || decision == Keep:
        return current, nil
    if decision == Update:
        return upstream, nil

    # Conflict — three-way merge at key level
    origKV    = kvparse.Parse(orig)
    currentKV = kvparse.Parse(current)
    upKV      = kvparse.Parse(upstream)

    mergedKV = threeWayMergeKV(origKV, currentKV, upKV)
    return kvparse.Serialize(mergedKV, currentAST.Formatting()), nil
```

The kv parser supports:
- `[section]` headers (INI sections)
- `key = value` pairs
- `key=value` pairs (no spaces around `=`)
- Blank lines and `#` comments preserved
- `source = path` and `include = path` directives

### 5.5 merge-section Strategy

For bashrc/zshrc-style files with marker blocks:

```bash
# >>> snry-shell >>>
... managed content ...
# <<< snry-shell <<<
```

```
Merge(step, orig, current, upstream):
    decision = decide(...)
    if decision == Noop || decision == Keep:
        return current, nil
    if decision == Update:
        # Replace only the managed block, leave surrounding context
        newBlock = extractBlock(upstream)
        return replaceManagedBlock(current, newBlock), nil

    # Conflict — three-way merge on the managed block content
    origBlock    = extractBlock(orig)
    currentBlock = extractBlock(current)
    upstreamBlock = extractBlock(upstream)

    mergedBlock = threeWayMergeKV(
        kvify(origBlock),
        kvify(currentBlock),
        kvify(upstreamBlock),
    )

    return replaceManagedBlock(current, serialize(mergedBlock)), nil
```

If the file has no marker blocks yet (first deployment), the entire file content is wrapped in markers.

### 5.6 template Strategy

```
Merge(step, orig, current, upstream):
    # First, render the template
    rendered = renderTemplate(upstream, step.Variables)

    # Then determine the underlying strategy
    underlyingStrategy = step.Strategy  # if step.Strategy == "template", default to "overwrite"
    # Actually: the manifest stores the base strategy alongside "template"
    # e.g. "template+merge-kv"

    # Apply the underlying strategy to the rendered content vs current
    renderedUpstream = rendered

    # Re-read the original deployed rendered content from the manifest
    # The manifest stores the SHA256 of the previously rendered version
    orig = readOrigRendered(step)

    return underlyingHandler.Merge(step, orig, current, renderedUpstream)
```

On first deploy, the flow is: render template → write result → save manifest entry with the rendered content's SHA256 as originalSHA256.

### 5.7 skip-if-exists Strategy

```
Merge(step, orig, current, upstream):
    decision = decide(...)
    if decision == New:
        return upstream, nil
    # File exists → keep as-is, no-op
    return current, nil
```

Additionally, the manifest records these files but marks them as `skip-if-exists` so future syncs know not to overwrite.

---

## 6. Hyprland Config Parser — Grammar (EBNF)

```
Config        = { Line } ;
Line          = SectionOpen
              | SectionClose
              | KeyVal
              | BindLine
              | ExecLine
              | SourceLine
              | HyprlangDirective
              | Comment
              | BlankLine ;

SectionOpen   = Identifier , "{" , { Line } ;
SectionClose  = "}" ;

(* e.g. general { ... }, decoration { ... }, input { ... } *)
(* Sections can be nested: layerrule { ... } *)

KeyVal        = Identifier , [Spaces] , "=" , [Spaces] , Value , [Spaces] ;
Value         = { Char | SubValue } ;
SubValue      = Bool | Number | String | RGBColor | VarRef ;
Bool          = "true" | "false" | "yes" | "no" | "on" | "off" ;
Number        = [Sign] , Digits , [ "." , Digits ] ;
String        = '"' , { Char } , '"' | "'" , { Char } , "'" ;
RGBColor      = "rgb(" , HexColor , ")" | "rgba(" , HexColor , ")" ;
VarRef        = "$" , Identifier | "$" , Identifier , [Spaces] , "=" , [Spaces] , Value ;

BindLine      = BindPrefix , [Spaces] , ModKeys , "," , Key , "," , Dispatcher , "," , Param ;
BindPrefix    = "bind" | "bindl" | "binde" | "bindm" ;
(* Extended format: *)
BindLine      |= BindPrefix , [Flags] , "=" , ModKeys , "," , Key , "," , Dispatcher ;
BindLine      |= "bindid"  , [Flags] , "=" , Mod1 , "," , Mod2 , "," , Label , "," , Dispatcher , "," , Param ;
BindLine      |= "bindit"  , "=" , ModKeys , "," , Key , "," , Dispatcher ;
BindLine      |= "binditn" , "=" , ModKeys , "," , Key , "," , Dispatcher ;
Flags         = "d" | "m" | "t" | "n" | "e" | "i" | "s" | Flags+Flags ;

ModKeys        = ModKey , { ["+"] , ModKey } ;
ModKey         = "Super" | "Super+Alt" | "Ctrl" | "Alt" | "Shift" | "None" ;

ExecLine      = "exec-once" , "=" , Command | "exec" , "=" , Command ;
SourceLine    = "source" , "=" , Path ;

HyprlangDirective = "# hyprlang" , Space , ("if" | "endif" | "noerror" | "error") , Space , Value ;
Comment       = "#" , { Char } ;
BlankLine     = { Space } , NewLine ;

Identifier    = Alpha , { Alpha | Digit | "_" | "-" } ;
HexColor      = HexDigit , { HexDigit } ;   (* 6 or 8 hex digits *)
Path          = { Char } ;   (* resolved at parse time *)
Command       = { Char } ;   (* shell command after = *)
```

### AST Node Types

```go
type Node interface {
	nodeMarker()
}

type Document struct {
	Children []Node
}

type Section struct {
	Name     string
	Children []Node
}

type KeyValue struct {
	Key   string
	Value string
	Comment string  // inline comment, if any
}

type BindEntry struct {
	Prefix    string  // "bind", "bindl", "binde", "bindm", "bindid", "bindit", "binditn"
	RawLine   string  // full original line for round-tripping
	Mods      string  // modifier string like "Super"
	Key       string  // key or modifier key
	Dispatcher string  // action
	Param     string  // parameter
}

type ExecEntry struct {
	IsOnce   bool
	Command  string
	RawLine  string
}

type SourceDirective struct {
	Path     string
	RawLine  string
}

type HyprlangDirective struct {
	Kind     string  // "if", "endif", "noerror", "error"
	Value    string
	RawLine  string
}

type Comment struct {
	Text string
}

type BlankLine struct{}
```

### Parse Algorithm

1. **Tokenize** into lines (Hyprland configs are line-oriented).
2. **Track section nesting** with a stack. When encountering `identifier {`, push a `Section` node. On `}`, pop.
3. **Classify each line** by prefix:
   - Starts with `bind`, `bindl`, `binde`, `bindm`, `bindid`, `bindit`, `binditn` → `BindEntry`
   - Starts with `exec-once=` or `exec=` → `ExecEntry`
   - Starts with `source=` → `SourceDirective`
   - Starts with `# hyprlang` → `HyprlangDirective`
   - Starts with `#` (not `# hyprlang`) → `Comment`
   - Contains `=` and not inside `bind`/`exec` prefix → `KeyValue`
   - Empty or whitespace only → `BlankLine`
4. **Build merge keys**: For `KeyValue`, the key is the merge identifier. For `BindEntry`, the entire line is the identifier (each bind is independent). For `ExecEntry`, the command text is the identifier. For `Section`, the section name is the identifier.

### Round-Trip Guarantee

The serializer must reproduce the input byte-for-byte when no changes are made to the AST. This means:
- Preserve original whitespace/indentation
- Preserve comment placement
- Preserve line endings
- Blank lines are kept as `BlankLine` nodes, not collapsed

---

## 7. Template Variable System

### Variable Resolution

```go
func ResolveTemplateVars(cfg Config) TemplateVars {
	return TemplateVars{
		User:       os.Getenv("USER"),
		Home:       cfg.Home,
		ConfigDir:  cfg.XDG.ConfigHome,
		DataDir:    cfg.XDG.DataHome,
		StateDir:   cfg.XDG.StateHome,
		BinDir:     cfg.XDG.BinHome,
		CacheDir:   cfg.XDG.CacheHome,
		RuntimeDir: cfg.XDG.RuntimeDir,
		VenvPath:   cfg.VenvPath(),
		Fontset:    cfg.FontsetDirName,
		Custom:     loadCustomVars(cfg.DotsConfDir() + "/config.json"),
	}
}

func RenderTemplate(data []byte, vars TemplateVars) ([]byte, error) {
	tmpl, err := template.New("sync").Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	// Strip existing markers like {{...}} that aren't Go templates
	// (e.g. matugen {{colors.primary.default.hex}} — those stay as-is
	// since they don't match our known variable names)
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}
```

### Safety: Only Known Variables

The template engine uses `Option("missingkey", "error")` so that any `{{.Unknown}}` causes a clear error rather than silent empty output. This prevents accidental destruction of content like matugen's `{{colors.background.default.hex_stripped}}`.

However, we need to be smarter: we only render variables that are in our known set. For files that may contain other `{{...}}` syntax (like matugen templates), we use a two-pass approach:

1. **First pass**: Replace only our known variable patterns using simple string substitution, not Go templates, to avoid mangling non-Go-template syntax.
2. **Second pass**: For files explicitly marked as Go templates (rare in this project), use Go `text/template`.

In practice, snry-shell config files don't use Go template syntax today. The matugen templates use `{{...}}` but those are processed by matugen, not by us. Our template system's primary use is substituting paths like `~/.config` → actual XDG paths, and `{{.User}}` → the username.

### Implementation: Safe Variable Substitution

```go
var knownVars = []struct{ Pattern, Value string }{
	{"{{.User}}", ""},
	{"{{.Home}}", ""},
	// etc.
}

func SafeTemplateRender(data []byte, vars TemplateVars) []byte {
	replacements := map[string]string{
		"{{.User}}":       vars.User,
		"{{.Home}}":       vars.Home,
		"{{.ConfigDir}}":  vars.ConfigDir,
		"{{.DataDir}}":    vars.DataDir,
		"{{.StateDir}}":   vars.StateDir,
		"{{.BinDir}}":     vars.BinDir,
		"{{.CacheDir}}":   vars.CacheDir,
		"{{.RuntimeDir}}": vars.RuntimeDir,
		"{{.VenvPath}}":   vars.VenvPath,
		"{{.Fontset}}":    vars.Fontset,
	}

	// Add custom variables
	for k, v := range vars.Custom {
		replacements["{{."+k+"}}"] = v
	}

	result := string(data)
	for pattern, value := range replacements {
		result = strings.ReplaceAll(result, pattern, value)
	}
	return []byte(result)
}
```

---

## 8. Conflict Resolution Flow

```
┌──────────────────────────────────┐
│  SyncEngine processes a file    │
│  decision == DecisionConflict  │
└────────────┬─────────────────────┘
             │
             ▼
┌──────────────────────────────────┐
│  1. Attempt three-way merge      │
│     using the file's strategy    │
└────────────┬─────────────────────┘
              │
       ┌──────┴──────┐
       │  Merge OK?  │
       └──────┬──────┘
         Yes / \ No
            /   \
           ▼     ▼
    ┌─────────┐  ┌──────────────────────────────────────┐
    │  Write  │  │  Conflict!                            │
    │ result  │  │  1. Write deployPath.orig = current  │
    │         │  │  2. Write deployPath.new  = upstream  │
    └─────────┘  │  3. Keep current version on disk     │
                 │  4. Log conflict to:                  │
                 │     snry-shell/conflicts.jsonl        │
                 │  5. Mark entry.conflict = true        │
                 │  6. Continue to next file             │
                 └──────────────────────────────────────┘
```

### Conflict Log Format

One JSON-lines entry per conflict, appended to `$XDG_CONFIG_HOME/snry-shell/conflicts.jsonl`:

```json
{
  "timestamp": "2025-10-15T14:30:00Z",
  "relPath": "hypr/hyprland/general.conf",
  "strategy": "merge-hyprland",
  "reason": "both modified: keys [gaps_in, border_size]",
  "origSHA": "a1b2c3...",
  "currentSHA": "d4e5f6...",
  "upstreamSHA": "g7h8i9...",
  "origFile": "/home/user/.config/hypr/hyprland/general.conf.orig",
  "newFile": "/home/user/.config/hypr/hyprland/general.conf.new"
}
```

The `.orig` and `.new` files are always written alongside the deployed path so the user can manually inspect and merge.

### Auto-merge Details

The three-way merge succeeds when:
- All key-level changes are non-overlapping (user changed key A, upstream changed key B)
- Structure-level changes are non-overlapping (user added a section, upstream modified a different section)

The merge **fails** (produces a conflict) when:
- Same key changed in both current and upstream with different values
- Same section has conflicting additions/deletions

In merge failure cases, the engine keeps the **user's current version** on disk, writes `.orig` and `.new` backups, and logs the conflict. The sync continues — it does not block.

---

## 9. Error Handling

### Error Categories

| Category     | Behavior                                                            |
|--------------|---------------------------------------------------------------------|
| Read Error   | Log warning, skip file, continue sync                               |
| Parse Error  | Fall back to `overwrite` strategy for that file, log warning       |
| Merge Error  | Write .orig/.new, keep current, log conflict, continue              |
| Write Error  | Log error, mark file as failed in results, continue with others    |
| Manifest Error| If missing/corrupt, create new manifest; log warning               |
| Template Error| Skip variable substitution, use file as-is, log warning           |

### Implementation

```go
type SyncResult struct {
	RelPath  string
	Decision SyncDecision
	Strategy SyncStrategy
	Err      error
	Conflict *ConflictInfo  // non-nil if conflict occurred
}

func (e *SyncEngine) Run(ctx context.Context, steps []SyncStep) []SyncResult {
	results := make([]SyncResult, 0, len(steps))
	for _, step := range steps {
		select {
		case <-ctx.Done():
			return results
		default:
		}

		result := e.syncFile(ctx, step)
		results = append(results, result)
	}
	return results
}

func (e *SyncEngine) syncFile(ctx context.Context, step SyncStep) SyncResult {
	// ... decision logic, strategy dispatch, error handling
	// All errors are captured in the result, never panic
}
```

### Manifest Corruption Recovery

If the manifest file cannot be parsed:
1. Rename the corrupted file to `sync-manifest.json.bak`
2. Start with an empty manifest
3. Treat every file as `DecisionNew` (first-time deployment behavior)
4. Log a warning to stderr

### Atomic Writes

All file writes use a write-then-rename pattern to avoid partial files:

```go
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".sync-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
```

---

## 10. Integration with Existing Code

### Changes to `internal/manager/filesync.go`

The current `SyncDirectory` and `CopyFile` functions remain for backward compatibility (used by `overwrite` strategy and non-config files). New smart sync logic lives in `internal/syncengine/`. The `filesync.go` file gains a thin adapter:

```go
// SmartSync replaces the naive SyncDirectory for config files that need
// intelligent merging. It delegates to the sync engine.
func SmartSync(ctx context.Context, cfg Config, steps []syncengine.SyncStep) []syncengine.SyncResult {
	engine := syncengine.New(syncengine.Config{
		ManifestPath: cfg.DotsConfDir() + "/sync-manifest.json",
		Variables:     syncengine.ResolveTemplateVars(cfg),
		Categorizer:   syncengine.DefaultCategorizer(),
	})
	return engine.Run(ctx, steps)
}
```

### Changes to `internal/manager/files.go`

The `syncHyprland`, `syncBash`, `syncMiscConfigs`, and other sync functions are restructured. Instead of calling `SyncDirectory` with delete, they build `SyncStep` lists and call `SmartSync`:

```go
func syncHyprland(cfg Config) error {
	steps := buildHyprlandSteps(cfg)
	results := SmartSync(context.Background(), cfg, steps)
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "  [sync] %s: %v\n", r.RelPath, r.Err)
		} else if r.Conflict != nil {
			fmt.Fprintf(os.Stderr, "  [conflict] %s: %s\n", r.RelPath, r.Conflict.Reason)
		}
	}
	return nil
}

func buildHyprlandSteps(cfg Config) []syncengine.SyncStep {
	var steps []syncengine.SyncStep
	// Walk configs/hypr/hyprland/ and create steps
	filepath.Walk(cfg.ConfigsDir()+"/hypr/hyprland", func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		relPath := "hypr/hyprland/" + strings.TrimPrefix(path, cfg.ConfigsDir()+"/hypr/hyprland/")
		steps = append(steps, syncengine.SyncStep{
			UpstreamPath: path,
			DeployPath:   cfg.XDG.ConfigHome + "/hypr/hyprland/" + relPath,
			RelPath:      "hypr/hyprland/" + relPath,
		})
		return nil
	})
	// Also add individual hypr config files
	hyprFiles := []string{"hyprlock.conf", "monitors.conf", "workspaces.conf", "hyprland.conf", "hypridle.conf"}
	for _, f := range hyprFiles {
		steps = append(steps, syncengine.SyncStep{
			UpstreamPath: cfg.ConfigsDir() + "/hypr/" + f,
			DeployPath:   cfg.XDG.ConfigHome + "/hypr/" + f,
			RelPath:      "hypr/" + f,
		})
	}
	return steps
}
```

### Changes to `internal/manager/steps.go`

The `FilesSteps` function remains largely the same. The `syncHyprland` and other functions it calls now use `SmartSync` internally. A new step is added for manifest initialization:

```go
{
	Name: "ensure-sync-manifest",
	Fn: func(ctx context.Context) error {
		return syncengine.EnsureManifest(cfg.DotsConfDir() + "/sync-manifest.json")
	},
},
```

This step runs before the sync steps, creating the manifest file if it doesn't exist.

### Backward Compatibility

- On first run with no manifest present, all files are treated as `DecisionNew` (equivalent to today's `Force` behavior).
- The `Force` flag on `Config` can be mapped to "redeploy everything" by clearing all `currentSHA256` entries in the manifest.
- The `IsFirstrun()` check maps to `DecisionNew` for all files.
- Existing `CopyFile`, `SyncDirectory`, `LineInFile`, and `WriteFile` remain unchanged for non-sync use cases (binary installation, systemd units, etc.).

### Manifest First-Run Migration

```go
func EnsureManifest(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil  // already exists
	}
	// Create initial manifest
	manifest := &Manifest{Version: "1", Entries: make(map[string]FileEntry)}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	return os.WriteFile(path, data, 0o644)
}
```

On the very first sync after upgrade, the manifest is empty. The engine computes SHA256 of the current on-disk files and stores them as both `originalSHA256` and `currentSHA256`. This establishes the baseline. The next sync can then detect user modifications.

---

## Appendix A: Hyprland Config Merge Examples

### Example: User changed `gaps_in`, upstream changed `border_size`

**Original (manifest):**
```
general {
    gaps_in = 4
    gaps_out = 5
    border_size = 1
    col.active_border = rgba(0DB7D455)
}
```

**Current (user modified `gaps_in`):**
```
general {
    gaps_in = 8
    gaps_out = 5
    border_size = 1
    col.active_border = rgba(0DB7D455)
}
```

**Upstream (changed `border_size`):**
```
general {
    gaps_in = 4
    gaps_out = 5
    border_size = 2
    col.active_border = rgba(0DB7D455)
}
```

**Merged result:**
```
general {
    gaps_in = 8
    gaps_out = 5
    border_size = 2
    col.active_border = rgba(0DB7D455)
}
```

### Example: Same key changed by both → conflict

**Current:** `gaps_in = 8`
**Upstream:** `gaps_in = 6`

Result: Keep `gaps_in = 8` (user's version), write `.orig` and `.new` with the conflicting versions, log the conflict.

---

## Appendix B: Section-Merge (bashrc) Example

**Original managed block:**
```bash
# >>> snry-shell >>>
export PATH="$HOME/.local/bin:$PATH"
alias clear="printf '\033[2J\033[3J\033[1;1H'"
# <<< snry-shell <<<
```

**User added an alias inside the block:**
```bash
# >>> snry-shell >>>
export PATH="$HOME/.local/bin:$PATH"
alias clear="printf '\033[2J\033[3J\033[1;1H'"
alias ll='ls -la'
# <<< snry-shell <<<
```

**Upstream changed the PATH:**
```bash
# >>> snry-shell >>>
export PATH="$HOME/.local/bin:$HOME/.cargo/bin:$PATH"
alias clear="printf '\033[2J\033[3J\033[1;1H'"
# <<< snry-shell <<<
```

**Merged result:**
```bash
# >>> snry-shell >>>
export PATH="$HOME/.local/bin:$HOME/.cargo/bin:$PATH"
alias clear="printf '\033[2J\033[3J\033[1;1H'"
alias ll='ls -la'
# <<< snry-shell <<<
```

User's `alias ll` is preserved; upstream's PATH change is incorporated.

---

## Appendix C: Implementation Priority

1. **Checksum manifest** — foundation for all decisions
2. **Categorizer** — needed to dispatch strategies
3. **overwrite strategy** — simplest, validates framework
4. **skip-if-exists** — needed for monitors.conf
5. **merge-kv** — covers fuzzel.ini, hyprlock.conf, hypridle.conf
6. **merge-hyprland** — most complex, needs parser
7. **merge-section** — needed for bashrc
8. **template** — adds variable substitution
9. **Conflict logging** — .orig/.new + jsonl logging
10. **Integration** — wire into existing `files.go`/`steps.go`