package common

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
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

func IsEmptyDir(path string) (bool, error) {
	fh, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer fh.Close()

	_, err = fh.ReadDir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

// Which - like the unix utility, return empty string for not-found.
// this might fit well in lib/, but currently lib's test imports
// squashfs creating a import loop.
func Which(name string) string {
	return whichSearch(name, strings.Split(os.Getenv("PATH"), ":"))
}

func whichSearch(name string, paths []string) string {
	var search []string

	if strings.ContainsRune(name, os.PathSeparator) {
		if filepath.IsAbs(name) {
			search = []string{name}
		} else {
			search = []string{"./" + name}
		}
	} else {
		search = []string{}
		for _, p := range paths {
			search = append(search, filepath.Join(p, name))
		}
	}

	for _, fPath := range search {
		if err := unix.Access(fPath, unix.X_OK); err == nil {
			return fPath
		}
	}

	return ""
}

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
