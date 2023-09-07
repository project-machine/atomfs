# atomfs binary

This can be used to mount an OCI+squashfs image.  If you are host
root, then squashfs will be mounted by the kernel.  If you are
container root but not host root, then squashfuse will be used.

Example:
```
atomfs mount containers/oci:minbase:latest mnt
atomfs umount mnt
```

Longer example:
```
serge@jerom ~$ lxc-usernsexec -s
root@jerom ~$ atomfs mount zothub:busybox-squashfs dest
root@jerom ~$ ls dest
bin  dev  etc  home  lib  lib64  root  tmp  usr  var
root@jerom ~$ atomfs umount dest
```

# Implementation details

We create $mountpoint/meta and pass that to stacker/atomfs as the
Metadatapath.  We do the readonly stacker/atomfs molecule mount
onto $metadir/ro.  Then if a readonly mount is requested
$metadir/ro is bind mounted onto $metadir.  Otherwise, we create
$metadir/work and $metadir/upper, and use these to do a rw
overlay mount of $metadir/ro onto $mountpoint.

Note that if you simply call `umount` on the mountpoint, then
you will be left with all the individual squashfs mounts under
`dest/mounts/*/`.

Note that you do need to be root in your namespace in order to
do the final bind or overlay mount.  (We could get around this
by using fuse-overlay, but creating a namespace seems overall
tidy).
