package oci

import (
	"bytes"
	"context"
	"io"

	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/umoci/oci/casext"
	"github.com/opencontainers/umoci/oci/casext/mediatype"
	"github.com/pkg/errors"
)

func LookupManifest(oci casext.Engine, tag string) (ispec.Manifest, error) {
	manifest, _, err := LookupManifestWithRaw(oci, tag)
	return manifest, err
}

// LookupManifestWithRaw resolves tag once and returns both the parsed image
// manifest and the exact bytes of its OCI manifest blob.
func LookupManifestWithRaw(oci casext.Engine, tag string) (ispec.Manifest, []byte, error) {
	descriptorPaths, err := oci.ResolveReference(context.Background(), tag)
	if err != nil {
		return ispec.Manifest{}, nil, err
	}

	if len(descriptorPaths) != 1 {
		return ispec.Manifest{}, nil, errors.Errorf("bad descriptor %s", tag)
	}

	descriptor := descriptorPaths[0].Descriptor()
	if descriptor.MediaType != ispec.MediaTypeImageManifest {
		return ispec.Manifest{}, nil, errors.Errorf("descriptor does not point to a manifest: %s", descriptor.MediaType)
	}

	reader, err := oci.GetVerifiedBlob(context.Background(), descriptor)
	if err != nil {
		return ispec.Manifest{}, nil, err
	}

	raw, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return ispec.Manifest{}, nil, errors.Wrap(readErr, "failed reading manifest blob")
	}
	if closeErr != nil {
		return ispec.Manifest{}, nil, errors.Wrap(closeErr, "failed verifying manifest blob")
	}

	parser := mediatype.GetParser(descriptor.MediaType)
	if parser == nil {
		return ispec.Manifest{}, nil, errors.Errorf("no parser registered for manifest media type %q", descriptor.MediaType)
	}
	parsed, err := parser(bytes.NewReader(raw))
	if err != nil {
		return ispec.Manifest{}, nil, errors.Wrap(err, "failed parsing manifest blob")
	}
	manifest, ok := parsed.(ispec.Manifest)
	if !ok {
		return ispec.Manifest{}, nil, errors.Errorf("parser returned %T, expected an image manifest", parsed)
	}

	return manifest, raw, nil
}

func LookupConfig(oci casext.Engine, desc ispec.Descriptor) (ispec.Image, error) {
	configBlob, err := oci.FromDescriptor(context.Background(), desc)
	if err != nil {
		return ispec.Image{}, err
	}

	if configBlob.Descriptor.MediaType != ispec.MediaTypeImageConfig {
		return ispec.Image{}, errors.Errorf("bad image config type: %s", configBlob.Descriptor.MediaType)
	}

	return configBlob.Data.(ispec.Image), nil

}

// UpdateImageConfig updates an oci tag with new config and new manifest
func UpdateImageConfig(oci casext.Engine, name string, newConfig ispec.Image, newManifest ispec.Manifest) (ispec.Descriptor, error) {
	configDigest, configSize, err := oci.PutBlobJSON(context.Background(), newConfig)
	if err != nil {
		return ispec.Descriptor{}, err
	}

	newManifest.Config = ispec.Descriptor{
		MediaType: ispec.MediaTypeImageConfig,
		Digest:    configDigest,
		Size:      configSize,
	}

	manifestDigest, manifestSize, err := oci.PutBlobJSON(context.Background(), newManifest)
	if err != nil {
		return ispec.Descriptor{}, err
	}

	desc := ispec.Descriptor{
		MediaType: ispec.MediaTypeImageManifest,
		Digest:    manifestDigest,
		Size:      manifestSize,
	}

	err = oci.UpdateReference(context.Background(), name, desc)
	if err != nil {
		return ispec.Descriptor{}, err
	}

	return desc, nil
}
