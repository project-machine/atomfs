package fs

import (
	"io"

	"machinerun.io/atomfs/pkg/common"
	"machinerun.io/atomfs/pkg/erofs"
	"machinerun.io/atomfs/pkg/squashfs"
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

const (
	SquashfsType FilesystemType = "squashfs"
	ErofsType    FilesystemType = "erofs"
)

func New(fsType FilesystemType) Filesystem {
	switch fsType {
	case SquashfsType:
		return squashfs.New()
	case ErofsType:
		return erofs.New()
	}

	return nil
}

func NewFromMediaType(mediaType string) Filesystem {
	if squashfs.IsSquashfsMediaType(mediaType) {
		return squashfs.New()
	} else if erofs.IsErofsMediaType(mediaType) {
		return erofs.New()
	}

	return nil
}
