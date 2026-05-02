package syncengine

import "testing"

func TestCategorizeHyprlandConf(t *testing.T) {
	c := DefaultCategorizer()
	s := c.Categorize("hypr/hyprland/general.conf")
	if s != StrategyMergeHyprland {
		t.Errorf("expected %q, got %q", StrategyMergeHyprland, s)
	}
}

func TestCategorizeHyprlockConf(t *testing.T) {
	c := DefaultCategorizer()
	s := c.Categorize("hypr/hyprlock.conf")
	if s != StrategyMergeKV {
		t.Errorf("expected %q, got %q", StrategyMergeKV, s)
	}
}

func TestCategorizeHypridleConf(t *testing.T) {
	c := DefaultCategorizer()
	s := c.Categorize("hypr/hypridle.conf")
	if s != StrategyMergeKV {
		t.Errorf("expected %q, got %q", StrategyMergeKV, s)
	}
}

func TestCategorizeFuzzelIni(t *testing.T) {
	c := DefaultCategorizer()
	s := c.Categorize("fuzzel/fuzzel.ini")
	if s != StrategyMergeKV {
		t.Errorf("expected %q, got %q", StrategyMergeKV, s)
	}
}

func TestCategorizeMonitorsConf(t *testing.T) {
	c := DefaultCategorizer()
	s := c.Categorize("hypr/monitors.conf")
	if s != StrategySkipIfExists {
		t.Errorf("expected %q, got %q", StrategySkipIfExists, s)
	}
}

func TestCategorizeBashrc(t *testing.T) {
	c := DefaultCategorizer()
	s := c.Categorize("bash/bashrc")
	if s != StrategyMergeSection {
		t.Errorf("expected %q, got %q", StrategyMergeSection, s)
	}
}

func TestCategorizeSVG(t *testing.T) {
	c := DefaultCategorizer()
	s := c.Categorize("icons/test.svg")
	if s != StrategyOverwrite {
		t.Errorf("expected %q, got %q", StrategyOverwrite, s)
	}
}

func TestCategorizeQuickshell(t *testing.T) {
	c := DefaultCategorizer()
	s := c.Categorize("quickshell/ii/main.qml")
	if s != StrategyOverwrite {
		t.Errorf("expected %q, got %q", StrategyOverwrite, s)
	}
}

func TestCategorizeTemplateDetection(t *testing.T) {
	// Template detection is in HasTemplateVariables, not Categorize.
	// Categorize returns StrategyTemplate for matugen/templates/* paths.
	c := DefaultCategorizer()
	s := c.Categorize("matugen/templates/colorscheme.conf")
	if s != StrategyTemplate {
		t.Errorf("expected %q, got %q", StrategyTemplate, s)
	}
}

func TestCategorizeUnknown(t *testing.T) {
	c := DefaultCategorizer()
	s := c.Categorize("some/random/file.xyz")
	if s != StrategyOverwrite {
		t.Errorf("expected %q, got %q", StrategyOverwrite, s)
	}
}

func TestCategorizeFontconfig(t *testing.T) {
	c := DefaultCategorizer()
	s := c.Categorize("fontconfig/fonts.conf")
	if s != StrategyOverwrite {
		t.Errorf("expected %q, got %q", StrategyOverwrite, s)
	}
}

func TestCategorizeHyprCustom(t *testing.T) {
	c := DefaultCategorizer()
	s := c.Categorize("hypr/custom/rules.conf")
	if s != StrategySkipIfExists {
		t.Errorf("expected %q, got %q", StrategySkipIfExists, s)
	}
}
