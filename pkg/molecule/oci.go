package molecule

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"path"
	"strings"

	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/umoci"
	"github.com/pkg/errors"
	stackeroci "machinerun.io/atomfs/pkg/oci"
)

type MountOCIOpts struct {
	OCIDir                 string
	Tag                    string
	Target                 string
	AddWriteableOverlay    bool
	WriteableOverlayPath   string
	AllowMissingVerityData bool
	MetadataDir            string
}

func (c MountOCIOpts) AtomsPath(parts ...string) string {
	atoms := path.Join(c.OCIDir, "blobs", "sha256")
	return path.Join(append([]string{atoms}, parts...)...)
}

func (c MountOCIOpts) WriteToFile(filename string) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filename, b, 0644)
	if err != nil {
		return err
	}
	return nil
}

func BuildMoleculeFromOCI(opts MountOCIOpts) (Molecule, error) {
	oci, err := umoci.OpenLayout(opts.OCIDir)
	if err != nil {
		return Molecule{}, err
	}
	defer oci.Close()

	man, err := stackeroci.LookupManifest(oci, opts.Tag)
	if err != nil {
		return Molecule{}, err
	}

	return buildMoleculeFromManifest(opts, man), nil
}

// BuildMoleculeFromOCIHash resolves opts.Tag in opts.OCIDir, computes the
// SHA-256 hash of the exact raw OCI manifest bytes, and compares it with
// expectedManifestSHA256. If they match, it builds the molecule from that
// same parsed manifest, so the tag is resolved only once and cannot redirect
// the molecule to a different manifest after verification. The expected hash
// may be supplied as 64 hexadecimal characters or with a "sha256:" prefix.
func BuildMoleculeFromOCIHash(opts MountOCIOpts, expectedManifestSHA256 string) (Molecule, error) {
	expectedManifestSHA256 = strings.TrimPrefix(expectedManifestSHA256, "sha256:")
	if len(expectedManifestSHA256) != sha256.Size*2 {
		return Molecule{}, errors.Errorf(
			"expected manifest SHA-256 must be 64 hexadecimal characters, got %q", expectedManifestSHA256)
	}
	if _, err := hex.DecodeString(expectedManifestSHA256); err != nil {
		return Molecule{}, errors.Wrapf(err, "invalid expected manifest SHA-256 %q", expectedManifestSHA256)
	}

	oci, err := umoci.OpenLayout(opts.OCIDir)
	if err != nil {
		return Molecule{}, errors.Wrapf(err, "failed opening OCI layout %q", opts.OCIDir)
	}
	defer oci.Close()

	manifest, rawManifest, err := stackeroci.LookupManifestWithRaw(oci, opts.Tag)
	if err != nil {
		return Molecule{}, errors.Wrapf(
			err, "failed looking up OCI manifest for tag %q in layout %q", opts.Tag, opts.OCIDir)
	}

	actualManifestSHA256 := sha256.Sum256(rawManifest)
	actualHash := hex.EncodeToString(actualManifestSHA256[:])
	if !strings.EqualFold(actualHash, expectedManifestSHA256) {
		return Molecule{}, errors.Errorf(
			"OCI manifest hash mismatch for tag %q in layout %q: got sha256:%s, expected sha256:%s",
			opts.Tag, opts.OCIDir, actualHash, strings.ToLower(expectedManifestSHA256),
		)
	}

	return buildMoleculeFromManifest(opts, manifest), nil
}

func buildMoleculeFromManifest(opts MountOCIOpts, manifest ispec.Manifest) Molecule {
	atoms := append([]ispec.Descriptor(nil), manifest.Layers...)

	// The OCI spec says that the first layer should be the bottom most
	// layer. In overlay it's the top most layer. Since the atomfs codebase
	// is mostly a wrapper around overlayfs, let's keep things in our db in
	// the same order that overlay expects them, i.e. the first layer is
	// the top most. That means we need to reverse the order in which the
	// atoms were inserted, because they were backwards.
	//
	// It's also terrible that golang doesn't have a reverse function, but
	// that's a discussion for a different block comment.
	for i := len(atoms)/2 - 1; i >= 0; i-- {
		opp := len(atoms) - 1 - i
		atoms[i], atoms[opp] = atoms[opp], atoms[i]
	}

	return Molecule{Atoms: atoms, config: opts}
}
