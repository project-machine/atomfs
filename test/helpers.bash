
check_root(){
    if [ "$(id -u)" != "0" ]; then
        echo "you should be root to run this suite"
        exit 1
    fi
}

ROOT_D=$(dirname $BATS_TEST_FILENAME)/..
TOOLS_D=$ROOT_D/tools
export PATH="$TOOLS_D/bin:$ROOT_D/bin:$PATH"

build_image_at() {
    cd $1
    stackerfilename=${2:-1.stacker.yaml}
    sudo env "PATH=$PATH" stacker --oci-dir $1/oci --stacker-dir=$1/stacker --roots-dir=$1/roots --debug build -f $(dirname $BATS_TEST_FILENAME)/$stackerfilename --layer-type squashfs
    sudo env "PATH=$PATH" stacker --oci-dir $1/oci-no-verity --stacker-dir=$1/stacker --roots-dir=$1/roots  --debug build -f $(dirname $BATS_TEST_FILENAME)/$stackerfilename --layer-type squashfs --no-squashfs-verity
    sudo env "PATH=$PATH" pre-erofs-stacker --oci-dir $1/oci-pre-erofs --stacker-dir=$1/stacker-pre-erofs --roots-dir=$1/roots-pre-erofs --debug build -f $(dirname $BATS_TEST_FILENAME)/$stackerfilename --layer-type squashfs
    sudo chown -R $(id -un):$(id -gn) $1/oci $1/oci-no-verity $1/oci-pre-erofs $1/stacker $1/stacker-pre-erofs $1/roots $1/roots-pre-erofs
}
