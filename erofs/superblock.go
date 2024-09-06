package erofs

import (
	"encoding/binary"
	"io"
	"os"

	"github.com/pkg/errors"
)

/*

https://docs.kernel.org/filesystems/erofs.html

On-disk details

                              |-> aligned with the block size
 ____________________________________________________________
| |SB| | ... | Metadata | ... | Data | Metadata | ... | Data |
|_|__|_|_____|__________|_____|______|__________|_____|______|
0 +1K

*/

const (
	// Definitions for superblock.
	superblockMagicV1 = 0xe0f5e1e2
	superblockMagic   = superblockMagicV1
	superblockOffset  = 1024

	// Inode slot size in bit shift.
	InodeSlotBits = 5

	// Max file name length.
	MaxNameLen = 255
)

// Bit definitions for Inode*::Format.
const (
	InodeLayoutBit  = 0
	InodeLayoutBits = 1

	InodeDataLayoutBit  = 1
	InodeDataLayoutBits = 3
)

// Inode layouts.
const (
	InodeLayoutCompact  = 0
	InodeLayoutExtended = 1
)

// Inode data layouts.
const (
	InodeDataLayoutFlatPlain = iota
	InodeDataLayoutFlatCompressionLegacy
	InodeDataLayoutFlatInline
	InodeDataLayoutFlatCompression
	InodeDataLayoutChunkBased
	InodeDataLayoutMax
)

// Features w/ backward compatibility.
// This is not exhaustive, unused features are not listed.
const (
	FeatureCompatSuperBlockChecksum = 0x00000001
)

// Features w/o backward compatibility.
//
// Any features that aren't in FeatureIncompatSupported are incompatible
// with this implementation.
//
// This is not exhaustive, unused features are not listed.
const (
	FeatureIncompatSupported = 0x0
)

// Sizes of on-disk structures in bytes.
const (
	superblockSize    = 128
	InodeCompactSize  = 32
	InodeExtendedSize = 64
	DirentSize        = 12
)

type superblock struct {
	Magic           uint32
	Checksum        uint32
	FeatureCompat   uint32
	BlockSizeBits   uint8
	ExtSlots        uint8
	RootNid         uint16
	Inodes          uint64
	BuildTime       uint64
	BuildTimeNsec   uint32
	Blocks          uint32
	MetaBlockAddr   uint32
	XattrBlockAddr  uint32
	UUID            [16]uint8
	VolumeName      [16]uint8
	FeatureIncompat uint32
	Union1          uint16
	ExtraDevices    uint16
	DevTableSlotOff uint16
	Reserved        [38]uint8
}

/*
// checkRange checks whether the range [off, off+n) is valid.
func (i *Image) checkRange(off, n uint64) bool {
	size := uint64(len(i.bytes))
	end := off + n
	return off < size && off <= end && end <= size
}

// BytesAt returns the bytes at [off, off+n) of the image.
func (i *Image) BytesAt(off, n uint64) ([]byte, error) {
	if ok := i.checkRange(off, n); !ok {
		//log.Warningf("Invalid byte range (off: 0x%x, n: 0x%x) for image (size: 0x%x)", off, n, len(i.bytes))
		return nil, linuxerr.EFAULT
	}
	return i.bytes[off : off+n], nil
}

// unmarshalAt deserializes data from the bytes at [off, off+n) of the image.
func (i *Image) unmarshalAt(data marshal.Marshallable, off uint64) error {
	bytes, err := i.BytesAt(off, uint64(data.SizeBytes()))
	if err != nil {
		//log.Warningf("Failed to deserialize %T from 0x%x.", data, off)
		return err
	}
	data.UnmarshalUnsafe(bytes)
	return nil
}

// initSuperBlock initializes the superblock of this image.
func (i *Image) initSuperBlock() error {
	// i.sb is used in the hot path. Let's save a copy of the superblock.
	if err := i.unmarshalAt(&i.sb, SuperBlockOffset); err != nil {
		return fmt.Errorf("image size is too small")
	}

	if i.sb.Magic != SuperBlockMagicV1 {
		return fmt.Errorf("unknown magic: 0x%x", i.sb.Magic)
	}

	if err := i.verifyChecksum(); err != nil {
		return err
	}

	if featureIncompat := i.sb.FeatureIncompat & ^uint32(FeatureIncompatSupported); featureIncompat != 0 {
		return fmt.Errorf("unsupported incompatible features detected: 0x%x", featureIncompat)
	}

	if i.BlockSize()%hostarch.PageSize != 0 {
		return fmt.Errorf("unsupported block size: 0x%x", i.BlockSize())
	}

	return nil
}

// verifyChecksum verifies the checksum of the superblock.
func (i *Image) verifyChecksum() error {
	if i.sb.FeatureCompat&FeatureCompatSuperBlockChecksum == 0 {
		return nil
	}

	sb := i.sb
	sb.Checksum = 0
	table := crc32.MakeTable(crc32.Castagnoli)
	checksum := crc32.Checksum(marshal.Marshal(&sb), table)
// unmarshalAt deserializes data from the bytes at [off, off+n) of the image.
func (i *Image) unmarshalAt(data marshal.Marshallable, off uint64) error {
	bytes, err := i.BytesAt(off, uint64(data.SizeBytes()))
	if err != nil {
		log.Warningf("Failed to deserialize %T from 0x%x.", data, off)
		return err
	}
	data.UnmarshalUnsafe(bytes)
	return nil
}
	off := SuperBlockOffset + uint64(i.sb.SizeBytes())
	if bytes, err := i.BytesAt(off, uint64(i.BlockSize())-off); err != nil {
		return fmt.Errorf("image size is too small")
	} else {
		checksum = ^crc32.Update(checksum, table, bytes)
	}
	if checksum != i.sb.Checksum {
		return fmt.Errorf("invalid checksum: 0x%x, expected: 0x%x", checksum, i.sb.Checksum)
	}

	return nil
}
*/

func parseSuperblock(b []byte) (*superblock, error) {
	if len(b) != superblockSize {
		return nil, errors.Errorf("superblock had %d bytes instead of expected %d", len(b), superblockSize)
	}

	magic := binary.LittleEndian.Uint32(b[0:4])
	if magic != superblockMagic {
		return nil, errors.Errorf("superblock had magic of %d instead of expected %d", magic, superblockMagic)
	}

	// FIXME: also verify checksum

	s := &superblock{
		Magic:           magic, // b[0:4]
		Checksum:        binary.LittleEndian.Uint32(b[4:8]),
		FeatureCompat:   binary.LittleEndian.Uint32(b[8:12]),
		BlockSizeBits:   b[12], // b[12:13]
		ExtSlots:        b[13], // b[13:14]
		RootNid:         binary.LittleEndian.Uint16(b[14:16]),
		Inodes:          binary.LittleEndian.Uint64(b[16:24]),
		BuildTime:       binary.LittleEndian.Uint64(b[24:32]),
		BuildTimeNsec:   binary.LittleEndian.Uint32(b[32:36]),
		Blocks:          binary.LittleEndian.Uint32(b[36:40]),
		MetaBlockAddr:   binary.LittleEndian.Uint32(b[40:44]),
		XattrBlockAddr:  binary.LittleEndian.Uint32(b[44:48]),
		UUID:            [16]byte(b[48:64]),
		VolumeName:      [16]byte(b[64:80]),
		FeatureIncompat: binary.LittleEndian.Uint32(b[80:84]),
		Union1:          binary.LittleEndian.Uint16(b[84:86]),
		ExtraDevices:    binary.LittleEndian.Uint16(b[86:88]),
		DevTableSlotOff: binary.LittleEndian.Uint16(b[88:90]),
		Reserved:        [38]byte(b[90:128]),
	}

	return s, nil
}

func readSuperblock(path string) (*superblock, error) {
	reader, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	buf := make([]byte, superblockOffset+superblockSize)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, err
	}

	return parseSuperblock(buf[superblockOffset:])
}
