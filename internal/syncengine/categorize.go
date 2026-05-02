package syncengine

import (
	"path/filepath"
	"strings"
)

type SyncStrategy string

const (
	StrategyOverwrite     SyncStrategy = "overwrite"
	StrategyMergeHyprland SyncStrategy = "merge-hyprland"
	StrategyMergeKV       SyncStrategy = "merge-kv"
	StrategyMergeSection  SyncStrategy = "merge-section"
	StrategyTemplate      SyncStrategy = "template"
	StrategySkipIfExists  SyncStrategy = "skip-if-exists"
)

type CategorizeRule struct {
	Pattern  string
	Strategy SyncStrategy
}

type Categorizer struct {
	Rules []CategorizeRule
}

func DefaultCategorizer() *Categorizer {
	rules := []CategorizeRule{
		{Pattern: "hypr/hyprland/*.conf", Strategy: StrategyMergeHyprland},
		{Pattern: "hypr/hyprland.conf", Strategy: StrategyMergeHyprland},
		{Pattern: "hypr/hyprlock.conf", Strategy: StrategyMergeKV},
		{Pattern: "hypr/hypridle.conf", Strategy: StrategyMergeKV},
		{Pattern: "fuzzel/*.ini", Strategy: StrategyMergeKV},
		{Pattern: "hypr/monitors.conf", Strategy: StrategySkipIfExists},
		{Pattern: "hypr/workspaces.conf", Strategy: StrategySkipIfExists},
		{Pattern: "bash/bashrc", Strategy: StrategyMergeSection},
		{Pattern: "bash/bash_profile", Strategy: StrategyMergeSection},
		{Pattern: "bash/zprofile", Strategy: StrategyMergeSection},
		{Pattern: "hypr/hyprland/scripts/*", Strategy: StrategyOverwrite},
		{Pattern: "**/*.svg", Strategy: StrategyOverwrite},
		{Pattern: "**/*.png", Strategy: StrategyOverwrite},
		{Pattern: "**/*.ttf", Strategy: StrategyOverwrite},
		{Pattern: "**/*.otf", Strategy: StrategyOverwrite},
		{Pattern: "fontconfig/**", Strategy: StrategyOverwrite},
		{Pattern: "Kvantum/**", Strategy: StrategyOverwrite},
		{Pattern: "quickshell/**", Strategy: StrategyOverwrite},
		{Pattern: "hypr/custom/*", Strategy: StrategySkipIfExists},
		{Pattern: "matugen/templates/*", Strategy: StrategyTemplate},
		{Pattern: "**/*.conf", Strategy: StrategyMergeKV},
		{Pattern: "**/*.ini", Strategy: StrategyMergeKV},
		{Pattern: "**", Strategy: StrategyOverwrite},
	}
	return &Categorizer{Rules: rules}
}

func (c *Categorizer) Categorize(relPath string) SyncStrategy {
	for _, rule := range c.Rules {
		if matchPattern(rule.Pattern, relPath) {
			return rule.Strategy
		}
	}
	return StrategyOverwrite
}

func matchPattern(pattern, path string) bool {
	if !strings.Contains(pattern, "**") {
		matched, err := filepath.Match(pattern, path)
		return err == nil && matched
	}

	if pattern == "**" {
		return true
	}

	before, after, _ := strings.Cut(pattern, "**")

	// The path must start with the prefix (if any)
	if before != "" && !strings.HasPrefix(path, before) {
		return false
	}

	// Handle the suffix after **
	if after == "" {
		return true // pattern is "prefix/**" which matches anything under prefix
	}

	// after is like "/*.conf" or "/*" or "/something"
	// The * in the suffix matches exactly one path segment (no /)
	suffix := after
	if strings.HasPrefix(suffix, "/*") {
		// ** matches any number of segments, then /* matches exactly one segment
		// So the path must end with the part after *, and have a / before it
		suffixExt := suffix[2:] // e.g. ".conf"
		if suffixExt == "" {
			// "**/*" matches any file (at any depth)
			if before != "" {
				return strings.HasPrefix(path, before)
			}
			return true
		}
		// Find last segment of path and check it ends with suffixExt
		lastSlash := strings.LastIndex(path, "/")
		if lastSlash < 0 {
			// No slash — ** matched zero segments, just check filename
			return strings.HasSuffix(path, suffixExt)
		}
		filename := path[lastSlash+1:]
		return strings.HasSuffix(filename, suffixExt)
	}

	// Literal suffix after ** (e.g. "**/foo.conf")
	return strings.HasSuffix(path, suffix)
}

var templateVarPatterns = []string{
	"{{.User}}",
	"{{.Home}}",
	"{{.ConfigDir}}",
	"{{.DataDir}}",
	"{{.StateDir}}",
	"{{.BinDir}}",
	"{{.CacheDir}}",
	"{{.RuntimeDir}}",
	"{{.VenvPath}}",
	"{{.Fontset}}",
}

func HasTemplateVariables(data []byte) bool {
	s := string(data)
	for _, pattern := range templateVarPatterns {
		if strings.Contains(s, pattern) {
			return true
		}
	}
	return false
}
