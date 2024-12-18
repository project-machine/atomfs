load helpers
load 'test_helper/bats-support/load'
load 'test_helper/bats-assert/load'
load 'test_helper/bats-file/load'

function setup_file() {
    export ATOMFS_TEST_RUN_DIR=${BATS_SUITE_TMPDIR}/run/atomfs
    mkdir -p $ATOMFS_TEST_RUN_DIR
}

@test "mounting tampered small images fails immediately" {
    build_image_at $BATS_TEST_TMPDIR

    sha256sum $BATS_TEST_TMPDIR/oci/blobs/sha256/* > initialsums

    # write some bad data onto the squash blobs to make them invalid
    for blob in $BATS_TEST_TMPDIR/oci/blobs/sha256/* ; do
        file $blob | grep "Squashfs filesystem" || continue
        dd if=/dev/random of=$blob conv=notrunc seek=100 count=100
    done

    sha256sum $BATS_TEST_TMPDIR/oci/blobs/sha256/* > finalsums

    # the sums should be different, so assert that diff finds diffs:
    run diff initialsums finalsums
    assert_failure

    mkdir -p mountpoint
    run atomfs-cover --debug mount ${BATS_TEST_TMPDIR}/oci:test-squashfs mountpoint
    assert_failure

}

@test "TODO: check atomfs verify on a mounted image that isn't detected immediately" {
    echo TODO
}
