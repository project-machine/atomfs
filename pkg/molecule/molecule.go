package molecule

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
	"machinerun.io/atomfs/pkg/common"
	"machinerun.io/atomfs/pkg/fs"
	"machinerun.io/atomfs/pkg/log"
	"machinerun.io/atomfs/pkg/mount"
	"machinerun.io/atomfs/pkg/verity"
)

type Molecule struct {
	// Atoms is the list of atoms in this Molecule. The first element in
	// this list is the top most layer in the overlayfs.
	Atoms []ispec.Descriptor

	config MountOCIOpts
}

func (m Molecule) MetadataPath() (string, error) {

	mountNSName, err := common.GetMountNSName()
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(m.config.Target)
	if err != nil {
		return "", err
	}
	metadir := filepath.Join(common.RuntimeDir(m.config.MetadataDir), "meta", mountNSName, common.ReplacePathSeparators(absTarget))
	return metadir, nil
}

func (m Molecule) MountedAtomsPath(parts ...string) (string, error) {
	metapath, err := m.MetadataPath()
	if err != nil {
		return "", err
	}
	mounts := path.Join(metapath, "mounts")
	return path.Join(append([]string{mounts}, parts...)...), nil
}

// mountUnderlyingAtoms mounts all the underlying atoms at
// MountedAtomsPath().
// it returns a cleanup function that will tear down any atoms it successfully mounted.
func (m Molecule) mountUnderlyingAtoms() (error, func()) {
	// in the case that we have a verity or other mount error we need to
	// tear down the other underlying atoms so we don't leave verity and loop
	// devices around unused.
	atomsMounted := []string{}
	cleanupAtoms := func() {
		for _, target := range atomsMounted {
			if umountErr := common.Umount(target); umountErr != nil {
				log.Warnf("cleanup: failed to unmount atom @ target %q: %s", target, umountErr)
			}
		}
	}
	noop := func() {}

	for _, a := range m.Atoms {
		target, err := m.MountedAtomsPath(a.Digest.Encoded())
		if err != nil {
			return errors.Wrapf(err, "failed to find mounted atoms path for %+v", a), cleanupAtoms
		}

		fsi := fs.NewFromMediaType(a.MediaType)

		rootHash := a.Annotations[verity.VerityRootHashAnnotation]

		if !m.config.AllowMissingVerityData {

			if rootHash == "" {
				return errors.Errorf("%v is missing verity data", a.Digest), cleanupAtoms
			}
			if !common.AmHostRoot() {
				return errors.Errorf("won't guestmount an image with verity data without --allow-missing-verity"), cleanupAtoms
			}
		}

		mounts, err := mount.ParseMounts("/proc/self/mountinfo")
		if err != nil {
			return err, cleanupAtoms
		}

		mountpoint, mounted := mounts.FindMount(target)

		if mounted {
			if rootHash != "" {
				err = verity.ConfirmExistingVerityDeviceHash(mountpoint.Source,
					rootHash,
					m.config.AllowMissingVerityData)
				if err != nil {
					return err, cleanupAtoms
				}
				err = verity.ConfirmExistingVerityDeviceCurrentValidity(mountpoint.Source)
				if err != nil {
					return err, cleanupAtoms
				}
			}
			continue
		}

		if err := os.MkdirAll(target, 0755); err != nil {
			return err, cleanupAtoms
		}

		err = fsi.Mount(m.config.AtomsPath(a.Digest.Encoded()), target, rootHash)
		if err != nil {
			return err, cleanupAtoms
		}

		atomsMounted = append(atomsMounted, target)
	}

	return nil, noop
}

// overlayArgs - returns a colon-separated string of dirs to be used as mount
// options to pass to the kernel to actually mount this molecule.
func (m Molecule) overlayLowerDirs() (string, error) {
	dirs := []string{}
	for _, a := range m.Atoms {
		target, err := m.MountedAtomsPath(a.Digest.Encoded())
		if err != nil {
			return "", err
		}
		dirs = append(dirs, target)
	}

	// overlay doesn't work with only one lowerdir and no upperdir.
	// For consistency in that specific case we add a hack here.
	// We create an empty directory called "workaround" in the mounts
	// directory, and add that to lowerdir list.
	if len(dirs) == 1 {
		workaround, err := m.MountedAtomsPath("workaround")
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(workaround, 0755); err != nil {
			return "", errors.Wrapf(err, "couldn't make workaround dir")
		}

		dirs = append(dirs, workaround)
	}

	// Note that in overlayfs, the first thing is the top most layer in the
	// overlay.

	return strings.Join(dirs, ":"), nil
}

// device mapper has no namespacing. if two different binaries invoke this code
// (for example, the stacker test suite), we want to be sure we don't both
// create or delete devices out from the other one when they have detected the
// device exists. so try to cooperate via this lock.
var advisoryLockPath = path.Join(os.TempDir(), ".atomfs-lock")

func makeLock(lockdir string) (*os.File, error) {
	lockfile, err := os.Create(advisoryLockPath)
	if err == nil {
		return lockfile, nil
	}
	// backup plan: lock the destination as ${path}.atomfs-lock
	lockdir = strings.TrimSuffix(lockdir, "/")
	lockPath := filepath.Join(lockdir, ".atomfs-lock")
	var err2 error
	lockfile, err2 = os.Create(lockPath)
	if err2 == nil {
		return lockfile, nil
	}

	err = errors.Errorf("Failed locking %s: %v\nFailed locking %s: %v", advisoryLockPath, err, lockPath, err2)
	return lockfile, err
}

var OverlayMountOptions = "index=off,xino=on,userxattr"

// Mount mounts an overlay at dest, with writeable overlay as per m.config
func (m Molecule) Mount(dest string) error {

	metadir, err := m.MetadataPath()
	if err != nil {
		return errors.Wrapf(err, "can't find metadata path")
	}
	if common.PathExists(metadir) {
		return fmt.Errorf("%q exists: cowardly refusing to mess with it", metadir)
	}

	if err := common.EnsureDir(metadir); err != nil {
		return err
	}

	lockfile, err := makeLock(metadir)
	if err != nil {
		return errors.WithStack(err)
	}
	defer lockfile.Close()

	err = unix.Flock(int(lockfile.Fd()), unix.LOCK_EX)
	if err != nil {
		return errors.WithStack(err)
	}

	overlayLowerDirs, err := m.overlayLowerDirs()
	if err != nil {
		return err
	}

	complete := false

	defer func() {
		if !complete {
			log.Errorf("Failure detected: cleaning up %q", metadir)
			os.RemoveAll(metadir)
		}
	}()

	err, cleanupUnderlyingAtoms := m.mountUnderlyingAtoms()
	if err != nil {
		return err
	}

	defer func() {
		if !complete {
			cleanupUnderlyingAtoms()
		}
	}()

	err = m.config.WriteToFile(filepath.Join(metadir, "config.json"))
	if err != nil {
		return err
	}

	overlayArgs := ""
	if m.config.AddWriteableOverlay {
		rodest := filepath.Join(metadir, "ro")
		if err = common.EnsureDir(rodest); err != nil {
			return err
		}

		persistMetaPath := m.config.WriteableOverlayPath
		if persistMetaPath == "" {
			// no configured path, use metadir
			persistMetaPath = metadir
		}

		workdir := filepath.Join(persistMetaPath, "work")
		if err := common.EnsureDir(workdir); err != nil {
			return errors.Wrapf(err, "failed to ensure workdir %q", workdir)
		}

		upperdir := filepath.Join(persistMetaPath, "persist")
		if err := common.EnsureDir(upperdir); err != nil {
			return errors.Wrapf(err, "failed to ensure upperdir %q", upperdir)
		}

		defer func() {
			if !complete && m.config.WriteableOverlayPath == "" {
				os.RemoveAll(m.config.WriteableOverlayPath)
			}
		}()

		overlayArgs = fmt.Sprintf("lowerdir=%s:%s,upperdir=%s,workdir=%s,%s", dest, overlayLowerDirs, upperdir, workdir, OverlayMountOptions)

	} else {
		// for readonly, just mount the overlay directly onto dest
		overlayArgs = fmt.Sprintf("lowerdir=%s,%s", overlayLowerDirs, OverlayMountOptions)

	}

	// The kernel doesn't allow mount options longer than 4096 chars
	if len(overlayArgs) > 4096 {
		return errors.Errorf("too many lower dirs; must have fewer than 4096 chars")
	}

	err = unix.Mount("overlay", dest, "overlay", 0, overlayArgs)
	if err != nil {
		return errors.Wrapf(err, "couldn't do overlay mount to %s, opts: %s", dest, overlayArgs)
	}

	// ensure deferred cleanups become noops:
	complete = true
	return nil
}

func Umount(dest string) error {
	var err error
	dest, err = filepath.Abs(dest)
	if err != nil {
		return errors.Wrapf(err, "couldn't create abs path for %v", dest)
	}

	lockfile, err := makeLock(dest)
	if err != nil {
		return errors.WithStack(err)
	}
	defer lockfile.Close()

	err = unix.Flock(int(lockfile.Fd()), unix.LOCK_EX)
	if err != nil {
		return errors.WithStack(err)
	}

	mounts, err := mount.ParseMounts("/proc/self/mountinfo")
	if err != nil {
		return err
	}

	underlyingAtoms := []string{}
	for _, m := range mounts {
		if m.FSType != "overlay" {
			continue
		}

		if m.Target != dest { // TODO is this still right
			continue
		}

		underlyingAtoms, err = m.GetOverlayDirs()
		if err != nil {
			return err
		}
		break
	}

	if len(underlyingAtoms) == 0 {
		return errors.Errorf("%s is not an atomfs mountpoint", dest)
	}

	if err := unix.Unmount(dest, 0); err != nil {
		return err
	}

	// now, "refcount" the remaining atoms and see if any of ours are
	// unused
	usedAtoms := map[string]int{}

	mounts, err = mount.ParseMounts("/proc/self/mountinfo")
	if err != nil {
		return err
	}

	for _, m := range mounts {
		if m.FSType != "overlay" {
			continue
		}

		dirs, err := m.GetOverlayDirs()
		if err != nil {
			return err
		}
		for _, d := range dirs {
			usedAtoms[d]++
		}
	}

	// If any of the atoms underlying the target mountpoint are now unused,
	// let's unmount them too.
	for _, a := range underlyingAtoms {
		_, used := usedAtoms[a]
		if used {
			continue
		}
		/* TODO: some kind of logging
		if !used {
			log.Warnf("unused atom %s was part of this molecule?")
			continue
		}
		*/

		// the workaround dir isn't really a mountpoint, so don't unmount it
		if path.Base(a) == "workaround" {
			continue
		}

		err = common.Umount(a)
		if err != nil {
			return err
		}
	}

	return nil
}
