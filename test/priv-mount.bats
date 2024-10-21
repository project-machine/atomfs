load helpers
load 'test_helper/bats-support/load'
load 'test_helper/bats-assert/load'
load 'test_helper/bats-file/load'

function setup_file() {
    check_root
    build_image_at $BATS_SUITE_TMPDIR
    export ATOMFS_TEST_RUN_DIR=${BATS_SUITE_TMPDIR}/run/atomfs
    mkdir -p $ATOMFS_TEST_RUN_DIR
    export MY_MNTNSNAME=$(readlink /proc/self/ns/mnt | cut -c 6-15)
}

function setup() {
    export MP=${BATS_TEST_TMPDIR}/testmountpoint
    mkdir -p $MP
}

@test "RO mount/umount and verify of good image works" {
    run atomfs --debug mount ${BATS_SUITE_TMPDIR}/oci:test-squashfs $MP
    assert_success
    assert_file_exists $MP/1.README.md
    assert_file_exists $MP/random.txt
    assert_dir_exists $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/

    run touch $MP/do-not-let-me
    assert_failure

    run atomfs verify $MP
    assert_success

    run atomfs --debug umount $MP
    assert_success

    # mount point and meta dir should exist but be empty:
    assert_dir_exists $MP
    assert [ -z $( ls -A $MP) ]
    assert_dir_exists $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/
    assert [ -z $( ls -A $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/ ) ]

}

@test "mount with missing verity data fails" {
    run atomfs --debug mount ${BATS_SUITE_TMPDIR}/oci-no-verity:test-squashfs $MP
    assert_failure
    assert_line --partial "is missing verity data"

    # mount point and meta dir should exist but be empty:
    assert_dir_exists $MP
    assert [ -z $( ls -A $MP) ]
    assert_dir_exists $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/
    assert [ -z $( ls -A $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/ ) ]

}

@test "mount with missing verity data passes if you ignore it" {
    run atomfs --debug mount --allow-missing-verity ${BATS_SUITE_TMPDIR}/oci-no-verity:test-squashfs $MP
    assert_success

    run atomfs --debug umount $MP
    assert_success

    # mount point and meta dir should exist but be empty:
    assert_dir_exists $MP
    assert [ -z $( ls -A $MP) ]
    assert_dir_exists $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/
    assert [ -z $( ls -A $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/ ) ]

}

@test "mount/umount with writeable overlay" {
    run atomfs --debug mount --writeable ${BATS_SUITE_TMPDIR}/oci:test-squashfs $MP
    assert_success
    assert_file_exists $MP/1.README.md
    assert_file_exists $MP/random.txt
    assert_dir_exists $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/

    run touch $MP/this-time-let-me
    assert_success

    run cp $MP/1.README.md $MP/3.README.md
    assert_success

    run atomfs --debug umount $MP
    assert_success

    # mount point and meta dir should exist but be empty:
    assert_dir_exists $MP
    assert [ -z $( ls -A $MP) ]
    assert_dir_exists $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/
    assert [ -z $( ls -A $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/ ) ]
}

@test "mount with writeable overlay in separate dir" {
    export PERSIST_DIR=${BATS_TEST_TMPDIR}/upperdir
    mkdir -p $PERSIST_DIR
    run atomfs --debug mount --persist=${PERSIST_DIR} ${BATS_SUITE_TMPDIR}/oci:test-squashfs $MP
    assert_success
    assert_file_exists $MP/1.README.md
    assert_file_exists $MP/random.txt

    run touch $MP/this-time-let-me
    assert_success
    run cp $MP/1.README.md $MP/3.README.md
    assert_success

    assert_file_exists $PERSIST_DIR/this-time-let-me
    assert_file_exists $PERSIST_DIR/3.README.md
    assert_file_not_exists $PERSIST_DIR/1.README.md

    run atomfs --debug umount $MP
    assert_success
    # mount point and meta dir should exist but be empty:
    assert_dir_exists $MP
    assert [ -z $( ls -A $MP) ]
    assert_dir_exists $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/
    assert [ -z $( ls -A $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/) ]

    # but persist dir should still be there:
    assert_file_exists $PERSIST_DIR/this-time-let-me
    assert_file_exists $PERSIST_DIR/3.README.md
}
