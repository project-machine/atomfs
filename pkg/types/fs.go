package fs

import (
	"io"

	"machinerun.io/atomfs/pkg/common"
	"machinerun.io/atomfs/pkg/verity"
)

type Filesystem interface {
	// Make creates a new filesystem image.
	Make(tempdir string, rootfs string, eps *common.ExcludePaths, verity verity.VerityMetadata) (io.ReadCloser, string, string, error)
	// ExtractSingle extracts a filesystem image.
	ExtractSingle(fsImgFile string, extractDir string) error
	// Mount mounts a filesystem image on a given mountpoint.
	Mount(fsImgFile, mountpoint, rootHash string) error
	// Unmount umounts a filesystem image.
	Umount(mountpoint string) error
}

type FilesystemType string

type FsExtractor interface {
	Name() string
	IsAvailable() error
	// Mount - Mount or extract path to dest.
	//   Return nil on "already extracted"
	//   Return error on failure.
	Mount(path, dest string) error
}
