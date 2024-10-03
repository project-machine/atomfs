package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/urfave/cli"
	"golang.org/x/sys/unix"
	"machinerun.io/atomfs"
	"machinerun.io/atomfs/pkg/squashfs"
)

var mountCmd = cli.Command{
	Name:   "mount",
	Usage:  "mount atomfs image",
	Action: doMount,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "persist, upper, upperdir",
			Usage: "Specify a directory to use as writeable overlay (implies --writeable)",
		},
		cli.BoolFlag{
			Name:  "writeable, writable",
			Usage: "Make the mount writeable using an overlay (ephemeral by default)",
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
	if !PathExists(ocidir) {
		return "", "", fmt.Errorf("oci directory %s does not exist: %w", ocidir, mountUsage(ctx.App.Name))
	}
	return ocidir, tag, nil
}

func doMount(ctx *cli.Context) error {
	if !amPrivileged() {
		fmt.Println("Please run as root, or in a user namespace")
		fmt.Println(" You could try:")
		fmt.Println("\tlxc-usernsexec -s -- /bin/bash")
		fmt.Println(" or")
		fmt.Println("\tunshare -Umr -- /bin/bash")
		fmt.Println("then run from that shell")
		os.Exit(1)
	}
	ocidir, tag, err := findImage(ctx)
	if err != nil {
		return err
	}

	target := ctx.Args()[1]
	metadir := filepath.Join(target, "meta")

	complete := false

	defer func() {
		if !complete {
			cleanupDest(metadir)
		}
	}()

	if PathExists(metadir) {
		return fmt.Errorf("%q exists: cowardly refusing to mess with it", metadir)
	}

	if err := EnsureDir(metadir); err != nil {
		return err
	}

	rodest := filepath.Join(metadir, "ro")
	if err = EnsureDir(rodest); err != nil {
		return err
	}

	opts := atomfs.MountOCIOpts{
		OCIDir:       ocidir,
		MetadataPath: metadir,
		Tag:          tag,
		Target:       rodest,
	}

	mol, err := atomfs.BuildMoleculeFromOCI(opts)
	if err != nil {
		return err
	}

	err = mol.Mount(rodest)
	if err != nil {
		return err
	}

	if ctx.Bool("writeable") || ctx.IsSet("persist") {
		err = overlay(target, rodest, metadir, ctx)
	} else {
		err = bind(target, rodest)
	}

	complete = err == nil
	return err
}

func cleanupDest(metadir string) {
	fmt.Printf("Failure detected: cleaning up %q", metadir)
	rodest := filepath.Join(metadir, "ro")
	if PathExists(rodest) {
		if err := unix.Unmount(rodest, 0); err != nil {
			fmt.Printf("Failed unmounting %q: %v", rodest, err)
		}
	}

	mountsdir := filepath.Join(metadir, "mounts")
	entries, err := os.ReadDir(mountsdir)
	if err != nil {
		fmt.Printf("Failed reading contents of %q: %v", mountsdir, err)
		os.RemoveAll(metadir)
		return
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Failed getting working directory")
		os.RemoveAll(metadir)
	}
	for _, e := range entries {
		n := filepath.Base(e.Name())
		if n == "workaround" {
			continue
		}
		if strings.HasSuffix(n, ".log") {
			continue
		}
		p := filepath.Join(wd, mountsdir, e.Name())
		if err := squashUmount(p); err != nil {
			fmt.Printf("Failed unmounting %q: %v\n", p, err)
		}
	}
	os.RemoveAll(metadir)
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
		return squashfs.Umount(p)
	}
	return RunCommand("fusermount", "-u", p)
}

func overlay(target, rodest, metadir string, ctx *cli.Context) error {
	workdir := filepath.Join(metadir, "work")
	if err := EnsureDir(workdir); err != nil {
		return err
	}
	upperdir := filepath.Join(metadir, "persist")
	if ctx.IsSet("persist") {
		upperdir = ctx.String("persist")
	}
	if err := EnsureDir(upperdir); err != nil {
		return err
	}
	overlayArgs := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s,index=off,userxattr", rodest, upperdir, workdir)
	return unix.Mount("overlayfs", target, "overlay", 0, overlayArgs)
}

func bind(target, source string) error {
	return syscall.Mount(source, target, "", syscall.MS_BIND, "")
}
