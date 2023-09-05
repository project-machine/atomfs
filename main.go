package main

import (
	"fmt"
	"os"

	"github.com/apex/log"
	"github.com/urfave/cli"
)

var Version string

func main() {
	app := cli.NewApp()
	app.Name = "atomfs"
	app.Usage = "mount and unmount atomfs filesystems"
	app.Version = Version
	app.Commands = []cli.Command{
		mountCmd,
		umountCmd,
	}

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "display additional debug information on stderr",
		},
	}

	app.Before = func(c *cli.Context) error {
		if c.Bool("debug") {
			log.SetLevel(log.DebugLevel)
		}
		return nil
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		os.Exit(1)
	}
}
