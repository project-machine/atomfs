package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/urfave/cli"
	"machinerun.io/atomfs/pkg/common"
	"machinerun.io/atomfs/pkg/log"
	"machinerun.io/atomfs/pkg/mount"
	"machinerun.io/atomfs/pkg/verity"
)

var verifyCmd = cli.Command{
	Name:      "verify",
	Usage:     "check atomfs image for dm-verity errors",
	ArgsUsage: "atomfs mountpoint",
	Action:    doVerify,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "metadir",
			Usage: "Directory to use for metadata. Use this if /run/atomfs is not writable for some reason.",
		},
	},
}

func verifyUsage(me string) error {
	return fmt.Errorf("Usage: %s verify mountpoint", me)
}

func doVerify(ctx *cli.Context) error {
	if ctx.NArg() != 1 {
		return verifyUsage(ctx.App.Name)
	}

	mountpoint := ctx.Args()[0]

	var err error

	if !filepath.IsAbs(mountpoint) {
		mountpoint, err = filepath.Abs(mountpoint)
		if err != nil {
			return fmt.Errorf("Failed to find mountpoint: %w", err)
		}
	}

	if !common.IsMountpoint(mountpoint) {
		return fmt.Errorf("%s is not a mountpoint", mountpoint)
	}

	mountNSName, err := common.GetMountNSName()
	if err != nil {
		return err
	}

	metadir := filepath.Join(common.RuntimeDir(ctx.String("metadir")), "meta", mountNSName, common.ReplacePathSeparators(mountpoint))
	mountsdir := filepath.Join(metadir, "mounts")

	mounts, err := mount.ParseMounts("/proc/self/mountinfo")
	if err != nil {
		return err
	}

	// quick check that the top level mount is an overlayfs as expected:
	for _, m := range mounts {
		if m.Target == mountpoint && m.FSType != "overlay" {
			return fmt.Errorf("%s is not an overlayfs, are you sure it is a mounted molecule? %+v", mountpoint, m)
		}
	}

	allOK := true
	checkedCount := 0
	for _, m := range mounts {
		if !strings.HasPrefix(m.Target, mountsdir) {
			continue
		}
		if m.FSType == "fuse.squashfuse_ll" {
			log.Warnf("found squashfuse mount not supported by verify at %q", m.Source)
			continue
		}
		if m.FSType != "squashfs" {
			continue
		}
		checkedCount = checkedCount + 1
		err = verity.ConfirmExistingVerityDeviceCurrentValidity(m.Source)
		if err != nil {
			fmt.Printf("%s: CORRUPTION FOUND\n", m.Source)
			allOK = false
		} else {
			fmt.Printf("%s: OK\n", m.Source)
		}
	}

	// TODO - want to also be able to compare to expected # of mounts from
	// molecule, need to write more molecule info during mol.mount
	if checkedCount == 0 {
		return fmt.Errorf("no applicable mounts found in %q", mountsdir)
	}

	if allOK {
		return nil
	}
	return fmt.Errorf("Found corrupt devices in molecule")
}
