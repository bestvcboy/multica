//go:build windows

package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// resolveAgentExecutablePathFallback covers Windows GUI launches whose PATH
// predates a per-user CLI installation. Explorer-launched apps do not always
// receive the same PATH as a fresh terminal, while npm CLIs and the Codex
// Desktop CLI intentionally live in per-user directories.
func resolveAgentExecutablePathFallback(cmd string) (string, bool) {
	if cmd == "" || strings.ContainsAny(cmd, "/\\") {
		return "", false
	}

	candidates := make([]string, 0, 3)
	if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
		npmDir := filepath.Join(appData, "npm")
		candidates = append(candidates,
			filepath.Join(npmDir, cmd+".cmd"),
			filepath.Join(npmDir, cmd+".exe"),
		)
	}
	if strings.EqualFold(cmd, "codex") {
		if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
			candidates = append(candidates,
				filepath.Join(localAppData, "OpenAI", "Codex", "bin", "codex.exe"),
			)
		}
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved, true
		}
	}
	return "", false
}
