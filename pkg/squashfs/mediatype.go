package squashfs

import (
	"fmt"
	"strings"

	"machinerun.io/atomfs/verity"
)

type SquashfsCompression string

const (
	BaseMediaTypeLayerSquashfs = "application/vnd.stacker.image.layer.squashfs"

	GzipCompression SquashfsCompression = "gzip"
	ZstdCompression SquashfsCompression = "zstd"
)

func IsSquashfsMediaType(mediaType string) bool {
	return strings.HasPrefix(mediaType, BaseMediaTypeLayerSquashfs)
}

func GenerateSquashfsMediaType(comp SquashfsCompression, verity VerityMetadata) string {
	verityString := ""
	if verity {
		verityString = fmt.Sprintf("+%s", verity.VeritySuffix)
	}
	return fmt.Sprintf("%s+%s%s", BaseMediaTypeLayerSquashfs, comp, verityString)
}

func HasVerityMetadata(mediaType string) VerityMetadata {
	return VerityMetadata(strings.HasSuffix(mediaType, verity.VeritySuffix))
}
