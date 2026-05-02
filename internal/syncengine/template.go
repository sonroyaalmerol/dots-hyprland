package syncengine

import (
	"os"
	"strings"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/manager"
)

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

func ResolveTemplateVars(cfg manager.Config) TemplateVars {
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("LOGNAME")
	}
	return TemplateVars{
		User:       user,
		Home:       cfg.Home,
		ConfigDir:  cfg.XDG.ConfigHome,
		DataDir:    cfg.XDG.DataHome,
		StateDir:   cfg.XDG.StateHome,
		BinDir:     cfg.XDG.BinHome,
		CacheDir:   cfg.XDG.CacheHome,
		RuntimeDir: cfg.XDG.RuntimeDir,
		VenvPath:   cfg.VenvPath(),
		Fontset:    cfg.FontsetDirName,
		Custom:     make(map[string]string),
	}
}

func RenderTemplate(data []byte, vars TemplateVars) []byte {
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

	for k, v := range vars.Custom {
		replacements["{{."+k+"}}"] = v
	}

	result := string(data)
	for pattern, value := range replacements {
		result = strings.ReplaceAll(result, pattern, value)
	}
	return []byte(result)
}
