load helpers
load test_helper/bats-support/load
load test_helper/bats-assert/load
load test_helper/bats-file/load

function setup_file() {
    build_image_at $BATS_SUITE_TMPDIR
    export ATOMFS_TEST_RUN_DIR=${BATS_SUITE_TMPDIR}/run/atomfs
    mkdir -p $ATOMFS_TEST_RUN_DIR
    export MY_MNTNSNAME=$(readlink /proc/self/ns/mnt | cut -c 6-15)
}

function setup() {
    export MP=${BATS_TEST_TMPDIR}/testmountpoint
    mkdir -p $MP
}

@test "guestmount works ignoring verity" {

    lxc-usernsexec -s <<EOF
    set -x
    export ATOMFS_TEST_RUN_DIR=$ATOMFS_TEST_RUN_DIR
    export PERSIST_DIR=${BATS_TEST_TMPDIR}/persist-dir
    mkdir -p \$PERSIST_DIR

    export INNER_MNTNSNAME=\$(readlink /proc/self/ns/mnt | cut -c 6-15)

    set +e
    atomfs --debug mount --persist=\$PERSIST_DIR ${BATS_SUITE_TMPDIR}/oci:test-squashfs $MP
    [ \$? -eq 0 ] && {
      echo guestmount without allow-missing should fail, because we do not have verity
      exit 1
    }
    set -e

    atomfs --debug mount --allow-missing-verity --persist=\$PERSIST_DIR ${BATS_SUITE_TMPDIR}/oci:test-squashfs $MP
    [ -f $MP/1.README.md ]
    [ -f $MP/random.txt ]
    touch $MP/let-me-write

    set +e
    atomfs --debug verify $MP
    [ \$? -eq 0 ] && {
       echo mount with squashfuse ignores verity, so verify should have failed, output should include warning
       exit 1
    }
    set -e

    find $ATOMFS_TEST_RUN_DIR/meta/\$INNER_MNTNSNAME/ -name config.json|xargs cat
    find $ATOMFS_TEST_RUN_DIR/meta/\$INNER_MNTNSNAME/

    atomfs --debug umount $MP
    [ -f \$PERSIST_DIR/persist/let-me-write ]

    # mount point and meta dir should be empty
    [ -d $MP ]
    [ -z \$( ls -A $MP) ]
    [ -d $ATOMFS_TEST_RUN_DIR/meta/\$INNER_MNTNSNAME/ ]
    [ -z \$( ls -A $ATOMFS_TEST_RUN_DIR/meta/\$INNER_MNTNSNAME/) ]
    rm -rf \$PERSIST_DIR
    rm -rf $ATOMFS_TEST_RUN_DIR/meta
EOF

}


@test "guestmount works on images without verity" {

    lxc-usernsexec -s <<EOF
    set -x
    export ATOMFS_TEST_RUN_DIR=$ATOMFS_TEST_RUN_DIR
    export PERSIST_DIR=${BATS_TEST_TMPDIR}/persist-dir
    mkdir -p \$PERSIST_DIR

    export INNER_MNTNSNAME=\$(readlink /proc/self/ns/mnt | cut -c 6-15)

    atomfs --debug mount --allow-missing-verity --persist=\$PERSIST_DIR ${BATS_SUITE_TMPDIR}/oci-no-verity:test-squashfs $MP
    [ -f $MP/1.README.md ]
    [ -f $MP/random.txt ]
    touch $MP/let-me-write

    set +e
    atomfs --debug verify $MP
    [ \$? -eq 0 ] && {
       echo mount with squashfuse ignores verity, so verify should have failed, output should include warning
       exit 1
    }
    set -e

    atomfs --debug umount $MP
    [ -f \$PERSIST_DIR/persist/let-me-write ]

    [ -d $MP ]
    [ -z \$( ls -A $MP) ]
    [ -d $ATOMFS_TEST_RUN_DIR/meta/\$INNER_MNTNSNAME/ ]

    find $ATOMFS_TEST_RUN_DIR/meta/\$INNER_MNTNSNAME/
    [ -z \$( ls -A $ATOMFS_TEST_RUN_DIR/meta/\$INNER_MNTNSNAME/) ]
    rm -rf \$PERSIST_DIR
    rm -rf $ATOMFS_TEST_RUN_DIR/meta
EOF
}

@test "mount with custom metadir and no ATOMFS_TEST_RUN_DIR env var works as guest" {
    unset ATOMFS_TEST_RUN_DIR
    export -n ATOMFS_TEST_RUN_DIR

    lxc-usernsexec -s <<EOF
    set -x
    export META_DIR=${BATS_TEST_TMPDIR}/metadir
    mkdir -p \$META_DIR

    export INNER_MNTNSNAME=\$(readlink /proc/self/ns/mnt | cut -c 6-15)

    atomfs --debug mount --allow-missing-verity --metadir=\$META_DIR ${BATS_SUITE_TMPDIR}/oci-no-verity:test-squashfs $MP
    [ -f $MP/1.README.md ]
    [ -f $MP/random.txt ]

    atomfs --debug umount --metadir=\$META_DIR $MP

    [ -d $MP ]
    [ -z \$( ls -A $MP) ]
    [ -d $META_DIR/meta/\$INNER_MNTNSNAME/ ]

    find $META_DIR/meta/\$INNER_MNTNSNAME/
    [ -z \$( ls -A $ATOMFS_TEST_RUN_DIR/meta/\$INNER_MNTNSNAME/) ]
    rm -rf \$META_DIR

EOF
}
