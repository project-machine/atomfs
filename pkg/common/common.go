package common

import "os"

func FileChanged(a os.FileInfo, path string) bool {
	b, err := os.Lstat(path)
	if err != nil {
		return true
	}
	return !os.SameFile(a, b)
}
