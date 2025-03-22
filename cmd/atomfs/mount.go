package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"machinerun.io/atomfs/pkg/common"
	"machinerun.io/atomfs/pkg/molecule"
)

var mountCmd = cli.Command{
	Name:      "mount",
	Usage:     "mount atomfs image",
	ArgsUsage: "ocidir:tag target",
	Action:    doMount,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "persist",
			Usage: "Specify a directory to use for the workdir and upperdir of a writeable overlay (implies --writeable)",
		},
		cli.BoolFlag{
			Name:  "writeable, writable",
			Usage: "Make the mount writeable using an overlay (ephemeral by default)",
		},
		cli.BoolFlag{
			Name:  "allow-missing-verity",
			Usage: "Mount even if the image has no verity data",
		},
		cli.StringFlag{
			Name:  "metadir",
			Usage: "Directory to use for metadata. Use this if /run/atomfs is not writable for some reason.",
		},
	},
}

func mountUsage(_ string) error {
	return errors.New("Usage: atomfs mount [--writeable] [--persist=/tmp/upperdir] ocidir:tag target")
}

func findImage(ctx *cli.Context) (string, string, error) {
	arg := ctx.Args()[0]
	r := strings.SplitN(arg, ":", 2)
	if len(r) != 2 {
		return "", "", mountUsage(ctx.App.Name)
	}
	ocidir := r[0]
	tag := r[1]
	if !common.PathExists(ocidir) {
		return "", "", fmt.Errorf("oci directory %s does not exist: %w", ocidir, mountUsage(ctx.App.Name))
	}
	return ocidir, tag, nil
}

func doMount(ctx *cli.Context) error {

	if len(ctx.Args()) == 0 {
		return mountUsage(ctx.App.Name)
	}

	ocidir, tag, err := findImage(ctx)
	if err != nil {
		return err
	}
	if !amPrivileged() {
		fmt.Println("Please run as root, or in a user namespace")
		fmt.Println(" You could try:")
		fmt.Println("\tlxc-usernsexec -s -- /bin/bash")
		fmt.Println(" or")
		fmt.Println("\tunshare -Umr -- /bin/bash")
		fmt.Println("then run from that shell")
		os.Exit(1)
	}
	target := ctx.Args()[1]
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}

	absOCIDir, err := filepath.Abs(ocidir)
	if err != nil {
		return err
	}

	persistPath := ""
	if ctx.IsSet("persist") {
		persistPath = ctx.String("persist")
		if persistPath == "" {
			return fmt.Errorf("--persist requires an argument")
		}
	}
	opts := molecule.MountOCIOpts{
		OCIDir:                 absOCIDir,
		Tag:                    tag,
		Target:                 absTarget,
		AddWriteableOverlay:    ctx.Bool("writeable") || ctx.IsSet("persist"),
		WriteableOverlayPath:   persistPath,
		AllowMissingVerityData: ctx.Bool("allow-missing-verity"),
		MetadataDir:            ctx.String("metadir"), // nil here means /run/atomfs
	}

	mol, err := molecule.BuildMoleculeFromOCI(opts)
	if err != nil {
		return errors.Wrapf(err, "couldn't build molecule with opts %+v", opts)
	}

	err = mol.Mount(target)
	if err != nil {
		return errors.Wrapf(err, "couldn't mount molecule at mntpt %q ", target)
	}

	return nil
}

func RunCommand(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

func amPrivileged() bool {
	return os.Geteuid() == 0
}

func squashUmount(p string) error {
	if amPrivileged() {
		return common.Umount(p)
	}
	return RunCommand("fusermount", "-u", p)
}
