package erofs

import (
	"io"

	"machinerun.io/atomfs/verity"
)

type erofs struct {
}

func New() *erofs {
	return &erofs{}
}

func (fs *erofs) Make(tempdir string, rootfs string, eps *ExcludePaths, verity verity.VerityMetadata) (io.ReadCloser, string, string, error) {
}

// Mount a filesystem as container root, without host root privileges.
func (fs *erofs) GuestMount(fsFile string, mountpoint string) error {
	return nil
}

func (fs *erofs) Mount(fsFile, mountpoint, rootHash string) error {
	return nil
}

func (fs *erofs) HostMount(fsFile string, mountpoint string, rootHash string) error {
	return nil
}

func (fs *erofs) Umount(mountpoint string) error {
	return nil
}

func (fs *erofs) VerityDataLocation() uint64 {
	return 0
}
