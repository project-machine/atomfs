# atomfs binary

This can be used to mount an OCI+squashfs image.  If you are host
root, then squashfs will be mounted by the kernel.  If you are
container root but not host root, then squashfuse will be used.

Example:
```
atomfs mount containers/oci:minbase:latest mnt
atomfs umount mnt
```

# Implementation details

We create $mountpoint/meta and pass that to stacker/atomfs as the
Metadatapath.  We do the readonly stacker/atomfs molecule mount
onto $metadir/ro.  Then if a readonly mount is requested
$metadir/ro is bind mounted onto $metadir.  Otherwise, we create
$metadir/work and $metadir/upper, and use these to do a rw
overlay mount of $metadir/ro onto $mountpoint.
