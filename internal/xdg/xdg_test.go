package xdg

import (
	"os"
	"testing"
)

func TestResolveDefaults(t *testing.T) {
	// Save and restore env vars
	orig := map[string]string{}
	for _, key := range []string{
		"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME",
		"XDG_STATE_HOME", "XDG_BIN_HOME", "HOME",
	} {
		orig[key] = os.Getenv(key)
		os.Unsetenv(key)
	}
	defer func() {
		for k, v := range orig {
			os.Setenv(k, v)
		}
	}()

	os.Setenv("HOME", "/home/testuser")

	p := Resolve()

	if p.ConfigHome != "/home/testuser/.config" {
		t.Errorf("ConfigHome: got %q", p.ConfigHome)
	}
	if p.CacheHome != "/home/testuser/.cache" {
		t.Errorf("CacheHome: got %q", p.CacheHome)
	}
	if p.DataHome != "/home/testuser/.local/share" {
		t.Errorf("DataHome: got %q", p.DataHome)
	}
	if p.StateHome != "/home/testuser/.local/state" {
		t.Errorf("StateHome: got %q", p.StateHome)
	}
	if p.BinHome != "/home/testuser/.local/bin" {
		t.Errorf("BinHome: got %q", p.BinHome)
	}
}

func TestResolveOverride(t *testing.T) {
	orig := map[string]string{}
	for _, key := range []string{
		"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME",
		"XDG_STATE_HOME", "XDG_BIN_HOME", "HOME",
	} {
		orig[key] = os.Getenv(key)
	}
	defer func() {
		for k, v := range orig {
			os.Setenv(k, v)
		}
	}()

	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	os.Setenv("XDG_CACHE_HOME", "/custom/cache")
	os.Setenv("XDG_DATA_HOME", "/custom/data")
	os.Setenv("XDG_STATE_HOME", "/custom/state")
	os.Setenv("XDG_BIN_HOME", "/custom/bin")

	p := Resolve()

	if p.ConfigHome != "/custom/config" {
		t.Errorf("ConfigHome: got %q", p.ConfigHome)
	}
	if p.CacheHome != "/custom/cache" {
		t.Errorf("CacheHome: got %q", p.CacheHome)
	}
	if p.DataHome != "/custom/data" {
		t.Errorf("DataHome: got %q", p.DataHome)
	}
	if p.StateHome != "/custom/state" {
		t.Errorf("StateHome: got %q", p.StateHome)
	}
	if p.BinHome != "/custom/bin" {
		t.Errorf("BinHome: got %q", p.BinHome)
	}
}
