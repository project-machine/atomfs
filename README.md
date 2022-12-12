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
# unshare -m -- /bin/bash
root@jerom ~$ cd delme/stacker-squash/
root@jerom ~/delme/stacker-squash$ ~/src/atomfs/atomfs mount oci:smtest-squashfs dest
/home/serge/delme/stacker-squash/dest/meta/mounts/5a27f94ae0691bd617c65cc99544994439acbda359c6375e103f4c099d7ab54c is not yet mounted...
/home/serge/delme/stacker-squash/dest/meta/mounts/369694e2bd95a30c8c742dbde3b21ad91a84ed829b36b41a76985025157dfd52 is not yet mounted...
root@jerom ~/delme/stacker-squash$ ls dest
bin  dev  etc  home  hw  proc  root  sys  tmp  usr  var  xxx
root@jerom ~/delme/stacker-squash$ touch dest/yyy
root@jerom ~/delme/stacker-squash$ ~/src/atomfs/atomfs umount dest
root@jerom ~/delme/stacker-squash$ ls dest/meta/upper/
xxx  yyy
```

# Implementation details

We create $mountpoint/meta and pass that to stacker/atomfs as the
Metadatapath.  We do the readonly stacker/atomfs molecule mount
onto $metadir/ro.  Then if a readonly mount is requested
$metadir/ro is bind mounted onto $metadir.  Otherwise, we create
$metadir/work and $metadir/upper, and use these to do a rw
overlay mount of $metadir/ro onto $mountpoint.
