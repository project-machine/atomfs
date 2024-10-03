package squashfs

import (
	"io"

	"machinerun.io/atomfs/verity"
)

type squashfs struct {
}

func New() *squashfs {
	return &squashfs{}
}

func (fs *squashfs) Make(tempdir string, rootfs string, eps *ExcludePaths, verity verity.VerityMetadata) (io.ReadCloser, string, string, error) {
}

// Mount a filesystem as container root, without host root privileges.
func (fs *squashfs) GuestMount(fsFile string, mountpoint string) error {
	return nil
}

func (fs *squashfs) Mount(fsFile, mountpoint, rootHash string) error {
	return nil
}

func (fs *squashfs) HostMount(fsFile string, mountpoint string, rootHash string) error {
	return nil
}

func (fs *squashfs) Umount(mountpoint string) error {
	return nil
}

func (fs *squashfs) VerityDataLocation() uint64 {
	return 0
}

func (fs *squashfs) ExtractSingle(fsFile string, extractDir string) error {
}
