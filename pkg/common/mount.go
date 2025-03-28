package common

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
	"machinerun.io/atomfs/pkg/mount"
	"machinerun.io/atomfs/pkg/verity"
)

func HostMount(fsImgFile string, fsType string, mountpoint string, rootHash string, veritySize int64, verityOffset uint64) error {
	return verity.VerityHostMount(fsImgFile, fsType, mountpoint, rootHash, veritySize, verityOffset)
}

// Mount a filesystem as container root, without host root
// privileges.  We do this using fuse "cmd" which is passed in from actual filesystem backends.
func GuestMount(fsImgFile string, mountpoint string, fuseCmd FuseCmd) error {
	if IsMountpoint(mountpoint) {
		return errors.Errorf("%s is already mounted", mountpoint)
	}

	abs, err := filepath.Abs(fsImgFile)
	if err != nil {
		return errors.Errorf("Failed to get absolute path for %s: %v", fsImgFile, err)
	}
	fsImgFile = abs

	abs, err = filepath.Abs(mountpoint)
	if err != nil {
		return errors.Errorf("Failed to get absolute path for %s: %v", mountpoint, err)
	}
	mountpoint = abs

	cmd, err := fuseCmd(fsImgFile, mountpoint)
	if err != nil {
		return err
	}

	if err := cmd.Process.Release(); err != nil {
		return errors.Errorf("Failed to release process after guestmount %s: %v", fsImgFile, err)
	}
	return nil
}

// unmounts a squash mountpoint and detaches any verity / loop devices that back
// it. Only use this if you are sure the underlying devices aren't in use by
// other mount points.
func Umount(mountpoint string) error {
	devPath, err := GetBackingDevice(mountpoint)

	err = unix.Unmount(mountpoint, 0)
	if err != nil {
		return errors.Wrapf(err, "failed unmounting %v", mountpoint)
	}

	return MaybeCleanupBackingDevice(devPath)
}

func GetBackingDevice(mountpoint string) (string, error) {
	mounts, err := mount.ParseMounts("/proc/self/mountinfo")
	if err != nil {
		return "", err
	}

	theMount, found := mounts.FindMount(mountpoint)
	if !found {
		return "", errors.Errorf("%s is not a mountpoint", mountpoint)
	}
	return theMount.Source, nil
}

// given a device path that had been used as the backing device for a squash
// mountpoint, cleans up and detaches verity device if it still exists.
//
// If the device path does not exist, that is OK - this happens if the device
// was a regular loopback and not -verity.
func MaybeCleanupBackingDevice(devPath string) error {
	if _, err := os.Stat(devPath); err != nil {
		if os.IsNotExist(err) {
			// It's been detached, this is fine.
			return nil
		}
		return errors.WithStack(err)
	}

	// was this a verity mount or a regular loopback mount? (if it's a
	// regular loopback mount, we detached it above, so need to do anything
	// special here; verity doesn't play as nicely)
	if strings.HasSuffix(devPath, verity.VeritySuffix) {
		err := verity.VerityUnmount(devPath)
		if err != nil {
			return errors.Wrapf(err, "failed verity-unmounting %v", devPath)
		}
	}

	return nil
}

func IsMountpoint(dest string) bool {
	mounted, err := mount.IsMountpoint(dest)
	return err == nil && mounted
}

func IsMountedAtDir(_, dest string) (bool, error) {
	dstat, err := os.Stat(dest)
	if os.IsNotExist(err) {
		return false, nil
	}
	if !dstat.IsDir() {
		return false, nil
	}
	mounts, err := mount.ParseMounts("/proc/self/mountinfo")
	if err != nil {
		return false, err
	}

	fdest, err := filepath.Abs(dest)
	if err != nil {
		return false, err
	}
	for _, m := range mounts {
		if m.Target == fdest {
			return true, nil
		}
	}

	return false, nil
}
