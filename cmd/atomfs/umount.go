package main

import (
	"fmt"
	"path/filepath"

	"github.com/urfave/cli"
	"machinerun.io/atomfs"
	"machinerun.io/atomfs/mount"
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

	if !filepath.IsAbs(mountpoint) {
		mountpoint, err = filepath.Abs(mountpoint)
		if err != nil {
			return fmt.Errorf("Failed to find mountpoint: %w", err)
		}
	}

	return atomfs.UmountWithMetadir(mountpoint, ctx.String("metadir"))
}
