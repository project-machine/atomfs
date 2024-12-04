package verity

import "strings"

type VerityMetadata bool

const (
	VeritySuffix = "verity"

	VerityMetadataPresent VerityMetadata = true
	VerityMetadataMissing VerityMetadata = false
)

func HasVerityMetadata(mediaType string) VerityMetadata {
	return VerityMetadata(strings.HasSuffix(mediaType, VeritySuffix))
}
