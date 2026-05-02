package syncengine

import (
	"os"
	"strings"
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

func ResolveTemplateVars(home, configDir, dataDir, stateDir, binDir, cacheDir, runtimeDir, venvPath, fontset string) TemplateVars {
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("LOGNAME")
	}
	return TemplateVars{
		User:       user,
		Home:       home,
		ConfigDir:  configDir,
		DataDir:    dataDir,
		StateDir:   stateDir,
		BinDir:     binDir,
		CacheDir:   cacheDir,
		RuntimeDir: runtimeDir,
		VenvPath:   venvPath,
		Fontset:    fontset,
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
