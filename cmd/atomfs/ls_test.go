package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	specs "github.com/opencontainers/image-spec/specs-go"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/umoci"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func createLSCLITestLayout(t *testing.T) (string, string) {
	t.Helper()

	layoutPath := filepath.Join(t.TempDir(), "oci")
	oci, err := umoci.CreateLayout(layoutPath)
	require.NoError(t, err)

	ctx := context.Background()
	configDigest, configSize, err := oci.PutBlobJSON(ctx, ispec.Image{})
	require.NoError(t, err)
	manifest := ispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ispec.MediaTypeImageManifest,
		Config: ispec.Descriptor{
			MediaType: ispec.MediaTypeImageConfig,
			Digest:    configDigest,
			Size:      configSize,
		},
		Layers: []ispec.Descriptor{{MediaType: ispec.MediaTypeImageLayer}},
	}
	rawManifest, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDigest, manifestSize, err := oci.PutBlob(ctx, bytes.NewReader(rawManifest))
	require.NoError(t, err)
	require.NoError(t, oci.UpdateReference(ctx, "latest", ispec.Descriptor{
		MediaType: ispec.MediaTypeImageManifest,
		Digest:    manifestDigest,
		Size:      manifestSize,
	}))
	require.NoError(t, oci.Close())

	digest := sha256.Sum256(rawManifest)
	return layoutPath, hex.EncodeToString(digest[:])
}

func runLSCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var output bytes.Buffer
	app := cli.NewApp()
	app.Name = "atomfs"
	app.Commands = []cli.Command{lsCmd}
	app.Writer = &output
	err := app.Run(append([]string{"atomfs"}, args...))
	return output.String(), err
}

func TestLSCommandListsTagsWithoutDigestsByDefault(t *testing.T) {
	layoutPath, _ := createLSCLITestLayout(t)

	output, err := runLSCommand(t, "ls", layoutPath)
	require.NoError(t, err)
	require.Equal(t, "latest\n", output)
}

func TestLSCommandIncludesManifestSHA256WhenRequested(t *testing.T) {
	layoutPath, expectedDigest := createLSCLITestLayout(t)

	output, err := runLSCommand(t, "ls", "--sha256", layoutPath)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("latest@sha256:%s\n", expectedDigest), output)
}

func TestLSCommandRejectsWrongNumberOfArguments(t *testing.T) {
	_, err := runLSCommand(t, "ls")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Usage: atomfs ls")
}

func TestFindImageParsesDigestQualifiedReference(t *testing.T) {
	layoutPath := t.TempDir()
	digest := "sha256:" + strings.Repeat("a", 64)

	gotLayout, gotTag, gotDigest, err := findImage(layoutPath + ":latest@" + digest)
	require.NoError(t, err)
	require.Equal(t, layoutPath, gotLayout)
	require.Equal(t, "latest", gotTag)
	require.Equal(t, digest, gotDigest)
}

func TestFindImageRejectsUnsupportedDigestAlgorithm(t *testing.T) {
	layoutPath := t.TempDir()

	_, _, _, err := findImage(layoutPath + ":latest@sha512:abcd")
	require.ErrorContains(t, err, "expected @sha256:<digest>")
}
