package verity

// #cgo pkg-config: libcryptsetup devmapper --static
// #include <libcryptsetup.h>
// #include <stdlib.h>
// #include <errno.h>
// #include <libdevmapper.h>
/*
int get_verity_params(char *device, char **params, int task_type)
{
	struct dm_task *dmt;
	struct dm_info dmi;
	int r;
	uint64_t start, length;
	char *type, *tmpParams;

	dmt = dm_task_create(task_type);
	if (!dmt)
		return 1;

	r = 2;
	if (!dm_task_secure_data(dmt))
		goto out;

	r = 3;
	if (!dm_task_set_name(dmt, device))
		goto out;

	r = 4;
	if (!dm_task_run(dmt))
		goto out;

	r = 5;
	if (!dm_task_get_info(dmt, &dmi))
		goto out;

	r = 6;
	if (!dmi.exists)
		goto out;

	r = 7;
	if (dmi.target_count <= 0)
		goto out;

	r = 8;
	dm_get_next_target(dmt, NULL, &start, &length, &type, &tmpParams);
	if (!type)
		goto out;

	r = 9;
	if (strcasecmp(type, CRYPT_VERITY)) {
		fprintf(stderr, "type: %s (%s) %d\n", type, CRYPT_VERITY, strcmp(type, CRYPT_VERITY));
		goto out;
	}
	*params = strdup(tmpParams);

	r = 0;
out:
	dm_task_destroy(dmt);
	return r;
}
int get_verity_table_params(char *device, char **params)
{
   return get_verity_params(device, params, DM_DEVICE_TABLE);
}

int get_verity_status_params(char *device, char **params)
{
   return get_verity_params(device, params, DM_DEVICE_STATUS);
}

*/
import "C"

import (
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/freddierice/go-losetup"
	"github.com/martinjungblut/go-cryptsetup"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

const VerityRootHashAnnotation = "io.stackeroci.stacker.atomfs_verity_root_hash"

type verityDeviceType struct {
	Flags      uint
	DataDevice string
	HashOffset uint64
}

func (verity verityDeviceType) Name() string {
	return C.CRYPT_VERITY
}

func (verity verityDeviceType) Unmanaged() (unsafe.Pointer, func()) {
	var cParams C.struct_crypt_params_verity

	cParams.hash_name = C.CString("sha256")
	cParams.data_device = C.CString(verity.DataDevice)
	cParams.fec_device = nil
	cParams.fec_roots = 0

	cParams.salt_size = 32 // DEFAULT_VERITY_SALT_SIZE for x86
	cParams.salt = nil

	// these can't be larger than a page size, but we want them to be as
	// big as possible so the hash data is small, so let's set them to a
	// page size.
	cParams.data_block_size = C.uint(os.Getpagesize())
	cParams.hash_block_size = C.uint(os.Getpagesize())

	cParams.data_size = C.ulong(verity.HashOffset / uint64(os.Getpagesize()))
	cParams.hash_area_offset = C.ulong(verity.HashOffset)
	cParams.fec_area_offset = 0
	cParams.hash_type = 1 // use format version 1 (i.e. "modern", non chrome-os)
	cParams.flags = C.uint(verity.Flags)

	deallocate := func() {
		C.free(unsafe.Pointer(cParams.hash_name))
		C.free(unsafe.Pointer(cParams.data_device))
	}

	return unsafe.Pointer(&cParams), deallocate
}

func isCryptsetupEINVAL(err error) bool {
	cse, ok := err.(*cryptsetup.Error)
	return ok && cse.Code() == -22
}

var CryptsetupTooOld = errors.Errorf("libcryptsetup not new enough, need >= 2.3.0")

func AppendVerityData(file string) (string, error) {
	fi, err := os.Lstat(file)
	if err != nil {
		return "", errors.WithStack(err)
	}

	verityOffset := fi.Size()

	// we expect make fs to have padded the file to the nearest 4k
	// (dm-verity requires device block size, which is 512 for loopback,
	// which is a multiple of 4k), let's check that here
	if verityOffset%512 != 0 {
		return "", errors.Errorf("bad verity file size %d", verityOffset)
	}

	verityDevice, err := cryptsetup.Init(file)
	if err != nil {
		return "", errors.WithStack(err)
	}

	verityType := verityDeviceType{
		Flags:      cryptsetup.CRYPT_VERITY_CREATE_HASH,
		DataDevice: file,
		HashOffset: uint64(verityOffset),
	}
	err = verityDevice.Format(verityType, cryptsetup.GenericParams{})
	if err != nil {
		return "", errors.WithStack(err)
	}

	// a bit ugly, but this is the only API for querying the root
	// hash (short of invoking the veritysetup binary), and it was
	// added in libcryptsetup commit 188cb114af94 ("Add support for
	// verity in crypt_volume_key_get and use it in status"), which
	// is relatively recent (ubuntu 20.04 does not have this patch,
	// for example).
	//
	// before that, we get a -22. so, let's test for that and
	// render a special error message.
	rootHash, _, err := verityDevice.VolumeKeyGet(cryptsetup.CRYPT_ANY_SLOT, "")
	if isCryptsetupEINVAL(err) {
		return "", CryptsetupTooOld
	} else if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", rootHash), errors.WithStack(err)
}

func verityName(p string) string {
	return fmt.Sprintf("%s-%s", p, VeritySuffix)
}

func VerityHostMount(fsImgFile string, fsType string, mountpoint string, rootHash string, veritySize int64, verityOffset uint64) error {
	if verityOffset == uint64(veritySize) && rootHash != "" {
		return errors.Errorf("asked for verity but no data present")
	}

	if rootHash == "" && verityOffset != uint64(veritySize) {
		return errors.Errorf("verity data present but no root hash specified")
	}

	mountSourcePath := ""

	var verityDevice *cryptsetup.Device
	name := verityName(path.Base(fsImgFile))

	loopDevNeedsClosedOnErr := false
	var loopDev losetup.Device
	var err error

	// set up the verity device if necessary
	if rootHash != "" {
		verityDevPath := path.Join("/dev/mapper", name)
		mountSourcePath = verityDevPath
		_, err = os.Stat(verityDevPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return errors.WithStack(err)
			}

			loopDev, err = losetup.Attach(fsImgFile, 0, true)
			if err != nil {
				return errors.WithStack(err)
			}
			loopDevNeedsClosedOnErr = true

			verityDevice, err = cryptsetup.Init(loopDev.Path())
			if err != nil {
				return errors.WithStack(err)
			}

			verityType := verityDeviceType{
				Flags:      0,
				DataDevice: loopDev.Path(),
				HashOffset: verityOffset,
			}

			err = verityDevice.Load(verityType)
			if err != nil {
				_ = loopDev.Detach()
				return errors.WithStack(err)
			}

			// each string byte hex encodes four bits of info...
			volumeKeySizeInBytes := len(rootHash) * 4 / 8
			rootHashBytes, err := hex.DecodeString(rootHash)
			if err != nil {
				_ = loopDev.Detach()
				return errors.WithStack(err)
			}

			if len(rootHashBytes) != volumeKeySizeInBytes {
				_ = loopDev.Detach()
				return errors.Errorf("unexpected key size for %s", rootHash)
			}

			err = verityDevice.ActivateByVolumeKey(name, string(rootHashBytes), volumeKeySizeInBytes, cryptsetup.CRYPT_ACTIVATE_READONLY)
			if err != nil {
				_ = loopDev.Detach()
				return errors.WithStack(err)
			}
		} else {
			err = ConfirmExistingVerityDeviceHash(verityDevPath, rootHash, rejectVerityFailure)
			if err != nil {
				return err
			}
		}

		// we have to check `dmsetup status $device` here because
		// ActivateByVolumeKey will return some errors but will only WARN to
		// stderr if corruption was found just after activation. It will
		// reliably return success for a corrupted image, and we need to check
		// the same thing it checks for its warning.
		err = ConfirmExistingVerityDeviceCurrentValidity(verityDevPath)
		if err != nil {
			return err
		}

	} else {
		loopDev, err = losetup.Attach(fsImgFile, 0, true)
		if err != nil {
			return errors.WithStack(err)
		}
		defer func() { _ = loopDev.Detach() }()
		mountSourcePath = loopDev.Path()

	}

	err = errors.WithStack(unix.Mount(mountSourcePath, mountpoint, fsType, unix.MS_RDONLY, ""))
	if err != nil {
		if verityDevice != nil {
			_ = verityDevice.Deactivate(name)
			_ = loopDev.Detach()
		}
		if loopDevNeedsClosedOnErr {
			_ = loopDev.Detach()
		}
		return err
	}
	return nil
}

func findLoopBackingVerity(device string) (int64, error) {
	fi, err := os.Stat(device)
	if err != nil {
		return -1, errors.WithStack(err)
	}

	var minor uint32
	switch stat := fi.Sys().(type) {
	case *unix.Stat_t:
		minor = unix.Minor(uint64(stat.Rdev))
	case *syscall.Stat_t:
		minor = unix.Minor(uint64(stat.Rdev))
	default:
		return -1, errors.Errorf("unknown stat info type %T", stat)
	}

	ents, err := os.ReadDir(fmt.Sprintf("/sys/block/dm-%d/slaves", minor))
	if err != nil {
		return -1, errors.WithStack(err)
	}

	if len(ents) != 1 {
		return -1, errors.Errorf("too many slaves for %v", device)
	}
	loop := ents[0]

	deviceNo, err := strconv.ParseInt(strings.TrimPrefix(filepath.Base(loop.Name()), "loop"), 10, 64)
	if err != nil {
		return -1, errors.Wrapf(err, "bad loop dev %v", loop.Name())
	}

	return deviceNo, nil
}

func VerityUnmount(mountPath string) error {
	// find the loop device that backs the verity device
	deviceNo, err := findLoopBackingVerity(mountPath)
	if err != nil {
		return err
	}

	loopDev := losetup.New(uint64(deviceNo), 0)
	// here, we don't have the loopback device any more (we detached it
	// above). the cryptsetup API allows us to pass NULL for the crypt
	// device, but go-cryptsetup doesn't have a way to initialize a NULL
	// crypt device short of making the struct by hand like this.
	err = (&cryptsetup.Device{}).Deactivate(mountPath)
	if err != nil {
		return errors.WithStack(err)
	}

	// finally, kill the loop dev
	err = loopDev.Detach()
	if err != nil {
		return errors.Wrapf(err, "failed to detach loop dev for %v", mountPath)
	}

	return nil
}

// If we are using fuse, then we will be unable to get verity has from
// the mount device.  This is not a safe thing, we we only allow it when the
// device was mounted originally with AllowMissingVerityData.

const (
	rejectVerityFailure = false
	allowVerityFailure  = false
)

func ConfirmExistingVerityDeviceHash(devicePath string, rootHash string, allowVerityFailure bool) error {
	device := filepath.Base(devicePath)
	cDevice := C.CString(device)
	defer C.free(unsafe.Pointer(cDevice))

	var cParams *C.char

	rc := C.get_verity_table_params(cDevice, &cParams)
	if rc != 0 {
		if allowVerityFailure {
			return nil
		}
		return errors.Errorf("problem getting hash from %v: %v", device, rc)
	}
	defer C.free(unsafe.Pointer(cParams))

	params := C.GoString(cParams)

	// https://gitlab.com/cryptsetup/cryptsetup/-/wikis/DMVerity
	fields := strings.Fields(params)
	if len(fields) < 10 {
		return errors.Errorf("invalid dm params for %v: %v", device, params)
	}

	if rootHash != fields[8] {
		return errors.Errorf("invalid root hash for %v: %v (expected: %v)", device, fields[7], rootHash)
	}

	return nil
}

func ConfirmExistingVerityDeviceCurrentValidity(devicePath string) error {
	device := filepath.Base(devicePath)
	cDevice := C.CString(device)
	defer C.free(unsafe.Pointer(cDevice))

	var cParams *C.char

	rc := C.get_verity_status_params(cDevice, &cParams)
	if rc != 0 {
		return errors.Errorf("problem getting dm params from %v: %v", device, rc)
	}
	defer C.free(unsafe.Pointer(cParams))

	params := C.GoString(cParams)

	if len(params) != 1 {
		return errors.Errorf("invalid params for dm status for %q: %+v", device, params)
	}
	// valid values are "C": corruption has been found, or "V": no corruption found, yet.
	if params != "V" {
		return errors.Errorf("verity reports corruption on device %q", device)
	}
	return nil
}
