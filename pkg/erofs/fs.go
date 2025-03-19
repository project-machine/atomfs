package erofs

import (
	"io"
	"os"

	"github.com/pkg/errors"
	"machinerun.io/atomfs/pkg/common"
	"machinerun.io/atomfs/pkg/verity"
)

type erofs struct {
}

func New() *erofs {
	return &erofs{}
}

func (er *erofs) Make(tempdir string, rootfs string, eps *common.ExcludePaths, verity verity.VerityMetadata) (io.ReadCloser, string, string, error) {
	return MakeErofs(tempdir, rootfs, eps, verity)
}

func (er *erofs) ExtractSingle(fsImgFile string, extractDir string) error {
	return ExtractSingleErofs(fsImgFile, extractDir)
}

func (er *erofs) Mount(fsImgFile, mountpoint, rootHash string) error {
	if !common.AmHostRoot() {
		return er.guestMount(fsImgFile, mountpoint)
	}
	err := er.hostMount(fsImgFile, mountpoint, rootHash)
	if err == nil || rootHash != "" {
		return err
	}
	return er.guestMount(fsImgFile, mountpoint)
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

func (er *erofs) hostMount(fsImgFile string, mountpoint string, rootHash string) error {
	veritySize, verityOffset, err := fsImgVerityLocation(fsImgFile)
	if err != nil {
		return err
	}

	return common.HostMount(fsImgFile, "erofs", mountpoint, rootHash, veritySize, verityOffset)
}

func (er *erofs) guestMount(fsImgFile string, mountpoint string) error {
	return common.GuestMount(fsImgFile, mountpoint, erofsFuse)
}

func (er *erofs) Umount(mountpoint string) error {
	return common.Umount(mountpoint)
}
