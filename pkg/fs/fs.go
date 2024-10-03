package fs

import (
	"machinerun.io/atomfs/pkg/erofs"
	"machinerun.io/atomfs/pkg/squashfs"
	"machinerun.io/atomfs/pkg/types"
)

// New creates a filesystem instance.
func New(fsType types.FilesystemType) types.Filesystem {
	switch fsType {
	case types.Squashfs:
		return squashfs.New()
	case types.Erofs:
		return erofs.New()
	default:
		return nil
	}
}

// NewFromMediaType creates a filesystem instance based on media-type.
func NewFromMediaType(mediaType string) types.Filesystem {
	if squashfs.IsSquashfsMediaType(mediaType) {
		return New(types.Squashfs)
	} else if erofs.IsErofsMediaType(mediaType) {
		return New(types.Erofs)
	}

	return nil
}
