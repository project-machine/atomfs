package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/urfave/cli"
	"stackerbuild.io/stacker/pkg/mount"
)

var umountCmd = cli.Command{
	Name:   "umount",
	Usage:  "unmount atomfs image",
	Action: doUmount,
}

func umountUsage(me string) error {
	return fmt.Errorf("Usage: %s mountpoint", me)
}

func isMountpoint(p string) bool {
	mounted, err := mount.IsMountpoint(p)
	return err == nil && mounted
}

func doUmount(ctx *cli.Context) error {
	if ctx.NArg() < 1 {
		return umountUsage(ctx.App.Name)
	}

	mountpoint := ctx.Args()[0]

	var err error
	var errs []error

	if !filepath.IsAbs(mountpoint) {
		mountpoint, err = filepath.Abs(mountpoint)
		if err != nil {
			return fmt.Errorf("Failed to find mountpoint: %w", err)
		}
	}

	// We expect the argument to be the mountpoint - either a readonly
	// bind mount, or a writeable overlay.
	err = syscall.Unmount(mountpoint, 0)
	if err != nil {
		errs = append(errs, fmt.Errorf("Failed unmounting %s: %v", mountpoint, err))
	}

	// Now that we've unmounted the mountpoint, we expect the following
	// under there:
	// $mountpoint/meta/ro - the original readonly overlay mountpoint
	// $mountpoint/meta/mounts/* - the original squashfs mounts
	metadir := filepath.Join(mountpoint, "meta")
	p := filepath.Join(metadir, "ro")
	err = syscall.Unmount(p, 0)
	if err != nil {
		errs = append(errs, fmt.Errorf("Failed unmounting RO mountpoint %s: %v", p, err))
	}

	mountsdir := filepath.Join(metadir, "mounts")
	mounts, err := os.ReadDir(mountsdir)
	if err != nil {
		errs = append(errs, fmt.Errorf("Failed reading list of mounts: %v", err))
		return fmt.Errorf("Encountered errors: %#v", errs)
	}

	for _, m := range mounts {
		p = filepath.Join(mountsdir, m.Name())
		if !m.IsDir() || !isMountpoint(p) {
			continue
		}
		err = syscall.Unmount(p, 0)
		if err != nil {
			errs = append(errs, fmt.Errorf("Failed unmounting squashfs dir %s: %v", p, err))
		}
	}

	if len(errs) != 0 {
		for i, e := range errs {
			fmt.Printf("Error %d: %v\n", i, e)
		}
		return fmt.Errorf("Encountered errors %d: %v", len(errs), errs)
	}

	if err := os.RemoveAll(metadir); err != nil {
		return fmt.Errorf("Failed removing %q: %v", metadir, err)
	}

	return nil
}
