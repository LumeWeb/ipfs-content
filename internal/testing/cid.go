// Package testing provides common testing utilities for the ipfs-content library.
package testing

import (
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
)

// CIDOptions provides configuration for creating test CIDs.
type CIDOptions struct {
	// Codec specifies the IPLD codec (e.g., cid.DagProtobuf, cid.DagCBOR, cid.Raw)
	Codec uint64
	// MhType specifies the multihash type (e.g., multihash.SHA2_256, multihash.SHA3_256)
	MhType uint64
	// MhLength specifies the multihash length (-1 for default/truncated by output size)
	MhLength int
}

// DefaultCIDOptions returns the default CID options (DagProtobuf with SHA2_256).
func DefaultCIDOptions() CIDOptions {
	return CIDOptions{
		Codec:    cid.DagProtobuf,
		MhType:   multihash.SHA2_256,
		MhLength: -1,
	}
}

// RawCIDOptions returns options for creating raw blocks (no IPLD structure).
func RawCIDOptions() CIDOptions {
	return CIDOptions{
		Codec:    cid.Raw,
		MhType:   multihash.SHA2_256,
		MhLength: -1,
	}
}

// DagCBORCIDOptions returns options for creating DAG-CBOR nodes.
func DagCBORCIDOptions() CIDOptions {
	return CIDOptions{
		Codec:    cid.DagCBOR,
		MhType:   multihash.SHA2_256,
		MhLength: -1,
	}
}

// GenerateCID creates a test CID from the given data and options.
// This is useful for creating deterministic test CIDs for testing purposes.
func GenerateCID(t *testing.T, data []byte, opts CIDOptions) cid.Cid {
	t.Helper()

	hash, err := multihash.Sum(data, opts.MhType, opts.MhLength)
	require.NoError(t, err, "Failed to create multihash")

	c := cid.NewCidV1(opts.Codec, hash)
	return c
}

// GenerateCIDFromString creates a test CID from a string and options.
// This is a convenience wrapper around GenerateCID.
func GenerateCIDFromString(t *testing.T, s string, opts CIDOptions) cid.Cid {
	t.Helper()
	return GenerateCID(t, []byte(s), opts)
}

// MustGenerateCID creates a test CID from a string with default options.
// Panics on error (use GenerateCID for testing with require.NoError).
func MustGenerateCID(data string) cid.Cid {
	c, err := cid.Prefix{
		Version:  1,
		Codec:    cid.DagProtobuf,
		MhType:   multihash.SHA2_256,
		MhLength: -1,
	}.Sum([]byte(data))
	if err != nil {
		panic(err)
	}
	return c
}

// GenerateTestCID creates a test CID from a string with DagProtobuf codec.
// This is the most commonly used CID type for UnixFS/IPFS testing.
func GenerateTestCID(t *testing.T, data string) cid.Cid {
	t.Helper()
	return GenerateCIDFromString(t, data, DefaultCIDOptions())
}

// GenerateRawBlockCID creates a CID for raw block data (no IPLD structure).
func GenerateRawBlockCID(t *testing.T, data []byte) cid.Cid {
	t.Helper()

	hash, err := multihash.Sum(data, multihash.SHA2_256, -1)
	require.NoError(t, err, "Failed to create multihash")

	return cid.NewCidV1(cid.Raw, hash)
}

// GenerateRawBlockCIDFromString creates a raw block CID from a string.
func GenerateRawBlockCIDFromString(t *testing.T, s string) cid.Cid {
	t.Helper()
	return GenerateRawBlockCID(t, []byte(s))
}

// GenerateDistinctCID creates a unique CID with an index byte.
// Useful when you need multiple distinct CIDs for testing.
func GenerateDistinctCID(t *testing.T, index byte) cid.Cid {
	t.Helper()
	return GenerateCID(t, []byte{index}, DefaultCIDOptions())
}

// ParseTestCID parses a CID string for testing.
// Panics if the CID is invalid.
func ParseTestCID(t *testing.T, s string) cid.Cid {
	t.Helper()

	c, err := cid.Parse(s)
	require.NoError(t, err, "Failed to parse test CID: %s", s)
	return c
}

// CIDsEqual checks if two CIDs are equal.
// This is a wrapper around cid.Equals for convenience in tests.
func CIDsEqual(t *testing.T, expected, actual cid.Cid) bool {
	t.Helper()
	return expected.Equals(actual)
}

// AssertCIDsEqual asserts that two CIDs are equal.
func AssertCIDsEqual(t *testing.T, expected, actual cid.Cid, msgAndArgs ...interface{}) {
	t.Helper()
	require.True(t, CIDsEqual(t, expected, actual), msgAndArgs...)
}
