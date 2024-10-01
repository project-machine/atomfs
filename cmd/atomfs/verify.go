package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/urfave/cli"
	"machinerun.io/atomfs/mount"
	"machinerun.io/atomfs/squashfs"
)

var verifyCmd = cli.Command{
	Name:      "verify",
	Usage:     "check atomfs image for dm-verity errors",
	ArgsUsage: "atomfs mountpoint",
	Action:    doVerify,
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

	if !isMountpoint(mountpoint) {
		return fmt.Errorf("%s is not a mountpoint", mountpoint)
	}

	// hidden by the final overlay mount, but visible in the mountinfo:
	// $mountpoint/meta/mounts/* - the original squashfs mounts
	mountsdir := filepath.Join(mountpoint, "meta", "mounts")

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
	for _, m := range mounts {

		if m.FSType != "squashfs" {
			continue
		}

		if !strings.HasPrefix(m.Target, mountsdir) {
			continue
		}

		err = squashfs.ConfirmExistingVerityDeviceCurrentValidity(m.Source)
		if err != nil {
			fmt.Printf("%s: CORRUPTION FOUND\n", m.Source)
			allOK = false
		} else {
			fmt.Printf("%s: OK\n", m.Source)
		}
	}

	if allOK {
		return nil
	}
	return fmt.Errorf("Found corrupt devices in molecule")
}
