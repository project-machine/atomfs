package squashfs

import (
	"fmt"
	"strings"

	vrty "machinerun.io/atomfs/pkg/verity"
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

func GenerateSquashfsMediaType(comp SquashfsCompression, verity vrty.VerityMetadata) string {
	verityString := ""
	if verity {
		verityString = fmt.Sprintf("+%s", vrty.VeritySuffix)
	}
	return fmt.Sprintf("%s+%s%s", BaseMediaTypeLayerSquashfs, comp, verityString)
}
