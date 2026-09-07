package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josephburnett/gridwell/internal/config"
)

// Each plugin entry resolves to a gridwell-plugin-<kind> binary, and a
// missing one is named.
func TestKindsResolvePluginBinaries(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gridwell-plugin-fs"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRIDWELL_PLUGIN_DIR", dir)

	cfg := &config.ServerConfig{Plugins: []config.PluginConfig{
		{ID: "p1", Kind: "fs"},
		{ID: "p3", Kind: "fs", Binary: "/pinned/gridwell-plugin-fs"},
	}}
	if err := resolvePluginBinaries(cfg); err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(cfg.Plugins[0].Binary); got != "gridwell-plugin-fs" {
		t.Fatalf("fs resolved %q", cfg.Plugins[0].Binary)
	}
	if cfg.Plugins[1].Binary != "/pinned/gridwell-plugin-fs" {
		t.Fatalf("an explicit binary: path must survive resolution: %q", cfg.Plugins[1].Binary)
	}
	missing := &config.ServerConfig{Plugins: []config.PluginConfig{{ID: "g1", Label: "todos", Kind: "gitlab"}}}
	if err := resolvePluginBinaries(missing); err == nil || !strings.Contains(err.Error(), "gridwell-plugin-gitlab") {
		t.Fatalf("a missing plugin binary must be named: %v", err)
	}
}

// The two platform facts about a built binary are pure functions of GOOS, so
// the Windows answers are checkable from a Linux dev box. Get either wrong
// and every plugin is unresolvable on the Windows artifact.
func TestBinaryShapePerPlatform(t *testing.T) {
	for _, tc := range []struct {
		goos   string
		suffix string
		bit    bool
	}{
		{"windows", ".exe", false},
		{"linux", "", true},
		{"darwin", "", true},
	} {
		if got := exeSuffixFor(tc.goos); got != tc.suffix {
			t.Errorf("exeSuffixFor(%q) = %q, want %q", tc.goos, got, tc.suffix)
		}
		if got := execBitRequiredOn(tc.goos); got != tc.bit {
			t.Errorf("execBitRequiredOn(%q) = %v, want %v", tc.goos, got, tc.bit)
		}
	}
}
