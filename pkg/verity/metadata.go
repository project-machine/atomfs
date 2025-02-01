package verity

type VerityMetadata bool

const (
	VeritySuffix = "verity"

	VerityMetadataPresent VerityMetadata = true
	VerityMetadataMissing VerityMetadata = false
)
