package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
	"github.com/urfave/cli"
	satomfs "stackerbuild.io/stacker/atomfs"
)

var mountCmd = cli.Command{
	Name:    "mount",
	Usage:   "mount atomfs image",
	Action:  doMount,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "ro",
			Usage: "Do not make writeable using an overlay",
		},
	},
}

func mountUsage(me string) error {
	return fmt.Errorf("Usage: atomfs mount [--unsafe] ocidir:tag target")
}

func findImage(ctx *cli.Context) (string, string, error) {
	arg := ctx.Args()[0]
	r := strings.SplitN(arg, ":", 2)
	if len(r) != 2 {
		return "", "", mountUsage(ctx.App.Name)
	}
	ocidir := r[0]
	tag := r[1]
	if !PathExists(ocidir) {
		return "", "", fmt.Errorf("oci directory %s does not exist: %w", ocidir, mountUsage(ctx.App.Name))
	}
	return ocidir, tag, nil
}

func doMount(ctx *cli.Context) error {
	if ctx.NArg() != 2 {
		return fmt.Errorf("source and destination required")
	}

	ocidir, tag, err := findImage(ctx)
	if err != nil {
		return err
	}

	target := ctx.Args()[1]
	metadir := filepath.Join(target, "meta")
	if err := EnsureDir(metadir); err != nil {
		return err
	}
	rodest := filepath.Join(metadir, "ro")
	if err = EnsureDir(rodest); err != nil {
		return err
	}

	opts := satomfs.MountOCIOpts{
		OCIDir: ocidir,
		MetadataPath: metadir,
		Tag: tag,
		Target: rodest,
	}

	mol, err := satomfs.BuildMoleculeFromOCI(opts)
	if err != nil {
		return err
	}

	err = mol.Mount(rodest)
	if err != nil {
		return err
	}

	if ctx.Bool("ro") {
		err = bind(target, rodest)
	} else {
		err = overlay(target, rodest, metadir)
	}

	if err != nil {
		satomfs.Umount(rodest)
		return err
	}
	return nil
}

func overlay(target, rodest, metadir string) error {
	workdir := filepath.Join(metadir, "work")
	if err := EnsureDir(workdir); err != nil {
		return err
	}
	upperdir := filepath.Join(metadir, "upper")
	if err := EnsureDir(upperdir); err != nil {
		return err
	}
	overlayArgs := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s,index=off,userxattr", rodest, upperdir, workdir)
	return unix.Mount("overlayfs", target, "overlay", 0, overlayArgs)
}

func bind(target, source string) error {
	return syscall.Mount(source, target, "", syscall.MS_BIND, "")
}
