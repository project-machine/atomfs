package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/urfave/cli"
	"machinerun.io/atomfs/pkg/common"
)

var umountCmd = cli.Command{
	Name:      "umount",
	Usage:     "unmount atomfs image",
	ArgsUsage: "mountpoint",
	Action:    doUmount,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "metadir",
			Usage: "Directory to use for metadata. Use this if /run/atomfs is not writable for some reason.",
		},
	},
}

func umountUsage(me string) error {
	return fmt.Errorf("Usage: %s umount mountpoint", me)
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

	// We expect the argument to be the mountpoint of the overlay
	err = syscall.Unmount(mountpoint, 0)
	if err != nil {
		errs = append(errs, fmt.Errorf("Failed unmounting %s: %v", mountpoint, err))
	}

	// We expect the following in the metadir
	//
	// $metadir/mounts/* - the original squashfs mounts
	// $metadir/meta/config.json

	// TODO: want to know mountnsname for a target mountpoint... not for our current proc???
	mountNSName, err := common.GetMountNSName()
	if err != nil {
		errs = append(errs, fmt.Errorf("Failed to get mount namespace name"))
	}
	metadir := filepath.Join(common.RuntimeDir(ctx.String("metadir")), "meta", mountNSName, common.ReplacePathSeparators(mountpoint))

	mountsdir := filepath.Join(metadir, "mounts")
	mounts, err := os.ReadDir(mountsdir)
	if err != nil {
		errs = append(errs, fmt.Errorf("Failed reading list of mounts: %v", err))
		return fmt.Errorf("Encountered errors: %v", errs)
	}

	for _, m := range mounts {
		p := filepath.Join(mountsdir, m.Name())
		if !m.IsDir() || !common.IsMountpoint(p) {
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
