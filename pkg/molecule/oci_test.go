package molecule

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

const testOCIImageTag = "test-version"

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
	err = oci.UpdateReference(ctx, testOCIImageTag, ispec.Descriptor{
		MediaType: ispec.MediaTypeImageManifest,
		Digest:    manifestDigest,
		Size:      manifestSize,
	})
	require.NoError(t, err)
	require.NoError(t, oci.Close())

	return layoutDir, manifest, rawManifest
}

func TestBuildMoleculeFromOCIHash(t *testing.T) {
	layoutDir, manifest, rawManifest := createTestOCIImage(t)
	hash := sha256.Sum256(rawManifest)
	expectedHash := "sha256:" + hex.EncodeToString(hash[:])

	mol, err := BuildMoleculeFromOCIHash(MountOCIOpts{
		OCIDir: layoutDir,
		Tag:    testOCIImageTag,
		Target: filepath.Join(t.TempDir(), "target"),
	}, expectedHash)
	require.NoError(t, err)
	require.Equal(t, []ispec.Descriptor{manifest.Layers[1], manifest.Layers[0]}, mol.Atoms)
}

func TestBuildMoleculeFromOCIHashRejectsMismatch(t *testing.T) {
	layoutDir, _, rawManifest := createTestOCIImage(t)
	hash := sha256.Sum256(rawManifest)
	wrongHash := hex.EncodeToString(hash[:])
	if wrongHash[0] == '0' {
		wrongHash = "1" + wrongHash[1:]
	} else {
		wrongHash = "0" + wrongHash[1:]
	}

	_, err := BuildMoleculeFromOCIHash(MountOCIOpts{
		OCIDir: layoutDir,
		Tag:    testOCIImageTag,
	}, wrongHash)
	require.ErrorContains(t, err, "OCI manifest hash mismatch")
}
