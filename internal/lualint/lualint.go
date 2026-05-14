// Package lualint validates generated Hyprland Lua config syntax using
// gopher-lua's parser. It catches syntax errors at generation time rather
// than at runtime when Hyprland loads the file.
package lualint

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// hyprlandStubs is a minimal Lua preamble that stubs out Hyprland's global
// API surface so the parser can validate call syntax without unknown-global
// errors.
const hyprlandStubs = `
hl = {
  monitor = function() end,
  workspace_rule = function() end,
  config = function() end,
  curve = function() end,
  animation = function() end,
  window_rule = function() end,
  layer_rule = function() end,
  bind = function() end,
}
`

// Validate checks that src is syntactically valid Lua. It prepends Hyprland
// API stubs so hl.monitor({...}) / hl.workspace_rule({...}) calls parse
// correctly. Returns nil on success, or an error describing the syntax issue.
func Validate(src string) error {
	L := lua.NewState()
	defer L.Close()

	// Load stubs first so hl.* globals exist.
	if err := L.DoString(hyprlandStubs); err != nil {
		return fmt.Errorf("lualint: stub load: %w", err)
	}

	// Load the user source under a chunk name for better error messages.
	if err := L.DoString(src); err != nil {
		return fmt.Errorf("lualint: %w", err)
	}
	return nil
}
