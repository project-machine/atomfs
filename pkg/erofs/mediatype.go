package erofs

import (
	"fmt"
	"strings"
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

func GenerateErofsMediaType(comp ErofsCompression) string {
	return fmt.Sprintf("%s+%s", BaseMediaTypeLayerErofs, comp)
}
