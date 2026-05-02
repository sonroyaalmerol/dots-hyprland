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
		if err == nil && matched {
			return true
		}
		return false
	}

	// Handle ** patterns
	if pattern == "**" {
		return true
	}

	prefix := ""
	suffix := ""
	if before, after, ok := strings.Cut(pattern, "**"); ok {
		prefix = before
		suffix = after
	}

	if prefix != "" {
		if !strings.HasPrefix(path, prefix) {
			return false
		}
	}
	if suffix != "" {
		if !strings.HasSuffix(path, suffix) {
			matched, _ := filepath.Match("*"+suffix, filepath.Base(path))
			if !matched {
				return false
			}
		}
		if strings.Contains(suffix, "/") {
			// suffix contains path separator, check against full path suffix
			if !strings.HasSuffix(path, suffix) {
				return false
			}
		}
	}

	if prefix != "" && suffix != "" {
		rest := path[len(prefix):]
		if !strings.HasSuffix(rest, suffix) {
			return false
		}
	}

	return true
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
