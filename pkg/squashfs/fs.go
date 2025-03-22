package squashfs

import (
	"io"
	"os"

	"github.com/pkg/errors"
	"machinerun.io/atomfs/pkg/common"
	"machinerun.io/atomfs/pkg/verity"
)

type squashfs struct {
}

func New() *squashfs {
	return &squashfs{}
}

func (sq *squashfs) Make(tempdir string, rootfs string, eps *common.ExcludePaths, verity verity.VerityMetadata) (io.ReadCloser, string, string, error) {
	return MakeSquashfs(tempdir, rootfs, eps, verity)
}

func (sq *squashfs) ExtractSingle(fsImgFile string, extractDir string) error {
	return ExtractSingleSquash(fsImgFile, extractDir)
}

func (sq *squashfs) Mount(fsImgFile, mountpoint, rootHash string) error {
	if !common.AmHostRoot() {
		return sq.guestMount(fsImgFile, mountpoint)
	}
	err := sq.hostMount(fsImgFile, mountpoint, rootHash)
	if err == nil || rootHash != "" {
		return err
	}
	return sq.guestMount(fsImgFile, mountpoint)
}

func fsImgVerityLocation(fsImgFile string) (int64, uint64, error) {
	fi, err := os.Stat(fsImgFile)
	if err != nil {
		return -1, 0, errors.WithStack(err)
	}

	sblock, err := readSuperblock(fsImgFile)
	if err != nil {
		return -1, 0, err
	}

	verityOffset, err := verityDataLocation(sblock)
	if err != nil {
		return -1, 0, err
	}

	return fi.Size(), verityOffset, nil
}

func (sq *squashfs) hostMount(fsImgFile string, mountpoint string, rootHash string) error {
	veritySize, verityOffset, err := fsImgVerityLocation(fsImgFile)
	if err != nil {
		return err
	}

	return common.HostMount(fsImgFile, "squashfs", mountpoint, rootHash, veritySize, verityOffset)
}

func (sq *squashfs) guestMount(fsImgFile string, mountpoint string) error {
	return common.GuestMount(fsImgFile, mountpoint, squashFuse)
}

func (sq *squashfs) Umount(mountpoint string) error {
	return common.Umount(mountpoint)
}
