package common

import "os/exec"

type FuseCmd func(fsImgFile, extractDir string) (*exec.Cmd, error)
