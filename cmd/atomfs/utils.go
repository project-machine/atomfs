package main

import (
	"fmt"
	"os"
	"strings"
)

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
