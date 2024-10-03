package types

import (
	"io"

	"machinerun.io/atomfs/verity"
)

type Filesystem interface {
	// Make a filesystem image.
	Make(tempdir string, rootfs string, eps *ExcludePaths, verity verity.VerityMetadata) (io.ReadCloser, string, string, error)

	// Mount a filesystem as container root, without host root privileges.
	GuestMount(fsFile string, mountpoint string) error

	Mount(fs, mountpoint, rootHash string) error

	HostMount(fs string, mountpoint string, rootHash string) error

	Umount(mountpoint string) error

	VerityDataLocation() uint64

	ExtractSingle(fsFile string, extractDir string) error
}

type FilesystemType string

const (
	Squashfs FilesystemType = "squashfs"
	Erofs    FilesystemType = "erofs"
)
