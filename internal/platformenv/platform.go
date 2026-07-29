package platformenv

import (
	"os"
	"strings"
)

// IsWSL reports whether the process runs in Windows Subsystem for Linux.
// Environment variables cover normal launches; the kernel release fallback
// also covers stripped environments and service-style invocations.
func IsWSL() bool {
	if os.Getenv("WSL_INTEROP") != "" || os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	release, err := os.ReadFile("/proc/sys/kernel/osrelease")
	return err == nil && isWSLRelease(string(release))
}

func isWSLRelease(release string) bool {
	release = strings.ToLower(release)
	return strings.Contains(release, "microsoft") || strings.Contains(release, "wsl")
}
