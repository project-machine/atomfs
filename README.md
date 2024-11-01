# atomfs [![GoDoc](https://godoc.org/machinerun.io/atomfs?status.svg)](https://godoc.org/machinerun.io/atomfs) [![build](https://github.com/project-machine/atomfs/actions/workflows/build.yaml/badge.svg?branch=main)](https://github.com/project-machine/atomfs/actions/workflows/build.yaml) [![codecov](https://codecov.io/gh/project-machine/atomfs/graph/badge.svg?token=175HCB255L)](https://codecov.io/gh/project-machine/atomfs) [![Apache 2 licensed](https://img.shields.io/badge/license-Apache2-blue.svg)](https://raw.githubusercontent.com/gojini/signal/main/LICENSE)

`atomfs` is a tool that can mount OCI images built in the `squashfs` format as
a read-only `overlayfs` filesystem that can be used by a container runtime.

For OCI-squashfs images that were created with dm-verity data appended, which [stacker](https://stackerbuild.io) does by default, then atomfs will mount each individual squashfs layer using dm-verity before constructing the final overlayfs stack. This ensures the integrity of the contents of the image when mounted, and the use of squashfs removes the window of time between tar extraction and image mounting when an image could be tampered with.

## atomfs library

Please find the atomfs library documentation at [godoc](https://godoc.org/machinerun.io/atomfs).

## atomfs binary

This can be used to mount an OCI+squashfs image.  If you are host
root, then squashfs will be mounted by the kernel.  If you are
container root but not host root, then squashfuse will be used.

Example:

```bash
atomfs mount containers/oci:minbase:latest mnt
atomfs umount mnt
```

Longer example:

```bash
$ lxc-usernsexec -s
$ atomfs mount zothub:busybox-squashfs dest
$ ls dest
bin  dev  etc  home  lib  lib64  root  tmp  usr  var
$ atomfs umount dest
$ mkdir upper
$ atomfs mount --upper=./upper zothub:busybox-squashfs dest
$ ls dest
bin  dev  etc  home  lib  lib64  root  tmp  usr  var
$ touch dest/ab
$ atomfs umount dest
$ ls upper/
ab
```

## Implementation details

The `atomfs` binary uses the `atomfs` package's Molecule API to mount oci
images.

Each squashfs layer is mounted separately at a subdir under
`/run/atomfs/meta/$mountnsid/$mountpoint/`, and then an overlay mount is
constructed for the specified mountpath. If specified in the config, a writeable
upperdir is added to the overlay mount.

Note that if you simply call `umount` on the mountpoint, then
you will be left with all the individual squashfs mounts under
`/run/atomfs/meta/$mountnsid/$mountpoint/`. Use `atomfs umount` instead.

Note that you do need to be root in your namespace in order to
do the final overlay mount.  (We could get around this
by using fuse-overlay, but creating a namespace seems overall
tidy).
