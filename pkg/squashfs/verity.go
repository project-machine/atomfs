package squashfs

// verityDataLocation returns the end of filesystem image where the verity data
// can be appended.
// squashfs image must be padded to be 4K aligned.
func verityDataLocation(sblock *superblock) (uint64, error) {
	squashLen := sblock.size

	// squashfs is padded out to the nearest 4k
	if squashLen%4096 != 0 {
		squashLen = squashLen + (4096 - squashLen%4096)
	}

	return squashLen, nil
}
