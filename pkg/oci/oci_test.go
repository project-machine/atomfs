package oci

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"testing"

	specs "github.com/opencontainers/image-spec/specs-go"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/umoci"
	"github.com/stretchr/testify/require"
)

const testImageTag = "test-version"

func createTestOCIImage(t *testing.T) (string, ispec.Manifest, []byte) {
	t.Helper()

	layoutDir := filepath.Join(t.TempDir(), "oci")
	oci, err := umoci.CreateLayout(layoutDir)
	require.NoError(t, err)

	ctx := context.Background()
	configDigest, configSize, err := oci.PutBlobJSON(ctx, ispec.Image{})
	require.NoError(t, err)
	layers := make([]ispec.Descriptor, 0, 2)
	for _, content := range []string{"first layer", "second layer"} {
		layerDigest, layerSize, err := oci.PutBlob(ctx, bytes.NewBufferString(content))
		require.NoError(t, err)
		layers = append(layers, ispec.Descriptor{
			MediaType: ispec.MediaTypeImageLayer,
			Digest:    layerDigest,
			Size:      layerSize,
		})
	}

	manifest := ispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ispec.MediaTypeImageManifest,
		Config: ispec.Descriptor{
			MediaType: ispec.MediaTypeImageConfig,
			Digest:    configDigest,
			Size:      configSize,
		},
		Layers: layers,
	}
	rawManifest, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDigest, manifestSize, err := oci.PutBlob(ctx, bytes.NewReader(rawManifest))
	require.NoError(t, err)
	err = oci.UpdateReference(ctx, testImageTag, ispec.Descriptor{
		MediaType: ispec.MediaTypeImageManifest,
		Digest:    manifestDigest,
		Size:      manifestSize,
	})
	require.NoError(t, err)
	require.NoError(t, oci.Close())

	return layoutDir, manifest, rawManifest
}

func TestLookupManifestWithRaw(t *testing.T) {
	layoutDir, wantManifest, wantRaw := createTestOCIImage(t)
	oci, err := umoci.OpenLayout(layoutDir)
	require.NoError(t, err)
	defer oci.Close()

	gotManifest, gotRaw, err := LookupManifestWithRaw(oci, testImageTag)
	require.NoError(t, err)
	require.Equal(t, wantManifest, gotManifest)
	require.Equal(t, wantRaw, gotRaw)

	// Keep the existing API behavior covered while it delegates to the helper.
	gotManifest, err = LookupManifest(oci, testImageTag)
	require.NoError(t, err)
	require.Equal(t, wantManifest, gotManifest)
}

func TestLookupManifestWithRawUnknownTag(t *testing.T) {
	layoutDir, _, _ := createTestOCIImage(t)
	oci, err := umoci.OpenLayout(layoutDir)
	require.NoError(t, err)
	defer oci.Close()

	_, _, err = LookupManifestWithRaw(oci, "missing-tag")
	require.Error(t, err)
}

func TestListTagManifestSHA256ReturnsSortedTagsAndRawManifestDigests(t *testing.T) {
	layoutDir, _, rawManifest := createTestOCIImage(t)
	oci, err := umoci.OpenLayout(layoutDir)
	require.NoError(t, err)
	defer oci.Close()

	index, err := oci.GetIndex(context.Background())
	require.NoError(t, err)
	require.Len(t, index.Manifests, 1)
	require.NoError(t, oci.UpdateReference(context.Background(), "alpha", index.Manifests[0]))

	digest := sha256.Sum256(rawManifest)
	wantDigest := hex.EncodeToString(digest[:])
	got, err := ListTagManifestSHA256(oci)
	require.NoError(t, err)
	require.Equal(t, []TagManifestSHA256{
		{Tag: "alpha", SHA256: wantDigest},
		{Tag: testImageTag, SHA256: wantDigest},
	}, got)
}
