package fs

import (
	"machinerun.io/atomfs/pkg/erofs"
	"machinerun.io/atomfs/pkg/squashfs"
	types "machinerun.io/atomfs/pkg/types"
)

const (
	SquashfsType types.FilesystemType = "squashfs"
	ErofsType    types.FilesystemType = "erofs"
)

func New(fsType types.FilesystemType) types.Filesystem {
	switch fsType {
	case SquashfsType:
		return squashfs.New()
	case ErofsType:
		return erofs.New()
	}

	return nil
}

func NewFromMediaType(mediaType string) types.Filesystem {
	if squashfs.IsSquashfsMediaType(mediaType) {
		return squashfs.New()
	} else if erofs.IsErofsMediaType(mediaType) {
		return erofs.New()
	}

	return nil
}
