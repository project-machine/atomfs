package common

import (
	"os"
	"strings"
)

func FileChanged(a os.FileInfo, path string) bool {
	b, err := os.Lstat(path)
	if err != nil {
		return true
	}
	return !os.SameFile(a, b)
}

// Takes /proc/self/uid_map contents as one string
// Returns true if this is a uidmap representing the whole host
// uid range.
func uidmapIsHost(oneline string) bool {
	oneline = strings.TrimSuffix(oneline, "\n")
	if len(oneline) == 0 {
		return false
	}
	lines := strings.Split(oneline, "\n")
	if len(lines) != 1 {
		return false
	}
	words := strings.Fields(lines[0])
	if len(words) != 3 || words[0] != "0" || words[1] != "0" || words[2] != "4294967295" {
		return false
	}

	return true
}

func AmHostRoot() bool {
	// if not uid 0, not host root
	if os.Geteuid() != 0 {
		return false
	}
	// if uid_map doesn't map 0 to 0, not host root
	bytes, err := os.ReadFile("/proc/self/uid_map")
	if err != nil {
		return false
	}
	return uidmapIsHost(string(bytes))
}
