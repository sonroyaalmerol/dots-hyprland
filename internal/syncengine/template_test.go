package syncengine

import "testing"

func TestRenderAllVariables(t *testing.T) {
	vars := TemplateVars{
		User:       "testuser",
		Home:       "/home/testuser",
		ConfigDir:  "/home/testuser/.config",
		DataDir:    "/home/testuser/.local/share",
		StateDir:   "/home/testuser/.local/state",
		BinDir:     "/home/testuser/.local/bin",
		CacheDir:   "/home/testuser/.cache",
		RuntimeDir: "/run/user/1000",
		VenvPath:   "/home/testuser/.venv",
		Fontset:    "JetBrains Mono",
		Custom:     make(map[string]string),
	}

	input := []byte("user={{.User}} home={{.Home}} config={{.ConfigDir}} data={{.DataDir}} state={{.StateDir}} bin={{.BinDir}} cache={{.CacheDir}} runtime={{.RuntimeDir}} venv={{.VenvPath}} font={{.Fontset}}")

	result := RenderTemplate(input, vars)
	expected := "user=testuser home=/home/testuser config=/home/testuser/.config data=/home/testuser/.local/share state=/home/testuser/.local/state bin=/home/testuser/.local/bin cache=/home/testuser/.cache runtime=/run/user/1000 venv=/home/testuser/.venv font=JetBrains Mono"

	if string(result) != expected {
		t.Errorf("render mismatch:\n got  %q\n want %q", string(result), expected)
	}
}

func TestRenderPreservesUnknown(t *testing.T) {
	vars := TemplateVars{
		User:   "testuser",
		Home:   "/home/testuser",
		Custom: make(map[string]string),
	}

	input := []byte("{{colors.primary.hex}} and {{.User}}")
	result := RenderTemplate(input, vars)

	// matugen variable should be preserved, our variable should be substituted
	if string(result) != "{{colors.primary.hex}} and testuser" {
		t.Errorf("expected matugen var preserved, got %q", string(result))
	}
}

func TestRenderCustomVariables(t *testing.T) {
	vars := TemplateVars{
		User: "testuser",
		Home: "/home/testuser",
		Custom: map[string]string{
			"MyVar":    "hello",
			"OtherVar": "world",
		},
	}

	input := []byte("{{.MyVar}} {{.OtherVar}} {{.User}}")
	result := RenderTemplate(input, vars)

	if string(result) != "hello world testuser" {
		t.Errorf("expected custom vars substituted, got %q", string(result))
	}
}

func TestHasTemplateVariables(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{"has User", "{{.User}}", true},
		{"has Home", "path={{.Home}}", true},
		{"has ConfigDir", "{{.ConfigDir}}", true},
		{"has Fontset", "{{.Fontset}}", true},
		{"mixed", "# User: {{.User}}\nHOME={{.Home}}", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasTemplateVariables([]byte(tt.data)); got != tt.want {
				t.Errorf("HasTemplateVariables(%q) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

func TestHasTemplateVariablesNegative(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"empty", ""},
		{"no vars", "key = value"},
		{"matugen only", "{{colors.primary.hex}}"},
		{"similar but not ours", "{{.Username}}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if HasTemplateVariables([]byte(tt.data)) {
				t.Errorf("HasTemplateVariables(%q) = true, want false", tt.data)
			}
		})
	}
}
