package erofs

import (
	"fmt"
	"strings"

	vrty "machinerun.io/atomfs/pkg/verity"
)

type ErofsCompression string

const (
	BaseMediaTypeLayerErofs = "application/vnd.stacker.image.layer.erofs"

	LZ4HCCompression ErofsCompression = "lz4hc"
	LZ4Compression   ErofsCompression = "lz4"
	ZstdCompression  ErofsCompression = "zstd"
)

func IsErofsMediaType(mediaType string) bool {
	return strings.HasPrefix(mediaType, BaseMediaTypeLayerErofs)
}

func GenerateErofsMediaType(comp ErofsCompression, verity vrty.VerityMetadata) string {
	verityString := ""
	if verity {
		verityString = fmt.Sprintf("+%s", vrty.VeritySuffix)
	}
	return fmt.Sprintf("%s+%s%s", BaseMediaTypeLayerErofs, comp, verityString)
}
