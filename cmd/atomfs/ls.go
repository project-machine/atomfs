package main

import (
	"fmt"
	"path/filepath"

	"github.com/opencontainers/umoci"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	stackeroci "machinerun.io/atomfs/pkg/oci"
)

var lsCmd = cli.Command{
	Name:      "ls",
	Usage:     "list tags in an OCI layout",
	ArgsUsage: "path/to/oci/layout",
	Action:    doLS,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "sha256",
			Usage: "include each tag's manifest SHA-256 digest",
		},
	},
}

func lsUsage(me string) error {
	return fmt.Errorf("Usage: %s ls path/to/oci/layout", me)
}

func doLS(ctx *cli.Context) error {
	if ctx.NArg() != 1 {
		return lsUsage(ctx.App.Name)
	}

	layoutPath, err := filepath.Abs(ctx.Args()[0])
	if err != nil {
		return errors.Wrap(err, "failed resolving OCI layout path")
	}

	oci, err := umoci.OpenLayout(layoutPath)
	if err != nil {
		return errors.Wrapf(err, "failed opening OCI layout %q", layoutPath)
	}
	defer oci.Close()

	refs, err := stackeroci.ListTagManifestSHA256(oci)
	if err != nil {
		return err
	}

	for _, ref := range refs {
		if ctx.Bool("sha256") {
			fmt.Fprintf(ctx.App.Writer, "%s@sha256:%s\n", ref.Tag, ref.SHA256)
		} else {
			fmt.Fprintln(ctx.App.Writer, ref.Tag)
		}
	}
	return nil
}
