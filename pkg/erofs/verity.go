package erofs

// verityDataLocation returns the end of filesystem image where the verity data
// can be appended.
// erofs image is always 4K aligned (default block size is 4k)
func verityDataLocation(sblock *superblock) (uint64, error) {
	return uint64((uint64)(sblock.Blocks) * (1 << sblock.BlockSizeBits)), nil
}
