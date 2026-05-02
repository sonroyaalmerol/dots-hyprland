package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/xdg"
)

func TestConfigPaths(t *testing.T) {
	cfg := Config{
		RepoRoot: "/opt/snry-shell",
		Home:     "/home/testuser",
		XDG: xdg.Paths{
			ConfigHome: "/home/testuser/.config",
			CacheHome:  "/home/testuser/.cache",
			DataHome:   "/home/testuser/.local/share",
			StateHome:  "/home/testuser/.local/state",
			BinHome:    "/home/testuser/.local/bin",
		},
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"ConfigsDir", cfg.ConfigsDir(), "/opt/snry-shell/configs"},
		{"DataDir", cfg.DataDir(), "/opt/snry-shell/data"},
		{"DotsConfDir", cfg.DotsConfDir(), "/home/testuser/.config/snry-shell"},
		{"VenvPath", cfg.VenvPath(), "/home/testuser/.local/state/quickshell/.venv"},
		{"BackupDir", cfg.BackupDir(), "/home/testuser/ii-original-dots-backup"},
		{"FirstrunFile", cfg.FirstrunFile(), "/home/testuser/.config/snry-shell/installed_true"},
		{"InstalledListfile", cfg.InstalledListfile(), "/home/testuser/.config/snry-shell/installed_listfile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestIsFirstrun(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		RepoRoot: dir,
		Home:     dir,
		XDG: xdg.Paths{
			ConfigHome: filepath.Join(dir, ".config"),
		},
	}

	if !cfg.IsFirstrun() {
		t.Error("expected firstrun=true when marker doesn't exist")
	}

	os.MkdirAll(filepath.Dir(cfg.FirstrunFile()), 0o755)
	os.WriteFile(cfg.FirstrunFile(), []byte{}, 0o644)

	if cfg.IsFirstrun() {
		t.Error("expected firstrun=false when marker exists")
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()

	existing := filepath.Join(dir, "exists.txt")
	os.WriteFile(existing, []byte("test"), 0o644)

	if !fileExists(existing) {
		t.Error("expected true for existing file")
	}
	if fileExists(filepath.Join(dir, "nope.txt")) {
		t.Error("expected false for non-existent file")
	}
}

func TestArchPackages(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "arch")
	os.MkdirAll(pkgDir, 0o755)

	content := `# comment
hyprland
quickshell-git
# another comment

fontconfig
`
	os.WriteFile(filepath.Join(pkgDir, "packages.conf"), []byte(content), 0o644)

	pkgs, err := ArchPackages(dir)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"hyprland", "quickshell-git", "fontconfig"}
	if len(pkgs) != len(expected) {
		t.Fatalf("got %d packages, want %d: %v", len(pkgs), len(expected), pkgs)
	}
	for i, p := range pkgs {
		if p != expected[i] {
			t.Errorf("pkg[%d]: got %q, want %q", i, p, expected[i])
		}
	}
}

func TestArchPackagesSkipsEmptyAndComments(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "arch")
	os.MkdirAll(pkgDir, 0o755)

	os.WriteFile(filepath.Join(pkgDir, "packages.conf"), []byte("\n\n# only comments\n\n"), 0o644)

	pkgs, err := ArchPackages(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected 0 packages, got %d: %v", len(pkgs), pkgs)
	}
}

func TestLoadFedoraDeps(t *testing.T) {
	dir := t.TempDir()
	fedDir := filepath.Join(dir, "fedora")
	os.MkdirAll(fedDir, 0o755)

	toml := `
[copr]
repos = ["user/repo1", "user/repo2"]

[groups.core]
packages = ["pkg1", "pkg2"]

[groups.extra]
packages = ["pkg3"]
install_opts = ["--allowerasing"]
`
	os.WriteFile(filepath.Join(fedDir, "feddeps.toml"), []byte(toml), 0o644)

	deps, err := LoadFedoraDeps(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(deps.COPR.Repos) != 2 {
		t.Errorf("expected 2 copr repos, got %d", len(deps.COPR.Repos))
	}
	if len(deps.Groups["core"].Packages) != 2 {
		t.Errorf("expected 2 core packages, got %d", len(deps.Groups["core"].Packages))
	}
	if len(deps.Groups["extra"].InstallOpts) != 1 {
		t.Errorf("expected 1 install_opt, got %d", len(deps.Groups["extra"].InstallOpts))
	}
}

func TestPythonRequirements(t *testing.T) {
	dir := t.TempDir()
	got := PythonRequirements(dir)
	want := filepath.Join(dir, "python", "requirements.txt")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStepRunner(t *testing.T) {
	// Test: runner stops on first non-optional error
	steps := []Step{
		{Name: "step1", Fn: func(ctx context.Context) error { return nil }},
		{Name: "step2", Fn: func(ctx context.Context) error { return fmt.Errorf("boom") }},
		{Name: "step3", Fn: func(ctx context.Context) error { return nil }}, // never reached
	}

	results := RunSteps(context.Background(), steps, nil)

	if len(results) != 2 {
		t.Fatalf("expected 2 results (stops at step2), got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("step1 should succeed: %v", results[0].Err)
	}
	if results[1].Err == nil {
		t.Error("step2 should fail")
	}
	if results[1].Name != "step2" {
		t.Errorf("step2 name: got %q", results[1].Name)
	}
}

func TestStepRunnerOptional(t *testing.T) {
	steps := []Step{
		{Name: "step1", Fn: func(ctx context.Context) error { return nil }},
		{Name: "step2", Fn: func(ctx context.Context) error { return fmt.Errorf("boom") }, Optional: true},
		{Name: "step3", Fn: func(ctx context.Context) error { return nil }},
	}

	results := RunSteps(context.Background(), steps, nil)

	if len(results) != 3 {
		t.Fatalf("expected 3 results (optional step2 doesn't stop), got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("step1 should succeed: %v", results[0].Err)
	}
	// Optional step2 errors are cleared
	if results[1].Err != nil {
		t.Errorf("optional step2 error should be cleared: %v", results[1].Err)
	}
	if results[2].Err != nil {
		t.Errorf("step3 should succeed: %v", results[2].Err)
	}
}

func TestEnsureDir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "c")

	if err := EnsureDir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDir() {
		t.Error("expected directory")
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "test.txt")

	if err := WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q, want %q", string(data), "hello")
	}
}

func TestLineInFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// File doesn't exist yet — should create and add line
	if err := LineInFile(path, "line1"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "line1") {
		t.Errorf("line1 should be in file, got: %q", string(data))
	}

	// Add same line again — should not duplicate
	if err := LineInFile(path, "line1"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	count := strings.Count(string(data), "line1")
	if count != 1 {
		t.Errorf("expected 1 occurrence, got %d", count)
	}

	// Add different line
	if err := LineInFile(path, "line2"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "line2") {
		t.Errorf("line2 should be in file, got: %q", string(data))
	}
}
