package erofs

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
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
	blockSize         = 4096

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

func verifyChecksum(sb *superblock, sbBlock []byte) error {
	if sb.FeatureCompat&FeatureCompatSuperBlockChecksum == 0 {
		return nil
	}

	sbsum := sb.Checksum

	// zero out Checksum field
	sbBlock[superblockOffset+4] = 0
	sbBlock[superblockOffset+5] = 0
	sbBlock[superblockOffset+6] = 0
	sbBlock[superblockOffset+7] = 0

	table := crc32.MakeTable(crc32.Castagnoli)

	checksum := crc32.Checksum(sbBlock[superblockOffset:superblockOffset+superblockSize], table)
	checksum = ^crc32.Update(checksum, table, sbBlock[superblockOffset+superblockSize:])
	if checksum != sbsum {
		return fmt.Errorf("invalid checksum: 0x%x, expected: 0x%x", checksum, sbsum)
	}

	sb.Checksum = sbsum

	return nil
}

func parseSuperblock(b []byte) (*superblock, error) {
	if len(b) != superblockSize {
		return nil, errors.Errorf("superblock had %d bytes instead of expected %d", len(b), superblockSize)
	}

	magic := binary.LittleEndian.Uint32(b[0:4])
	if magic != superblockMagic {
		return nil, errors.Errorf("superblock had magic of %d instead of expected %d", magic, superblockMagic)
	}

	sb := &superblock{
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

	if featureIncompat := sb.FeatureIncompat & ^uint32(FeatureIncompatSupported); featureIncompat != 0 {
		return nil, errors.Errorf("unsupported incompatible features detected: 0x%x", featureIncompat)
	}

	if (1<<sb.BlockSizeBits)%os.Getpagesize() != 0 {
		return nil, errors.Errorf("unsupported block size: 0x%x", 1<<sb.BlockSizeBits)
	}

	return sb, nil
}

func readSuperblock(path string) (*superblock, error) {
	reader, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// first read just enough to parse/validate the superblock
	buf := make([]byte, superblockOffset+superblockSize)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, err
	}

	sb, err := parseSuperblock(buf[superblockOffset:])
	if err != nil {
		return nil, err
	}

	// next read a full block and verify superblock checksum
	reader, err = os.Open(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	buf = make([]byte, 1<<sb.BlockSizeBits)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, errors.Wrapf(err, "image is too small")
	}

	if err := verifyChecksum(sb, buf); err != nil {
		return nil, errors.Wrapf(err, "superblock checksum mismatch")
	}

	return sb, nil
}
