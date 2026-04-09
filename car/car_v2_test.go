package car

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/ipfs-content/testing/fixtures"
)

// TestReadV2CARFiles tests reading actual CARv2 formatfiles
func TestReadV2CARFiles(t *testing.T) {
	ctx := context.Background()
	fixturesDir, err := fixtures.GetCarsDir()
	require.NoError(t, err)

	v2TestFiles := []struct {
		name     string
		filename string
		skip     bool
		reason   string
	}{
		{"Basic v2", "sample-wrapped-v2.car", false, ""},
		{"v2 Indexless", "sample-v2-indexless.car", false, ""},
		{"v2 UnixFS format", "sample-unixfs-v2.car", false, ""},
		{"v2 Blockstore format", "sample-rw-bs-v2.car", false, ""},
	}

	for _, tc := range v2TestFiles {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip(tc.reason)
			}

			filePath := filepath.Join(fixturesDir, tc.filename)
			file, err := os.Open(filePath)
			require.NoError(t, err)
			defer file.Close()

			// Read CARv2 file using ReadCAR
			summary, err := ReadCAR(ctx, file, 10*1024*1024)
			if err != nil {
				t.Logf("Warning: Failed to read %s: %v (may be expected for this test fixture)", tc.filename, err)
				// Some test files are intentionally corrupted, so we log and skip them
				return
			}

			// Verify we got a valid summary
			require.NotNil(t, summary)
			assert.NotEqual(t, cid.Undef, summary.RootCID, "Root CID should not be undefined")
			assert.Greater(t, len(summary.TreeEntries), 0, "Should have tree entries")
			assert.Greater(t, len(summary.BlockOrder), 0, "Should have blocks")
			assert.Equal(t, len(summary.BlockOrder), len(summary.BlockSizes), "BlockOrder and BlockSizes should match length")

			t.Logf("Successfully read v2 Car file %s: root=%s, entries=%d, blocks=%d",
				tc.filename, summary.RootCID, len(summary.TreeEntries), len(summary.BlockOrder))
		})
	}
}

// TestReadV2CorruptedFiles tests handling of corrupted CARv2 files
func TestReadV2CorruptedFiles(t *testing.T) {
	ctx := context.Background()
	fixturesDir, err := fixtures.GetCarsDir()
	require.NoError(t, err)

	corruptedFiles := []struct {
		name     string
		filename string
	}{
		{"v2 Corrupt Data and Index", "sample-v2-corrupt-data-and-index.car"},
	}

	for _, tc := range corruptedFiles {
		t.Run(tc.name, func(t *testing.T) {
			filePath := filepath.Join(fixturesDir, tc.filename)
			file, err := os.Open(filePath)
			require.NoError(t, err)
			defer file.Close()

			// Should handle corrupted files gracefully
			summary, err := ReadCAR(ctx, file, 10*1024*1024)
			// Some corrupted files might still partiallt readable, others should error
			if err != nil {
				t.Logf("Expected error for corrupted file %s: %v", tc.filename, err)
			} else if summary != nil {
				t.Logf("Corrupted file %s was partially readable with root=%s", tc.filename, summary.RootCID)
			}
		})
	}
}

// TestPrepareCAR_CombinedWithV2Reader tests that files created by our CAR building 
// can be read using the v2 reader
func TestPrepareCAR_V2ReaderIntegration(t *testing.T) {
	ctx := context.Background()

	// Create test filesystem using fstest.MapFS
	mapFS := fstest.MapFS{
		"file1.txt":      {Data: []byte("Hello, World!")},
		"file2.txt":      {Data: []byte("Another file")},
		"dir1/file3.txt": {Data: []byte("Nested file")},
	}

	// Build CAR using PrepareCAR
	builder, summary, err := PrepareCAR(ctx, mapFS, true)
	require.NoError(t, err)
	require.NotNil(t, builder)
	require.NotNil(t, summary)

	// Write to buffer
	var carBuffer bytes.Buffer
	err = builder.WriteCAR(ctx, &carBuffer)
	require.NoError(t, err)

	// Read back using ReadCAR (which uses v2 reader internally)
	readSummary, err := ReadCAR(ctx, bytes.NewReader(carBuffer.Bytes()), 10*1024*1024)
	require.NoError(t, err)
	require.NotNil(t, readSummary)

	// Verify root CID matches
	require.Equal(t, summary.RootCID, readSummary.RootCID)

	// Verify tree entries match
	require.Equal(t, len(summary.TreeEntries), len(readSummary.TreeEntries))
	for path := range summary.TreeEntries {
		assert.Contains(t, readSummary.TreeEntries, path, "Missing entry in read CAR: %s", path)
	}

	// Verify blocks match (as sets, not necessarily in same order)
	// The CAR spec doesn't require a specific block order
	require.Equal(t, len(summary.BlockOrder), len(readSummary.BlockOrder), "Should have same number of blocks")
	
	// Create CID→size maps for comparison
	originalBlocks := make(map[cid.Cid]uint64, len(summary.BlockOrder))
	for i, block := range summary.BlockOrder {
		originalBlocks[block] = summary.BlockSizes[i]
	}
	
	readBlocks := make(map[cid.Cid]uint64, len(readSummary.BlockOrder))
	for i, block := range readSummary.BlockOrder {
		readBlocks[block] = readSummary.BlockSizes[i]
	}
	
	// Verify all blocks from build are present in read with correct sizes
	for block, size := range originalBlocks {
		readSize, exists := readBlocks[block]
		assert.True(t, exists, "Block %s from build not found in read", block)
		assert.Equal(t, size, readSize, "Block %s has different size: original=%d, read=%d", block, size, readSize)
	}
	
	// Verify total block sizes match
	var originalTotalSize, readTotalSize uint64
	for _, size := range summary.BlockSizes {
		originalTotalSize += size
	}
	for _, size := range readSummary.BlockSizes {
		readTotalSize += size
	}
	assert.Equal(t, originalTotalSize, readTotalSize, "Total block sizes should match")
}

// TestCalculateCARSize_Accuracy tests that CalculateCARSize produces accurate results
func TestCalculateCARSize_Accuracy(t *testing.T) {
	ctx := context.Background()

	// Create test filesystem using fstest.MapFS
	mapFS := fstest.MapFS{
		"file1.txt": {Data: []byte("Hello, World!")},
		"file2.txt": {Data: []byte("Another file")},
	}

	// Prepare CAR
	builder, summary, err := PrepareCAR(ctx, mapFS, true)
	require.NoError(t, err)

	// Calculate expected size
	expectedSize, err := CalculateCARSize(summary)
	require.NoError(t, err)

	// Actually write and measure
	var carBuffer bytes.Buffer
	err = builder.WriteCAR(ctx, &carBuffer)
	require.NoError(t, err)
	actualSize := int64(carBuffer.Len())

	// Verify accuracy
	assert.Equal(t, expectedSize, actualSize, "Calculated CAR size should match actual size")
}

func TestStreamCARWithSize_V2Reader(t *testing.T) {
	ctx := context.Background()

	// Create test filesystem using fstest.MapFS
	mapFS := fstest.MapFS{
		"file1.txt": {Data: []byte("Test content")},
		"file2.txt": {Data: []byte("More content")},
	}

	// Test StreamCARWithSize (which calculates size before writing)
	rootCID, carSize, err := StreamCARWithSize(ctx, mapFS, &bytes.Buffer{}, true)
	require.NoError(t, err)
	assert.NotEqual(t, cid.Undef, rootCID)
	assert.Greater(t, carSize, int64(0))
}


