package common

import (
	"fmt"
	"os"
	"strings"
)

var TestOverrideRuntimeDirKey = "ATOMFS_TEST_RUN_DIR"

func EnsureDir(dir string) error {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("Failed creating directory %s: %w", dir, err)
	}
	return nil
}

func PathExists(d string) bool {
	_, err := os.Stat(d)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}

// remove dir separators to make one dir name. It is OK that this can't be
// cleanly backed out, we don't need it to
func ReplacePathSeparators(p string) string {
	if p[0] == '/' {
		p = p[1:]
	}
	return strings.ReplaceAll(p, "/", "-")
}

func GetMountNSName() (string, error) {
	val, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil {
		return "", err
	}
	// link target looks like 'mnt:[NUMBER]', we just want NUMBER
	return val[5 : len(val)-1], nil
}

// Allow overriding runtime dir for tests so we can assert empty dirs, etc.
func RuntimeDir(metadir string) string {
	testOverrideDir := os.Getenv(TestOverrideRuntimeDirKey)
	if testOverrideDir == "" {
		if metadir == "" {
			return "/run/atomfs"
		} else {
			return metadir
		}
	}
	return testOverrideDir
}
