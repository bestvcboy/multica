//go:build !windows

package daemon

func resolveAgentExecutablePathFallback(_ string) (string, bool) {
	return "", false
}
