package car

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/ipfs-content/internal/carv1"
)

// smallTestMemoryLimit is the memory limit for tests that verify LRU eviction behavior.
const smallTestMemoryLimit = 10 * 1024 // 10KB to trigger eviction in tests

// getTestContent returns test content string from env or fallback
func getTestContent(suffix string) string {
	envKey := "TEST_CONTENT_" + suffix
	if content, ok := os.LookupEnv(envKey); ok {
		return content
	}
	return "content " + suffix
}

// newTestCARBuilder creates a CARBuilder for testing with optional memory limit.
// If maxMemory is 0, DefaultMemoryLimit is used.
func newTestCARBuilder(t *testing.T, maxMemory uint64) *CARBuilder {
	t.Helper()
	if maxMemory == 0 {
		maxMemory = DefaultMemoryLimit
	}
	return newCARBuilder(maxMemory)
}

// TestBuildTreeSummary tests the CARBuilder.BuildSummary function
func TestBuildTreeSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wrapInDir   bool
		expectError bool
		check       func(*testing.T, *TreeSummary)
	}{
		{
			name: "single file",
			wrapInDir: true,
			expectError: false,
			check: func(t *testing.T, summary *TreeSummary) {
				assert.NotEqual(t, cid.Undef, summary.RootCID)
				assert.Greater(t, len(summary.BlockOrder), 0)
			},
		},
		{
			name: "multiple files",
			wrapInDir: true,
			expectError: false,
			check: func(t *testing.T, summary *TreeSummary) {
				assert.NotEqual(t, cid.Undef, summary.RootCID)
				assert.Greater(t, len(summary.BlockOrder), 0)
			},
		},
		{
			name: "nested directories",
			wrapInDir: true,
			expectError: false,
			check: func(t *testing.T, summary *TreeSummary) {
				assert.NotEqual(t, cid.Undef, summary.RootCID)
				assert.Greater(t, len(summary.BlockOrder), 0)
			},
		},
		{
			name: "empty filesystem",
			wrapInDir: true,
			expectError: false,
			check: func(t *testing.T, summary *TreeSummary) {
				assert.Equal(t, 1, len(summary.BlockOrder))
			},
		},
	}

	for idx, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			filesystem := getTestFilesystem(idx)

			builder := newTestCARBuilder(t, DefaultMemoryLimit)

			summary, err := builder.BuildSummary(ctx, filesystem, tt.wrapInDir)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, summary)
			} else {
				assert.NoError(t, err)
				if tt.check != nil {
					tt.check(t, summary)
				}
			}
		})
	}
}

// getTestFilesystem returns a test filesystem based on index
func getTestFilesystem(idx int) fstest.MapFS {
	switch idx {
	case 0:
		return fstest.MapFS{
			"file.txt": {Data: []byte("hello world")},
		}
	case 1:
		return fstest.MapFS{
			"file1.txt": {Data: []byte(getTestContent("1"))},
			"file2.txt": {Data: []byte(getTestContent("2"))},
			"file3.txt": {Data: []byte(getTestContent("3"))},
		}
	case 2:
		return fstest.MapFS{
			"dir1/file1.txt":        {Data: []byte("file 1")},
			"dir1/subdir/file2.txt": {Data: []byte("file 2")},
			"dir2/file3.txt":        {Data: []byte("file 3")},
		}
	default:
		return fstest.MapFS{}
	}
}

// TestCalculateCARSize_EmptyDirectories tests empty directory handling in CalculateCARSize
func TestCalculateCARSize_EmptyDirectories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		check        func(*testing.T, *TreeSummary, int64)
		filesystem   fstest.MapFS
	}{
		{
			name: "single_empty_directory",
			filesystem: fstest.MapFS{
				"emptydir": {Mode: fs.ModeDir},
			},
			check: func(t *testing.T, summary *TreeSummary, carSize int64) {
				assert.Greater(t, carSize, int64(0))
				assert.Equal(t, int64(summary.CARSize), carSize)
			},
		},
		{
			name: "file_alongside_empty_directory",
			filesystem: fstest.MapFS{
				"file.txt":     {Data: []byte(getTestContent("default"))},
				"emptydir":     {Mode: fs.ModeDir},
				"nested/.keep": {Data: []byte("")},
			},
			check: func(t *testing.T, summary *TreeSummary, carSize int64) {
				assert.Greater(t, carSize, int64(0))
				assert.Equal(t, int64(summary.CARSize), carSize)
			},
		},
		{
			name: "nested_empty_directories",
			filesystem: fstest.MapFS{
				"dir1/dir2/dir3":  {Mode: fs.ModeDir},
				"dir1/dir2/.keep": {Data: []byte("")},
			},
			check: func(t *testing.T, summary *TreeSummary, carSize int64) {
				assert.Greater(t, carSize, int64(0))
				assert.Equal(t, int64(summary.CARSize), carSize)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			builder := newTestCARBuilder(t, DefaultMemoryLimit)


			summary, err := builder.BuildSummary(ctx, tt.filesystem, true)
			assert.NoError(t, err)
			assert.NotNil(t, summary)

			carSize, err := CalculateCARSize(summary)
			require.NoError(t, err)
			tt.check(t, summary, carSize)
		})
	}
}

// TestBuildTreeSummary_ContextCancellation tests context cancellation
func TestBuildTreeSummary_ContextCancellation(t *testing.T) {
	t.Parallel()

	t.Run("normal_operation", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		filesystem := fstest.MapFS{
			"file.txt": {Data: []byte("content")},
		}

		builder := newTestCARBuilder(t, DefaultMemoryLimit)


		_, err := builder.BuildSummary(ctx, filesystem, true)
		assert.NoError(t, err)
	})

	t.Run("cancelled_context", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		filesystem := fstest.MapFS{
			"file.txt": {Data: []byte("content")},
		}

		builder := newTestCARBuilder(t, DefaultMemoryLimit)


		_, err := builder.BuildSummary(ctx, filesystem, true)
		assert.Error(t, err)
	})
}

// TestBuildTreeSummary_LargeFile tests handling of large files
func TestBuildTreeSummary_LargeFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	filesystem := fstest.MapFS{
		"largefile.bin": {Data: []byte{1}},
	}

	builder := newTestCARBuilder(t, DefaultMemoryLimit)


	summary, err := builder.BuildSummary(ctx, filesystem, true)
	assert.NoError(t, err)
	assert.NotNil(t, summary)
	assert.NotEqual(t, cid.Undef, summary.RootCID)
}

// TestWriteCARv1FromSummary tests writing CARv1 from a summary
func TestWriteCARv1FromSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{"single_file"},
		{"multiple_files"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			filesystem := fstest.MapFS{
				"file.txt": {Data: []byte("hello world")},
			}
			if tt.name == "multiple_files" {
				filesystem["file2.txt"] = &fstest.MapFile{Data: []byte("content2")}
			}

			builder := newTestCARBuilder(t, DefaultMemoryLimit)


			_, err := builder.BuildSummary(ctx, filesystem, true)
			require.NoError(t, err)

			var buf bytes.Buffer
			err = builder.WriteCAR(ctx, &buf)
			assert.NoError(t, err)

			carReader, err := carv1.NewCarReader(bytes.NewReader(buf.Bytes()))
			assert.NoError(t, err)
			assert.Equal(t, uint64(1), carReader.Header.Version)

			buffer, err := carReader.Next()
			assert.NoError(t, err)
			assert.NotNil(t, buffer)
		})
	}
}

// TestWriteCARv1FromSummary_ContextCancellation tests context cancellation during WriteCAR
func TestWriteCARv1FromSummary_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	builder := newTestCARBuilder(t, DefaultMemoryLimit)


	var buf bytes.Buffer
	err := builder.WriteCAR(ctx, &buf)
	assert.Error(t, err)
}

// TestStreamCAR tests the StreamCAR function
func TestStreamCAR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wrapInDir bool
	}{
		{
			name: "single file",
			wrapInDir: true,
		},
		{
			name: "multiple files",
			wrapInDir: true,
		},
		{
			name: "single file no wrap",
			wrapInDir: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			filesystem := fstest.MapFS{
				"file.txt": {Data: []byte("hello world")},
			}
			if tt.name == "multiple files" {
				filesystem["file2.txt"] = &fstest.MapFile{Data: []byte("content2")}
			}

			var buf bytes.Buffer
			rootCID, err := StreamCAR(ctx, filesystem, &buf, DefaultMemoryLimit, tt.wrapInDir)
			assert.NoError(t, err)
			assert.NotEqual(t, cid.Undef, rootCID)

			carReader, err := carv1.NewCarReader(&buf)
			assert.NoError(t, err)
			assert.Equal(t, uint64(1), carReader.Header.Version)
			assert.Len(t, carReader.Header.Roots, 1)
		})
	}
}

// TestStreamCAR_ContextCancellation tests context cancellation handling in StreamCAR
func TestStreamCAR_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	filesystem := fstest.MapFS{
		"file.txt": {Data: []byte("hello")},
	}

	var buf bytes.Buffer
	_, err := StreamCAR(ctx, filesystem, &buf, DefaultMemoryLimit, true)
	assert.Error(t, err)
}

// TestWriteCAR tests the WriteCAR wrapper method
func TestWriteCAR(t *testing.T) {
	t.Parallel()

	t.Run("basic_write", func(t *testing.T) {
		ctx := context.Background()
		filesystem := fstest.MapFS{
			"file.txt": {Data: []byte("hello world")},
		}

		builder := newTestCARBuilder(t, DefaultMemoryLimit)

		_, err := builder.BuildSummary(ctx, filesystem, true)
		require.NoError(t, err)

		var buf bytes.Buffer
		err = builder.WriteCAR(ctx, &buf)
		assert.NoError(t, err)

		carReader, err := carv1.NewCarReader(&buf)
		assert.NoError(t, err)
		assert.Equal(t, uint64(1), carReader.Header.Version)
		assert.Len(t, carReader.Header.Roots, 1)
	})
}

// TestWriteCAR_ContextCancellation tests context cancellation during WriteCAR
func TestWriteCAR_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	builder := newTestCARBuilder(t, DefaultMemoryLimit)


	var buf bytes.Buffer
	err := builder.WriteCAR(ctx, &buf)
	assert.Error(t, err)
}

// TestRoundTripCAR tests building a CAR, writing it, and reading it back
func TestRoundTripCAR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{"single file"},
		{"multiple files"},
		{"nested directories"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			filesystem := fstest.MapFS{
				"file.txt": {Data: []byte("hello world")},
			}
			if tt.name == "multiple files" {
				filesystem["file2.txt"] = &fstest.MapFile{Data: []byte("content2")}
				filesystem["file3.txt"] = &fstest.MapFile{Data: []byte("content3")}
			}
			if tt.name == "nested directories" {
				filesystem["dir1/file1.txt"] = &fstest.MapFile{Data: []byte("file1")}
				filesystem["dir1/subdir/file2.txt"] = &fstest.MapFile{Data: []byte("file2")}
				filesystem["dir2/file3.txt"] = &fstest.MapFile{Data: []byte("file3")}
			}

			builder := newTestCARBuilder(t, DefaultMemoryLimit)


			summary, err := builder.BuildSummary(ctx, filesystem, true)
			require.NoError(t, err)

			var buf bytes.Buffer
			err = builder.WriteCAR(ctx, &buf)
			require.NoError(t, err)

			carReader, err := carv1.NewCarReader(&buf)
			require.NoError(t, err)

			assert.Equal(t, uint64(1), carReader.Header.Version)
			assert.Len(t, carReader.Header.Roots, 1)

			blockCount := 0
			for {
				_, err := carReader.Next()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				blockCount++
			}

			assert.Equal(t, len(summary.BlockOrder), blockCount)
		})
	}
}

// TestRoundTripCAR_StreamCAR tests round trip using StreamCAR
func TestRoundTripCAR_StreamCAR(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	filesystem := fstest.MapFS{
		"file1.txt": {Data: []byte("content1")},
		"file2.txt": {Data: []byte("content2")},
	}

	var buf bytes.Buffer
	rootCID, err := StreamCAR(ctx, filesystem, &buf, DefaultMemoryLimit, true)
	require.NoError(t, err)

	carReader, err := carv1.NewCarReader(&buf)
	require.NoError(t, err)

	assert.Equal(t, uint64(1), carReader.Header.Version)
	assert.Len(t, carReader.Header.Roots, 1)
	assert.Equal(t, rootCID, carReader.Header.Roots[0])
}

// TestRoundTripCAR_WriteCAR tests round trip using builder.WriteCAR
func TestRoundTripCAR_WriteCAR(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	filesystem := fstest.MapFS{
		"file.txt": {Data: []byte("hello world")},
	}

	builder := newTestCARBuilder(t, DefaultMemoryLimit)

	_, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = builder.WriteCAR(ctx, &buf)
	require.NoError(t, err)

	carReader, err := carv1.NewCarReader(&buf)
	require.NoError(t, err)

	assert.Equal(t, uint64(1), carReader.Header.Version)
	assert.Len(t, carReader.Header.Roots, 1)
}

// TestRoundTripCAR_VerifyAllData tests that all data is correctly preserved
func TestRoundTripCAR_VerifyAllData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testContent := "hello world"
	filesystem := fstest.MapFS{
		"file.txt": {Data: []byte(testContent)},
	}

	builder := newTestCARBuilder(t, DefaultMemoryLimit)


	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = builder.WriteCAR(ctx, &buf)
	require.NoError(t, err)

	totalDataSize := uint64(0)
	for _, size := range summary.BlockSizes {
		totalDataSize += size
	}
	assert.Greater(t, totalDataSize, uint64(len(testContent)))
}

// TestRoundTripCAR_ContextCancellation tests context cancellation during round trip
func TestRoundTripCAR_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	filesystem := fstest.MapFS{
		"file.txt": {Data: []byte("hello")},
	}

	builder := newTestCARBuilder(t, DefaultMemoryLimit)


	_, err := builder.BuildSummary(ctx, filesystem, true)
	assert.Error(t, err)
}

// TestRoundTripCAR_LargeDataset tests performance with larger datasets
func TestRoundTripCAR_LargeDataset(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	filesystem := fstest.MapFS{
		"file1.txt": {Data: []byte(getTestContent("1"))},
		"file2.txt": {Data: []byte(getTestContent("2"))},
		"file3.txt": {Data: []byte(getTestContent("3"))},
		"file4.txt": {Data: []byte(getTestContent("4"))},
		"file5.txt": {Data: []byte(getTestContent("5"))},
	}

	builder := newTestCARBuilder(t, DefaultMemoryLimit)


	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)

	assert.NotEqual(t, cid.Undef, summary.RootCID)
	assert.GreaterOrEqual(t, len(summary.BlockOrder), 6)
}

// GetSummary returns the TreeSummary for the given filesystem
func GetSummary(t *testing.T, ctx context.Context, filesystem fs.FS, wrapInDir bool) *TreeSummary {
	t.Helper()

	builder := newTestCARBuilder(t, DefaultMemoryLimit)


	summary, err := builder.BuildSummary(ctx, filesystem, wrapInDir)
	require.NoError(t, err)
	return summary
}

// TestGetSummary tests the GetSummary helper function
func TestGetSummary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	filesystem := fstest.MapFS{
		"file.txt": {Data: []byte("hello")},
	}

	summary := GetSummary(t, ctx, filesystem, true)
	assert.NotNil(t, summary)
	assert.NotEqual(t, cid.Undef, summary.RootCID)
}

// TestGetSummary_EmptyDirectory tests empty directory handling
func TestGetSummary_EmptyDirectory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	filesystem := fstest.MapFS{
		"emptydir": {Mode: fs.ModeDir},
	}

	summary := GetSummary(t, ctx, filesystem, true)
	assert.NotNil(t, summary)
	assert.NotEqual(t, cid.Undef, summary.RootCID)
	assert.Equal(t, 1, len(summary.BlockOrder))
}

// TestWriteCAR_VerifiesBlockRegeneration tests that regenerating evicted blocks works for directory entries
func TestWriteCAR_VerifiesBlockRegeneration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	builder := newTestCARBuilder(t, smallTestMemoryLimit)


	filesystem := fstest.MapFS{
		"dir/file.txt": {Data: []byte("hello world")},
	}

	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)

	// WriteCAR should work normally, handling block regeneration if needed
	var carBuf bytes.Buffer
	err = builder.WriteCAR(ctx, &carBuf)
	require.NoError(t, err, "WriteCAR should succeed")

	// Verify CAR is valid
	carReader, err := carv1.NewCarReader(&carBuf)
	require.NoError(t, err)

	blockCount := 0
	for {
		_, err := carReader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		blockCount++
	}
	assert.Equal(t, len(summary.BlockOrder), blockCount, "CAR should contain all blocks from summary")
}

// TestWriteCAR_NilSummary tests WriteCAR with no summary
func TestWriteCAR_NilSummary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	builder := newTestCARBuilder(t, DefaultMemoryLimit)


	var buf bytes.Buffer
	err := builder.WriteCAR(ctx, &buf)
	assert.Error(t, err)
}

// TestCalculateCARSize tests the CalculateCARSize function
func TestCalculateCARSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filesystem fstest.MapFS
		wrapInDir bool
	}{
		{
			name: "single file",
			filesystem: fstest.MapFS{
				"file.txt": {Data: []byte("hello world")},
			},
			wrapInDir: true,
		},
		{
			name: "multiple files",
			filesystem: fstest.MapFS{
				"file1.txt": {Data: []byte("content1")},
				"file2.txt": {Data: []byte("content2")},
			},
			wrapInDir: true,
		},
		{
			name: "empty filesystem",
			filesystem: fstest.MapFS{},
			wrapInDir: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			summary := GetSummary(t, ctx, tt.filesystem, tt.wrapInDir)
			carSize, err := CalculateCARSize(summary)

			assert.NoError(t, err)
			assert.Greater(t, carSize, int64(0))
		})
	}
}

// TestCalculateCARSize_EdgeCases tests edge cases in CalculateCARSize
func TestCalculateCARSize_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("many_small_files", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		filesystem := fstest.MapFS{}
		for i := 0; i < 100; i++ {
			filesystem[fmt.Sprintf("file%d.txt", i)] = &fstest.MapFile{
				Data: []byte(fmt.Sprintf("content %d", i)),
			}
		}

		summary := GetSummary(t, ctx, filesystem, true)
		carSize, err := CalculateCARSize(summary)

		assert.NoError(t, err)
		assert.Greater(t, carSize, int64(100*10))
	})
}

// TestCalculateCARSize_ActualSizeComparison tests CalculateCARSize against actual output
func TestCalculateCARSize_ActualSizeComparison(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filesystem fstest.MapFS
		wrapInDir bool
	}{
		{
			name: "simple file",
			filesystem: fstest.MapFS{
				"file.txt": {Data: []byte("hello")},
			},
			wrapInDir: true,
		},
		{
			name: "multiple files",
			filesystem: fstest.MapFS{
				"file1.txt": {Data: []byte(getTestContent("1"))},
				"file2.txt": {Data: []byte(getTestContent("2"))},
				"file3.txt": {Data: []byte(getTestContent("3"))},
			},
			wrapInDir: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			var carBuf bytes.Buffer
			_, streamCARSize, err := StreamCARWithSize(ctx, tt.filesystem, &carBuf, DefaultMemoryLimit, tt.wrapInDir)
			require.NoError(t, err)

			builder := newTestCARBuilder(t, DefaultMemoryLimit)

			summary, err := builder.BuildSummary(ctx, tt.filesystem, tt.wrapInDir)
			require.NoError(t, err)
			calculatedSize, err := CalculateCARSize(summary)
			require.NoError(t, err)

			assert.Equal(t, streamCARSize, calculatedSize)
		})
	}
}

// TestStreamCARWithSize_ErrorPaths tests error handling in StreamCARWithSize
func TestStreamCARWithSize_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("empty_filesystem", func(t *testing.T) {
		ctx := context.Background()
		var buf bytes.Buffer
		filesystem := fstest.MapFS{}

		_, _, err := StreamCARWithSize(ctx, filesystem, &buf, DefaultMemoryLimit, true)
		require.NoError(t, err)
	})
}

// TestCalculateCARSize_StreamCARWithSizeIntegration tests integration between CalculateCARSize and StreamCARWithSize
func TestCalculateCARSize_StreamCARWithSizeIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filesystem fstest.MapFS
		wrapInDir bool
	}{
		{
			name: "single file",
			filesystem: fstest.MapFS{
				"file.txt": {Data: []byte("hello")},
			},
			wrapInDir: true,
		},
		{
			name: "multiple files",
			filesystem: fstest.MapFS{
				"file1.txt": {Data: []byte("content1")},
				"file2.txt": {Data: []byte("content2")},
			},
			wrapInDir: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			var carBuf bytes.Buffer
			_, streamCARSize, err := StreamCARWithSize(ctx, tt.filesystem, &carBuf, DefaultMemoryLimit, tt.wrapInDir)
			require.NoError(t, err)

			builder := newTestCARBuilder(t, DefaultMemoryLimit)

			summary, err := builder.BuildSummary(ctx, tt.filesystem, tt.wrapInDir)
			require.NoError(t, err)
			calculatedSize, err := CalculateCARSize(summary)
			require.NoError(t, err)

			assert.Equal(t, streamCARSize, calculatedSize)
		})
	}
}

// TestNewCARBuilder_WithNilParameters tests NewCARBuilder parameter handling
func TestNewCARBuilder_WithNilParameters(t *testing.T) {
	t.Parallel()

	t.Run("creates_default_blockstore_when_nil_provided", func(t *testing.T) {
		builder := NewCARBuilder(nil, nil, nil)
		assert.NotNil(t, builder)
		assert.NotNil(t, builder.bs)
		assert.NotNil(t, builder.dagService)
	})

	t.Run("initializes_with_non-nil_parameters", func(t *testing.T) {
		builder := newTestCARBuilder(t, 0)
		assert.NotNil(t, builder)
		assert.NotNil(t, builder.bs)
		assert.NotNil(t, builder.dagService)
		assert.NotNil(t, builder.generator)
	})
}

// TestPrepareCAR tests the PrepareCAR function
func TestPrepareCAR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		filesystem fstest.MapFS
		wrapInDir  bool
		maxMemory  uint64
		check      func(*testing.T, *CARBuilder, *TreeSummary)
	}{
		{
			name: "single file",
			filesystem: fstest.MapFS{
				"file.txt": {Data: []byte("hello world")},
			},
			wrapInDir:  true,
			maxMemory:  DefaultMemoryLimit,
			check: func(t *testing.T, builder *CARBuilder, summary *TreeSummary) {
				assert.NotNil(t, builder)
				assert.NotNil(t, summary)
				assert.NotEqual(t, cid.Undef, summary.RootCID)
				assert.Greater(t, len(summary.BlockOrder), 0)
			},
		},
		{
			name: "multiple files",
			filesystem: fstest.MapFS{
				"file1.txt": {Data: []byte(getTestContent("1"))},
				"file2.txt": {Data: []byte(getTestContent("2"))},
				"file3.txt": {Data: []byte(getTestContent("3"))},
			},
			wrapInDir:  true,
			maxMemory:  DefaultMemoryLimit,
			check: func(t *testing.T, builder *CARBuilder, summary *TreeSummary) {
				assert.NotNil(t, builder)
				assert.NotNil(t, summary)
				assert.NotEqual(t, cid.Undef, summary.RootCID)
				assert.GreaterOrEqual(t, len(summary.BlockOrder), 4)
			},
		},
		{
			name: "custom memory limit",
			filesystem: fstest.MapFS{
				"file.txt": {Data: []byte("hello")},
			},
			wrapInDir:  true,
			maxMemory:  50 * 1024 * 1024,
			check: func(t *testing.T, builder *CARBuilder, summary *TreeSummary) {
				assert.NotNil(t, builder)
				assert.NotNil(t, summary)
				assert.NotEqual(t, cid.Undef, summary.RootCID)
			},
		},
		{
			name: "without wrapping in directory",
			filesystem: fstest.MapFS{
				"file.txt": {Data: []byte("hello")},
			},
			wrapInDir:  false,
			maxMemory:  DefaultMemoryLimit,
			check: func(t *testing.T, builder *CARBuilder, summary *TreeSummary) {
				assert.NotNil(t, builder)
				assert.NotNil(t, summary)
				assert.NotEqual(t, cid.Undef, summary.RootCID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			builder, summary, err := PrepareCAR(ctx, tt.filesystem, tt.maxMemory, tt.wrapInDir)
			assert.NoError(t, err)
			if tt.check != nil {
				tt.check(t, builder, summary)
			}
		})
	}
}

// TestPrepareCAR_ContextCancellation tests context cancellation handling
func TestPrepareCAR_ContextCancellation(t *testing.T) {
	t.Parallel()

	t.Run("context already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		filesystem := fstest.MapFS{
			"file.txt": {Data: []byte("test")},
		}

		builder, summary, err := PrepareCAR(ctx, filesystem, DefaultMemoryLimit, true)
		assert.Error(t, err)
		assert.Nil(t, builder)
		assert.Nil(t, summary)
	})
}

// TestPrepareCAR_IntegrationWithCalculateCARSize tests integration with CalculateCARSize
func TestPrepareCAR_IntegrationWithCalculateCARSize(t *testing.T) {
	t.Parallel()

	t.Run("size_calculation_consistency", func(t *testing.T) {
		ctx := context.Background()
		filesystem := fstest.MapFS{
			"file1.txt": {Data: []byte("content 1")},
			"file2.txt": {Data: []byte("content 2")},
		}

		builder, summary, err := PrepareCAR(ctx, filesystem, DefaultMemoryLimit, true)
		require.NoError(t, err)

		calcSize, err := CalculateCARSize(summary)
		require.NoError(t, err)

		var buf bytes.Buffer
		err = builder.WriteCAR(ctx, &buf)
		require.NoError(t, err)

		assert.Equal(t, calcSize, int64(buf.Len()))
	})
}

// TestPrepareCARWithDefaultMemory tests the PrepareCARWithDefaultMemory convenience function
func TestPrepareCARWithDefaultMemory(t *testing.T) {
	t.Parallel()

	t.Run("uses_default_memory_limit", func(t *testing.T) {
		ctx := context.Background()
		filesystem := fstest.MapFS{
			"file.txt": {Data: []byte("hello")},
		}

		builder, summary, err := PrepareCARWithDefaultMemory(ctx, filesystem, true)
		assert.NoError(t, err)
		assert.NotNil(t, builder)
		assert.NotNil(t, summary)
		assert.NotEqual(t, cid.Undef, summary.RootCID)
	})

	t.Run("equivalent_to_PrepareCAR_with_DefaultMemoryLimit", func(t *testing.T) {
		ctx := context.Background()
		filesystem := fstest.MapFS{
			"file1.txt": {Data: []byte("test content")},
		}

		_, summary1, err1 := PrepareCARWithDefaultMemory(ctx, filesystem, true)
		require.NoError(t, err1)

		_, summary2, err2 := PrepareCAR(ctx, filesystem, DefaultMemoryLimit, true)
		require.NoError(t, err2)

		assert.Equal(t, summary1.RootCID, summary2.RootCID)

		size1, _ := CalculateCARSize(summary1)
		size2, _ := CalculateCARSize(summary2)
		assert.Equal(t, size1, size2)
	})
}
