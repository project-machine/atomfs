load helpers
load 'test_helper/bats-support/load'
load 'test_helper/bats-assert/load'
load 'test_helper/bats-file/load'

function setup_file() {
    check_root
    build_image_at $BATS_SUITE_TMPDIR mount-umount-trees.stacker.yaml
    export ATOMFS_TEST_RUN_DIR=${BATS_SUITE_TMPDIR}/run/atomfs
    mkdir -p $ATOMFS_TEST_RUN_DIR
    export MY_MNTNSNAME=$(readlink /proc/self/ns/mnt | cut -c 6-15)
}

function setup() {
    export MP=${BATS_TEST_TMPDIR}/testmountpoint
    mkdir -p $MP
}

function verity_checkusedloops() {
    # search for loopdevices which have backing files with the current
    # BATS_TEST_DIRNAME value and complain if they're present.
    local usedloops="" found="" x=""
    for ((x=0; x<5; x++)); do
        usedloops=$(losetup -a | grep $BATS_TEST_DIRNAME || echo)
        if [ -n "$usedloops" ]; then
            found=1
            udevadm settle
        else
            return 0
        fi
    done
    echo "found used loops in testdir=$BATS_TEST_DIRNAME :$usedloops" >&3
    [ $found = 1 ]
}

@test "mount + umount + mount a tree of images works" {

    echo MOUNT A
    mkdir -p $MP/a
    run atomfs --debug mount ${BATS_SUITE_TMPDIR}/oci:a-squashfs $MP/a
    assert_success
    assert_file_exists $MP/a/a

    echo MOUNT B
    mkdir -p $MP/b
    run atomfs --debug mount ${BATS_SUITE_TMPDIR}/oci:b-squashfs $MP/b
    assert_success
    assert_file_exists $MP/b/b

    echo UMOUNT B
    atomfs --debug umount $MP/b
    assert_success

    # first layer should still exist since a is still mounted
    manifest=$(cat ${BATS_SUITE_TMPDIR}/oci/index.json | jq -r .manifests[0].digest | cut -f2 -d:)
    first_layer_hash=$(cat ${BATS_SUITE_TMPDIR}/oci/blobs/sha256/$manifest | jq -r .layers[0].digest | cut -f2 -d:)
    assert_block_exists "/dev/mapper/$first_layer_hash-verity"

    echo MOUNT C
    mkdir -p $MP/c
    atomfs --debug mount ${BATS_SUITE_TMPDIR}/oci:c-squashfs $MP/c
    assert_success
    assert_file_exists $MP/c/c

    echo UMOUNT A
    atomfs --debug umount $MP/a
    assert_success

    # first layer should still exist since c is still mounted
    manifest=$(cat ${BATS_SUITE_TMPDIR}/oci/index.json | jq -r .manifests[0].digest | cut -f2 -d:)
    first_layer_hash=$(cat ${BATS_SUITE_TMPDIR}/oci/blobs/sha256/$manifest | jq -r .layers[0].digest | cut -f2 -d:)
    assert_block_exists "/dev/mapper/$first_layer_hash-verity"

    # c should still be ok
    assert_file_exists $MP/c/c
    assert_file_exists $MP/c/bin/sh

    atomfs --debug umount $MP/c
    assert_success

    # c's last layer shouldn't exist any more, since it is unique
    manifest=$(cat ${BATS_SUITE_TMPDIR}/oci/index.json | jq -r .manifests[0].digest | cut -f2 -d:)
    last_layer_num=$(($(cat ${BATS_SUITE_TMPDIR}/oci/blobs/sha256/$manifest | jq -r '.layers | length')-1))
    last_layer_hash=$(cat ${BATS_SUITE_TMPDIR}/oci/blobs/sha256/$manifest | jq -r .layers[$last_layer_num].digest | cut -f2 -d:)
    echo "last layer hash is $last_layer_hash"
    assert_block_not_exists /dev/mapper/$last_layer_hash-verity
    verity_checkusedloops

    # mount points and meta dir should exist but be empty:
    for subdir in a b c; do
        assert_dir_exists $MP/$subdir
        assert [ -z $( ls -A $MP/$subdir) ]
    done
    assert_dir_exists $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/
    assert [ -z $( ls -A $ATOMFS_TEST_RUN_DIR/meta/$MY_MNTNSNAME/) ]

}
