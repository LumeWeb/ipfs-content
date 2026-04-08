package car

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// TestCARRoundTrip_NestedDirectories tests CAR round-trip with nested directory structures.
func TestCARRoundTrip_NestedDirectories(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Build filesystem with nested directories
	filesystem := fstest.MapFS{
		"file1.txt":           &fstest.MapFile{Data: []byte("content 1")},
		"dir1/file2.txt":      &fstest.MapFile{Data: []byte("file 2")},
		"dir1/file5.txt":      &fstest.MapFile{Data: []byte("file 5")},
		"dir1/dir2/file3.txt": &fstest.MapFile{Data: []byte("file 3")},
		"dir1/dir2/file4.txt": &fstest.MapFile{Data: []byte("file 4")},
	}

	// Build original CAR
	builder := newTestCARBuilder(t)
	originalSummary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)

	t.Logf("Original summary has %d entries:", len(originalSummary.TreeEntries))
	for path := range originalSummary.TreeEntries {
		t.Logf("  - %s", path)
	}

	// Write CAR to buffer
	var carBuf bytes.Buffer
	err = builder.WriteCAR(ctx, &carBuf)
	require.NoError(t, err)

	// Read CAR back
	reconstructedSummary, err := ReadCAR(ctx, bytes.NewReader(carBuf.Bytes()), DefaultMemoryLimit)
	require.NoError(t, err)

	t.Logf("Reconstructed summary has %d entries:", len(reconstructedSummary.TreeEntries))
	for path := range reconstructedSummary.TreeEntries {
		t.Logf("  - %s", path)
	}

	// Verify both summaries have the same number of entries
	require.Equal(t, len(originalSummary.TreeEntries), len(reconstructedSummary.TreeEntries),
		"Number of tree entries should match after round trip")

	// Verify ROOT CID matches
	require.Equal(t, originalSummary.RootCID, reconstructedSummary.RootCID,
		"Root CID should match after round trip")

	// Compare tree entries path by path
	for path, originalEntry := range originalSummary.TreeEntries {
		reconstructedEntry, exists := reconstructedSummary.TreeEntries[path]
		require.True(t, exists, "Path %s missing in reconstructed summary", path)

		// Verify entry properties match
		require.Equal(t, originalEntry.Name, reconstructedEntry.Name,
			"Name mismatch for path %s", path)
		require.Equal(t, originalEntry.IsDir, reconstructedEntry.IsDir,
			"IsDir mismatch for path %s", path)
		require.Equal(t, originalEntry.CID, reconstructedEntry.CID,
			"CID mismatch for path %s", path)

		// Verify children match
		require.Equal(t, len(originalEntry.Children), len(reconstructedEntry.Children),
			"Children count mismatch for path %s", path)

		for _, childPath := range originalEntry.Children {
			contains := slices.Contains(reconstructedEntry.Children, childPath)
			require.True(t, contains, "Child %s missing from %s children", childPath, path)
		}

		t.Logf("✓ Path %s: name=%s, isDir=%v, children=%d",
			path, originalEntry.Name, originalEntry.IsDir, len(originalEntry.Children))
	}

	// Verify ROOT children
	originalRootChildren := originalSummary.TreeEntries[ROOT].Children
	reconstructedRootChildren := reconstructedSummary.TreeEntries[ROOT].Children

	require.Equal(t, len(originalRootChildren), len(reconstructedRootChildren),
		"ROOT children count mismatch after round trip")

	for _, child := range originalRootChildren {
		contains := slices.Contains(reconstructedRootChildren, child)
		require.True(t, contains, "Child %s missing from reconstructed ROOT.Children", child)
	}

	// Count files
	fileCount := 0
	for _, entry := range originalSummary.TreeEntries {
		if !entry.IsDir {
			fileCount++
		}
	}
	t.Logf("Round trip successful: %d entries, %d files", len(originalSummary.TreeEntries), fileCount)
}

// TestCARRoundTrip_SingleFile tests CAR round-trip with a single file.
func TestCARRoundTrip_SingleFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	filesystem := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("hello world")},
	}

	// Build original summary
	builder := newTestCARBuilder(t)
	originalSummary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)

	// Write CAR to buffer
	var carBuf bytes.Buffer
	err = builder.WriteCAR(ctx, &carBuf)
	require.NoError(t, err)

	// Read CAR back
	reconstructedSummary, err := ReadCAR(ctx, bytes.NewReader(carBuf.Bytes()), DefaultMemoryLimit)
	require.NoError(t, err)

	// Verify match
	require.Equal(t, originalSummary.RootCID, reconstructedSummary.RootCID)
	require.Equal(t, len(originalSummary.TreeEntries), len(reconstructedSummary.TreeEntries))

	t.Logf("Single file round trip successful: %d entries", len(originalSummary.TreeEntries))
}

// TestCARRoundTrip_DeepNesting tests CAR round-trip with deeply nested directory structures.
func TestCARRoundTrip_DeepNesting(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create deeply nested structure
	filesystem := fstest.MapFS{
		"a/b/c/d/e/f/g/h/i.txt": &fstest.MapFile{Data: []byte("deep file")},
		"root.txt":              &fstest.MapFile{Data: []byte("root file")},
	}

	// Build original summary
	builder := newTestCARBuilder(t)
	originalSummary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)

	t.Logf("Deep nesting summary has %d entries:", len(originalSummary.TreeEntries))
	for path := range originalSummary.TreeEntries {
		t.Logf("  - %s", path)
	}

	// Write CAR to buffer
	var carBuf bytes.Buffer
	err = builder.WriteCAR(ctx, &carBuf)
	require.NoError(t, err)

	// Read CAR back
	reconstructedSummary, err := ReadCAR(ctx, bytes.NewReader(carBuf.Bytes()), DefaultMemoryLimit)
	require.NoError(t, err)

	t.Logf("Reconstructed deep nesting has %d entries:", len(reconstructedSummary.TreeEntries))
	for path := range reconstructedSummary.TreeEntries {
		t.Logf("  - %s", path)
	}

	// Verify all paths preserved
	for path := range originalSummary.TreeEntries {
		_, exists := reconstructedSummary.TreeEntries[path]
		require.True(t, exists, "Path %s lost in deep nesting round trip", path)
	}

	require.Equal(t, originalSummary.RootCID, reconstructedSummary.RootCID)

	t.Logf("Deep nesting round trip successful: %d entries", len(originalSummary.TreeEntries))
}

// TestCARRoundTrip_MultipleRootDirectories tests that multiple root-level directories
// are preserved correctly after round trip.
func TestCARRoundTrip_MultipleRootDirectories(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	filesystem := fstest.MapFS{
		"dir1/file.txt":     &fstest.MapFile{Data: []byte("file 1")},
		"dir2/other.txt":    &fstest.MapFile{Data: []byte("file 2")},
		"dir3/nested/f.txt": &fstest.MapFile{Data: []byte("file 3")},
		"rootfile.txt":      &fstest.MapFile{Data: []byte("root file")},
	}

	// Build original summary
	builder := newTestCARBuilder(t)
	originalSummary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)

	// Write CAR to buffer
	var carBuf bytes.Buffer
	err = builder.WriteCAR(ctx, &carBuf)
	require.NoError(t, err)

	// Read CAR back
	reconstructedSummary, err := ReadCAR(ctx, bytes.NewReader(carBuf.Bytes()), DefaultMemoryLimit)
	require.NoError(t, err)

	// Verify all root-level directories preserved
	require.Equal(t, originalSummary.RootCID, reconstructedSummary.RootCID)
	require.Equal(t, len(originalSummary.TreeEntries), len(reconstructedSummary.TreeEntries))

	// Verify ROOT only has files, not directories
	rootChildren := reconstructedSummary.TreeEntries[ROOT].Children
	t.Logf("ROOT children: %v", rootChildren)
	for _, child := range rootChildren {
		childEntry := reconstructedSummary.TreeEntries[child]
		require.False(t, childEntry.IsDir, "ROOT should only contain files, not directories")
	}

	// Verify directories exist in TreeEntries
	for _, dir := range []string{"dir1", "dir2", "dir3"} {
		_, exists := reconstructedSummary.TreeEntries[dir]
		require.True(t, exists, "Directory %s should exist after round trip", dir)
	}

	t.Logf("Multiple root directories round trip successful: %d entries", len(originalSummary.TreeEntries))
}
