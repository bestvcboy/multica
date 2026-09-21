//go:build windows

package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAgentExecutablePathFallbackFindsUserNpmShim(t *testing.T) {
	root := t.TempDir()
	appData := filepath.Join(root, "Roaming")
	npmDir := filepath.Join(appData, "npm")
	if err := os.MkdirAll(npmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(npmDir, "codex.cmd")
	if err := os.WriteFile(want, []byte("@echo off\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("APPDATA", appData)
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "Local"))
	t.Setenv("PATH", filepath.Join(root, "empty-path"))

	got, err := resolveAgentExecutablePath("codex")
	if err != nil {
		t.Fatalf("resolveAgentExecutablePath did not find the user npm shim: %v", err)
	}
	if !strings.EqualFold(filepath.Clean(got), filepath.Clean(want)) {
		t.Fatalf("fallback path = %q, want %q", got, want)
	}
}
