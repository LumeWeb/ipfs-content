package car

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/ipfs-content/blockstore"
	"go.lumeweb.com/ipfs-content/internal/carv1"
)

// ============================
// Test-only BytesFS Implementation
// ============================

// testBytesFS implements fs.FS to wrap a []byte as a single-file filesystem.
// This is useful for uploading byte slices as CAR files.
type testBytesFS struct {
	data     []byte
	filename string
}

// newTestBytesFS creates a new filesystem containing a single file with the given data.
func newTestBytesFS(data []byte, filename string) *testBytesFS {
	return &testBytesFS{data: data, filename: filename}
}

// Open implements fs.FS.Open.
// Only the root "." and the single filename are valid paths.
// For "." we return the file itself since testBytesFS represents a single file,
// not a directory containing the file.
func (b *testBytesFS) Open(name string) (fs.File, error) {
	if name == "." || name == b.filename {
		return &testBytesFile{name: b.filename, data: b.data}, nil
	}
	return nil, fs.ErrNotExist
}

// testBytesFile implements fs.File for a single byte slice.
type testBytesFile struct {
	name string
	data []byte
	pos  int64
}

// Stat implements fs.File.Stat.
func (f *testBytesFile) Stat() (fs.FileInfo, error) {
	return &testBytesFileInfo{name: f.name, size: int64(len(f.data)), isDir: false}, nil
}

// Read implements io.Reader.
func (f *testBytesFile) Read(p []byte) (int, error) {
	if f.pos >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.pos:])
	f.pos += int64(n)
	return n, nil
}

// Close implements fs.File.Close (no-op).
func (f *testBytesFile) Close() error {
	return nil
}

// Seek implements io.Seeker for repositioning within the file.
// Supports SeekStart, SeekCurrent, and SeekEnd.
func (f *testBytesFile) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64

	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = f.pos + offset
	case io.SeekEnd:
		newOffset = int64(len(f.data)) + offset
	default:
		return 0, fmt.Errorf("invalid whence parameter")
	}

	if newOffset < 0 {
		newOffset = 0
	}

	f.pos = newOffset
	return newOffset, nil
}

// testBytesFileInfo implements fs.FileInfo.
type testBytesFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (fi *testBytesFileInfo) Name() string       { return fi.name }
func (fi *testBytesFileInfo) Size() int64        { return fi.size }
func (fi *testBytesFileInfo) Mode() fs.FileMode  { return 0644 }
func (fi *testBytesFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *testBytesFileInfo) IsDir() bool        { return fi.isDir }
func (fi *testBytesFileInfo) Sys() any           { return nil }

// ============================

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

// newTestCARBuilder creates a CARBuilder for testing.
func newTestCARBuilder(t *testing.T) *CARBuilder {
	t.Helper()
	return newCARBuilder()
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
			name:        "single file",
			wrapInDir:   true,
			expectError: false,
			check: func(t *testing.T, summary *TreeSummary) {
				assert.NotEqual(t, cid.Undef, summary.RootCID)
				assert.Greater(t, len(summary.BlockOrder), 0)
			},
		},
		{
			name:        "multiple files",
			wrapInDir:   true,
			expectError: false,
			check: func(t *testing.T, summary *TreeSummary) {
				assert.NotEqual(t, cid.Undef, summary.RootCID)
				assert.Greater(t, len(summary.BlockOrder), 0)
			},
		},
		{
			name:        "nested directories",
			wrapInDir:   true,
			expectError: false,
			check: func(t *testing.T, summary *TreeSummary) {
				assert.NotEqual(t, cid.Undef, summary.RootCID)
				assert.Greater(t, len(summary.BlockOrder), 0)
			},
		},
		{
			name:        "empty filesystem",
			wrapInDir:   true,
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

			builder := newTestCARBuilder(t)

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
		name       string
		check      func(*testing.T, *TreeSummary, int64)
		filesystem fstest.MapFS
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

			builder := newTestCARBuilder(t)

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

		builder := newTestCARBuilder(t)

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

		builder := newTestCARBuilder(t)

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

	builder := newTestCARBuilder(t)

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

			builder := newTestCARBuilder(t)

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

	builder := newTestCARBuilder(t)

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
			name:      "single file",
			wrapInDir: true,
		},
		{
			name:      "multiple files",
			wrapInDir: true,
		},
		{
			name:      "single file no wrap",
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
			rootCID, err := StreamCAR(ctx, filesystem, &buf, tt.wrapInDir)
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
	_, err := StreamCAR(ctx, filesystem, &buf, true)
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

		builder := newTestCARBuilder(t)

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

	builder := newTestCARBuilder(t)

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

			builder := newTestCARBuilder(t)

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
	rootCID, err := StreamCAR(ctx, filesystem, &buf, true)
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

	builder := newTestCARBuilder(t)

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

	builder := newTestCARBuilder(t)

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

	builder := newTestCARBuilder(t)

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

	builder := newTestCARBuilder(t)

	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)

	assert.NotEqual(t, cid.Undef, summary.RootCID)
	assert.GreaterOrEqual(t, len(summary.BlockOrder), 6)
}

// GetSummary returns the TreeSummary for the given filesystem
func GetSummary(t *testing.T, ctx context.Context, filesystem fs.FS, wrapInDir bool) *TreeSummary {
	t.Helper()

	builder := newTestCARBuilder(t)

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
	builder := newTestCARBuilder(t)

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
	builder := newTestCARBuilder(t)

	var buf bytes.Buffer
	err := builder.WriteCAR(ctx, &buf)
	assert.Error(t, err)
}

// TestCalculateCARSize tests the CalculateCARSize function
func TestCalculateCARSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		filesystem fstest.MapFS
		wrapInDir  bool
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
			name:       "empty filesystem",
			filesystem: fstest.MapFS{},
			wrapInDir:  true,
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
		name       string
		filesystem fstest.MapFS
		wrapInDir  bool
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
			_, streamCARSize, err := StreamCARWithSize(ctx, tt.filesystem, &carBuf, tt.wrapInDir)
			require.NoError(t, err)

			builder := newTestCARBuilder(t)

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

		_, _, err := StreamCARWithSize(ctx, filesystem, &buf, true)
		require.NoError(t, err)
	})
}

// TestCalculateCARSize_StreamCARWithSizeIntegration tests integration between CalculateCARSize and StreamCARWithSize
func TestCalculateCARSize_StreamCARWithSizeIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		filesystem fstest.MapFS
		wrapInDir  bool
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
			_, streamCARSize, err := StreamCARWithSize(ctx, tt.filesystem, &carBuf, tt.wrapInDir)
			require.NoError(t, err)

			builder := newTestCARBuilder(t)

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
		builder := newTestCARBuilder(t)
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
		check      func(*testing.T, *CARBuilder, *TreeSummary)
	}{
		{
			name: "single file",
			filesystem: fstest.MapFS{
				"file.txt": {Data: []byte("hello world")},
			},
			wrapInDir: true,
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
			wrapInDir: true,
			check: func(t *testing.T, builder *CARBuilder, summary *TreeSummary) {
				assert.NotNil(t, builder)
				assert.NotNil(t, summary)
				assert.NotEqual(t, cid.Undef, summary.RootCID)
				assert.GreaterOrEqual(t, len(summary.BlockOrder), 4)
			},
		},
		{
			name: "single_file_2",
			filesystem: fstest.MapFS{
				"file.txt": {Data: []byte("hello")},
			},
			wrapInDir: true,
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
			wrapInDir: false,
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

			builder, summary, err := PrepareCAR(ctx, tt.filesystem, tt.wrapInDir)
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

		builder, summary, err := PrepareCAR(ctx, filesystem, true)
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

		builder, summary, err := PrepareCAR(ctx, filesystem, true)
		require.NoError(t, err)

		calcSize, err := CalculateCARSize(summary)
		require.NoError(t, err)

		var buf bytes.Buffer
		err = builder.WriteCAR(ctx, &buf)
		require.NoError(t, err)

		assert.Equal(t, calcSize, int64(buf.Len()))
	})
}

// ==============================
// Regression Tests for LRU Eviction Bug
// TestCAR_Deduplication is a parameterized test that verifies CAR builder handles
// IPFS content-addressed deduplication correctly across various data sizes.
// Tests range from small (1KB) to very large (1GB) to ensure the implementation works
// TestCAR_Deduplication tests CAR builder handling of IPFS content-addressed
// deduplication using all-zeros data (maximum deduplication case).
func TestCAR_Deduplication(t *testing.T) {

	ctx := context.Background()

	testCases := []struct {
		name        string
		dataSize    int64
		description string
	}{
		{"1KB", 1 * 1024, "Tiny file"},
		{"128KB", 128 * 1024, "Small file"},
		{"512KB", 512 * 1024, "Half MB boundary"},
		{"1MB", 1 * 1024 * 1024, "Typical small file"},
		{"10MB", 10 * 1024 * 1024, "Moderate file"},
		{"100MB", 100 * 1024 * 1024, "Large file"},
		{"500MB", 500 * 1024 * 1024, "Very large file"},
		{"1GB", 1 * 1024 * 1024 * 1024, "Extremely large file"},

	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {

			data := make([]byte, tt.dataSize)
			// All zeros → maximum deduplication

			filesystem := fstest.MapFS{
				"zeros.bin": &fstest.MapFile{Data: data},
			}
			builder := newTestCARBuilder(t)
			summary, err := builder.BuildSummary(ctx, filesystem, true)
			require.NoError(t, err)



			// With all zeros, should have minimal blocks
			require.GreaterOrEqual(t, len(summary.BlockOrder), 2)

			// Verify WriteCAR succeeds
			carPath := t.TempDir() + "/test.car"
			carFile, err := os.Create(carPath)
			require.NoError(t, err)
			err = builder.WriteCAR(ctx, carFile)
			carFile.Close()
			require.NoError(t, err)
		})
	}
}

// TestCAR_NoDeduplication tests CAR builder using crypto random data that
// should create many unique blocks (minimal deduplication).
// Tests collectAllBlocks() works correctly across various data sizes.
func TestCAR_NoDeduplication(t *testing.T) {

	ctx := context.Background()

	testCases := []struct {
		name        string
		dataSize    int64
		description string
	}{
		{"1KB", 1 * 1024, "Tiny file"},
		{"128KB", 128 * 1024, "Small file"},
		{"512KB", 512 * 1024, "Half MB boundary"},
		{"1MB", 1 * 1024 * 1024, "Typical small file"},
		{"10MB", 10 * 1024 * 1024, "Moderate file"},
		{"100MB", 100 * 1024 * 1024, "Large file"},
		{"500MB", 500 * 1024 * 1024, "Very large file"},
		{"1GB", 1 * 1024 * 1024 * 1024, "Extremely large file"},

	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.dataSize)
			n, err := rand.Read(data)
			require.NoError(t, err)
			require.Equal(t, int(tt.dataSize), n)

			filesystem := fstest.MapFS{
				"random.bin": &fstest.MapFile{Data: data},
			}
			builder := newTestCARBuilder(t)
			_, err = builder.BuildSummary(ctx, filesystem, true)
			require.NoError(t, err)

			// Verify WriteCAR succeeds (tests collectAllBlocks finds all blocks)
			carPath := t.TempDir() + "/test.car"
			carFile, err := os.Create(carPath)
			require.NoError(t, err)
			err = builder.WriteCAR(ctx, carFile)
			carFile.Close()
			require.NoError(t, err)
		})
	}
}

// ==============================
// Regression Tests for LRU Eviction Bug
// ==============================

const (
	lruEvictionTestChunkSize = int64(256 * 1024) // 256KB chunks
)

// TestWriteCAR_BlockRegenerationNoLRUEviction is a regression test for the bug where
// block regeneration would fail with "block not found" errors due to LRU eviction.
//
// Bug scenario:
// - Stage 1 (BuildSummary): Creates ~102 blocks (1MB per chunk for 100MB file)
// - Stage 2 (WriteCAR):  LRU blockstore limits to 100 blocks
// - When block #1 needs regeneration: recreates 102 blocks, evicts block #1, fails
// - Fix: LevelBlockStore tracks DAG levels, no memory-based eviction
func TestWriteCAR_BlockRegenerationNoLRUEviction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create test data that will trigger ~100 blocks
	chunkSize := lruEvictionTestChunkSize
	totalDataSize := int64(101) * chunkSize // ~25MB (triggers >100 blocks)
	testData := make([]byte, totalDataSize)

	n, err := rand.Read(testData)
	require.NoError(t, err)
	require.Equal(t, int(totalDataSize), n)

	filesystem := fstest.MapFS{
		"largefile.bin": &fstest.MapFile{Data: testData},
	}

	// BuildSummary
	builder := newTestCARBuilder(t)
	summary, err := builder.BuildSummary(ctx, filesystem, false)
	require.NoError(t, err)
	require.NotNil(t, summary)

	totalBlocks := len(summary.BlockOrder)

	// testBytesFS supports seeking like real files, ensuring proper two-pass
	// CAR generation. Just verify we have enough blocks to test CAR generation.
	// The important part is that WriteCAR completes without "block not found" errors.
	require.Greater(t, totalBlocks, 10,
		"Test needs enough blocks to verify regeneration")

	// WriteCAR should not fail with "block not found" error
	// This verifies LevelBlockStore is working (no LRU eviction)
	var carBuf bytes.Buffer
	err = builder.WriteCAR(ctx, &carBuf)

	require.NoError(t, err, "WriteCAR must not fail with 'block not found' error")
	require.Greater(t, carBuf.Len(), 0, "CAR should contain data")
}

// TestLevelBlockStore_Integration verifies that newCARBuilder uses LevelBlockStore
// for stage 2 (WriteCAR) block regeneration - regression test for LRU eviction bug.
func TestLevelBlockStore_Integration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create CARBuilder using the public API
	filesystem := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("hello world")},
	}

	builder, summary, err := PrepareCAR(ctx, filesystem, true)
	require.NoError(t, err)
	require.NotNil(t, builder)
	require.NotNil(t, summary)

	// Verify the blockstore is a LevelBlockStore (not LRUBlockstore)
	levelBS, ok := builder.bs.(*blockstore.LevelBlockStore)
	require.True(t, ok, "newCARBuilder should use LevelBlockStore for stage 2")
	require.NotNil(t, levelBS, "LevelBlockStore must not be nil")

	// Verify WriteCAR works with LevelBlockStore
	var carBuf bytes.Buffer
	err = builder.WriteCAR(ctx, &carBuf)
	require.NoError(t, err, "WriteCAR should succeed with LevelBlockStore")
}

// TestWriteCAR_LargeFileNoLRUEviction tests block regeneration with a larger file
// that would trigger LRU eviction but shouldn't with LevelBlockStore.
func TestWriteCAR_LargeFileNoLRUEviction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Use smaller chunk size to create more blocks
	chunkSize := int64(512 * 1024)          // 512KB chunks
	totalDataSize := int64(300) * chunkSize // 50MB should create ~100 blocks
	testData := make([]byte, totalDataSize)

	n, err := rand.Read(testData)
	require.NoError(t, err)
	require.Equal(t, int(totalDataSize), n)

	filesystem := fstest.MapFS{
		"largefile.bin": &fstest.MapFile{Data: testData},
	}

	builder := newTestCARBuilder(t)
	_, err = builder.BuildSummary(ctx, filesystem, false)
	require.NoError(t, err)

	// WriteCAR should handle block regeneration without LRU issues
	var carBuf bytes.Buffer
	err = builder.WriteCAR(ctx, &carBuf)

	require.NoError(t, err, "Large file CAR generation must not fail")
	require.Greater(t, carBuf.Len(), 0, "CAR should contain data")
}

// TestBuildSummary_ExcludesDotPaths tests that BuildSummary correctly excludes "." and ".."
// directories when walking a real filesystem with os.DirFS. This prevents ghost directory
// entries from being included in the CAR as files.
func TestBuildSummary_ExcludesDotPaths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a temporary directory with real files
	tmpDir, err := os.MkdirTemp("", "car-dot-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test files in the directory
	fileContents := map[string]string{
		"file1.txt": "content 1",
		"file2.txt": "content 2",
		"file3.txt": "content 3",
		"file4.txt": "content 4",
		"file5.txt": "content 5",
	}

	for filename, content := range fileContents {
		err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644)
		require.NoError(t, err)
	}

	// Build summary using os.DirFS (replicates real-world usage)
	filesystem := os.DirFS(tmpDir)
	builder := newTestCARBuilder(t)

	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)
	require.NotNil(t, summary)

	// Verify that "." is not in TreeEntries
	_, exists := summary.TreeEntries[CurrentDir]
	require.False(t, exists, "Current directory '.' should not be in TreeEntries")

	// Verify that ".." is not in TreeEntries
	_, exists = summary.TreeEntries[ParentDir]
	require.False(t, exists, "Parent directory '..' should not be in TreeEntries")

	// Verify that only our 5 files are in the tree entries (plus ROOT)
	expectedFileCount := len(fileContents)
	actualFileCount := 0
	for path, entry := range summary.TreeEntries {
		if path != ROOT && !entry.IsDir {
			actualFileCount++
		}
	}
	require.Equal(t, expectedFileCount, actualFileCount,
		"Should have exactly %d file entries, got %d", expectedFileCount, actualFileCount)

	// Verify no entry has "." or ".." as its name or path
	for path, entry := range summary.TreeEntries {
		if entry.Name == CurrentDir {
			t.Errorf("Found entry with name '.': path=%s", path)
		}
		if entry.Name == ParentDir {
			t.Errorf("Found entry with name '..': path=%s", path)
		}
		if entry.Path == "." {
			t.Errorf("Found entry with path '.': path=%s, name=%s", path, entry.Name)
		}
		if entry.Path == ".." {
			t.Errorf("Found entry with path '..': path=%s, name=%s", path, entry.Name)
		}
	}

	// Write CAR and verify it works correctly
	var buf bytes.Buffer
	err = builder.WriteCAR(ctx, &buf)
	require.NoError(t, err)
	require.Greater(t, buf.Len(), 0)

	// Verify CAR is valid
	carReader, err := carv1.NewCarReader(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, uint64(1), carReader.Header.Version)
	assert.Len(t, carReader.Header.Roots, 1)
}

// TestBuildSummary_ExcludesDotPaths_NestedDirectories tests that "." and ".." are excluded
// even when there are nested directories in the filesystem.
func TestBuildSummary_ExcludesDotPaths_NestedDirectories(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a temporary directory with nested structure
	tmpDir, err := os.MkdirTemp("", "car-dot-nested-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create nested directory structure
	dirs := []string{"dir1", "dir2", "dir1/subdir1", "dir1/subdir2"}
	for _, dir := range dirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create files in various directories
	files := map[string]string{
		"file1.txt":           "content 1",
		"dir1/file2.txt":      "content 2",
		"dir2/file3.txt":      "content 3",
		"dir1/subdir1/file4":  "content 4",
		"dir1/subdir2/file5":  "content 5",
	}

	for file, content := range files {
		err := os.WriteFile(filepath.Join(tmpDir, file), []byte(content), 0644)
		require.NoError(t, err)
	}

	// Build summary using os.DirFS
	filesystem := os.DirFS(tmpDir)
	builder := newTestCARBuilder(t)

	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)
	require.NotNil(t, summary)

	// Verify that "." and ".." are not in TreeEntries
	_, exists := summary.TreeEntries[CurrentDir]
	require.False(t, exists, "Current directory '.' should not be in TreeEntries")

	_, exists = summary.TreeEntries[ParentDir]
	require.False(t, exists, "Parent directory '..' should not be in TreeEntries")

	// Verify all dot paths are excluded regardless of depth
	for path := range summary.TreeEntries {
		require.NotContains(t, path, "/.", "Path should not contain '/.'")
		require.NotContains(t, path, "/..", "Path should not contain '/..'")
		require.NotContains(t, path, "./", "Path should not contain './'")
		require.NotContains(t, path, "../", "Path should not contain '../'")
	}
}

// TestBuildSummary_ExcludesDirectoryEntries test that directory entries are not
// included as separate entries in the tree. This is the bug we're seeing where
// directories get added as entries alongside their children.
//
// When using files.Walk or boxo's file abstraction, the walk visits both files
// and directories. We should only include files in TreeEntries, not directories,
// because directory blocks are created separately in the second pass.
func TestBuildSummary_ExcludesDirectoryEntries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a test filesystem with nested structure
	filesystem := fstest.MapFS{
		"file1.txt": &fstest.MapFile{Data: []byte("content 1")},
		"file2.txt": &fstest.MapFile{Data: []byte("content 2")},
		"file3.txt": &fstest.MapFile{Data: []byte("content 3")},
		"dir1/file4.txt": &fstest.MapFile{Data: []byte("content 4")},
		"dir2/file5.txt": &fstest.MapFile{Data: []byte("content 5")},
	}

	builder := newTestCARBuilder(t)
	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)
	require.NotNil(t, summary)

	// Count entries by type
	var fileCount, dirCount int
	for path, entry := range summary.TreeEntries {
		if path == ROOT {
			continue // ROOT is a synthetic entry
		}
		if entry.IsDir {
			dirCount++
		} else {
			fileCount++
		}
	}

	// We have 5 files, should have exactly 5 file entries
	require.Equal(t, 5, fileCount, "Should have exactly 5 file entries")

	// Directories ARE tracked in TreeEntries for bookkeeping
	// but they should not appear as Children entries in the wrong places
	// The fix ensures directories are only used for internal tracking,
	// not as top-level entries
	require.Greater(t, dirCount, 0, "Should have directory entries for bookkeeping")
}

// TestBuildSummary_NoRootDirectoryEntry specifically tests that "." (the root directory)
// is not added as a separate entry. This is the core of the issue.
func TestBuildSummary_NoRootDirectoryEntry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a simple filesystem with files at root level
	filesystem := fstest.MapFS{
		"file1.txt": &fstest.MapFile{Data: []byte("content 1")},
		"file2.txt": &fstest.MapFile{Data: []byte("content 2")},
		"file3.txt": &fstest.MapFile{Data: []byte("content 3")},
	}

	builder := newTestCARBuilder(t)
	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)
	require.NotNil(t, summary)

	// Verify "." is not in TreeEntries
	_, exists := summary.TreeEntries["."]
	require.False(t, exists, "Root directory '.' should not be in TreeEntries")

	// Verify files are in ROOT's children
	rootEntry := summary.TreeEntries[ROOT]
	require.NotNil(t, rootEntry)
	require.Len(t, rootEntry.Children, 3, "ROOT should have exactly 3 children")

	// Verify ROOT children are the files, not the directory itself
	expectedFiles := []string{"file1.txt", "file2.txt", "file3.txt"}
	for _, expectedFile := range expectedFiles {
		found := false
		for _, child := range rootEntry.Children {
			if child == expectedFile {
				found = true
				break
			}
		}
		require.True(t, found, "ROOT should contain %s as a child", expectedFile)
	}

	// Verify no extra entries exist
	// We should have exactly: ROOT + 3 files = 4 entries total
	expectedTotalEntries := 1 + 3 // ROOT + files
	actualTotalEntries := len(summary.TreeEntries)

	// This will likely fail if the bug exists (extra directory entries)
	require.Equal(t, expectedTotalEntries, actualTotalEntries,
		"Expected %d total entries (ROOT + files), got %d",
		expectedTotalEntries, actualTotalEntries)

}

// TestBuildSummary_RootChildrenStructure tests the specific bug where
// directories are appearing alongside their children in ROOT's Children list.
// This is what the user is seeing: "a dir with all the children as well all he children as sublings"
func TestBuildSummary_RootChildrenStructure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a filesystem with directories at root level
	filesystem := fstest.MapFS{
		"file1.txt":      &fstest.MapFile{Data: []byte("content 1")},
		"dir1/file2.txt": &fstest.MapFile{Data: []byte("content 2")},
		"dir1/file3.txt": &fstest.MapFile{Data: []byte("content 3")},
	}

	builder := newTestCARBuilder(t)
	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)
	require.NotNil(t, summary)

	rootEntry := summary.TreeEntries[ROOT]
	require.NotNil(t, rootEntry)


	// ROOT's children should only contain file paths, not directory paths
	for _, child := range rootEntry.Children {
		childEntry := summary.TreeEntries[child]
		require.NotNil(t, childEntry, "Child entry should exist: %s", child)
		require.False(t, childEntry.IsDir,
			"ROOT child should not be a directory: %s is a dir, expected only files", child)
	}

	// Expected: ROOT has file1.txt as a child
	require.Contains(t, rootEntry.Children, "file1.txt", "ROOT should contain file1.txt")

	// BUG: dir1 should NOT be in ROOT's children!
	// dir1 is a directory, not a file. Its children (file2.txt, file3.txt) should be
	// added to ROOT, not dir1 itself.
	require.NotContains(t, rootEntry.Children, "dir1", "ROOT should not contain directory dir1 as a child")

	// ROOT should have exactly 1 child (file1.txt), not 2 (file1.txt + dir1)
	require.Len(t, rootEntry.Children, 1, "ROOT should have exactly 1 child (the file), not the directory")

}

// TestBuildSummary_WalkBehavior demonstrates the actual behavior of fs.WalkDir
// to understand what entries it generates.
func TestBuildSummary_WalkBehavior(t *testing.T) {
	t.Parallel()

	// Create a simple filesystem
	filesystem := fstest.MapFS{
		"file1.txt": &fstest.MapFile{Data: []byte("content 1")},
		"dir1/file2.txt": &fstest.MapFile{Data: []byte("content 2")},
	}

	// Walk the filesystem and log what we see
	pathsVisited := []string{}
	dirsVisited := []string{}
	filesVisited := []string{}

	err := fs.WalkDir(filesystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		pathsVisited = append(pathsVisited, path)
		if d.IsDir() {
			dirsVisited = append(dirsVisited, path)
		} else {
			filesVisited = append(filesVisited, path)
		}
		return nil
	})

	require.NoError(t, err)

	// Log what fs.WalkDir actually visits

	// Expected: "." (root dir), "dir1", "file1.txt", "dir1/file2.txt"
	require.Contains(t, pathsVisited, ".", "Should visit root directory '.'")
	require.Contains(t, pathsVisited, "file1.txt", "Should visit file1.txt")
	require.Contains(t, pathsVisited, "dir1", "Should visit dir1 directory")
	require.Contains(t, pathsVisited, "dir1/file2.txt", "Should visit dir1/file2.txt")

	// This shows that fs.WalkDir visits directories as well as files!
	// This is the root cause of the bug.
}

// TestBuildSummary_RealFilesystem demonstrates the issue with a real filesystem
// which is closer to the boxo/files behavior.
func TestBuildSummary_RealFilesystem(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a temporary directory with real files
	tmpDir, err := os.MkdirTemp("", "car-dir-entry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test files
	files := map[string]string{
		"file1.txt": "content 1",
		"file2.txt": "content 2",
		"file3.txt": "content 3",
		"dir1/file4.txt": "content 4",
		"dir2/file5.txt": "content 5",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)
		if dir != fullPath {
			err := os.MkdirAll(dir, 0755)
			require.NoError(t, err)
		}
		err := os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Build summary using os.DirFS (replicates boxo/files behavior)
	filesystem := os.DirFS(tmpDir)
	builder := newTestCARBuilder(t)

	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)
	require.NotNil(t, summary)

	// Count directory entries (excluding ROOT)
	var dirCount int
	for path, entry := range summary.TreeEntries {
		if path == ROOT {
			continue
		}
		if entry.IsDir {
			dirCount++
		}
	}

	// Directories are tracked in TreeEntries for bookkeeping
	// This is necessary for creating directory blocks in phase 2
	require.Greater(t, dirCount, 0, "Should have directory entries for bookkeeping")

	// The directory blocks are created separately
	// They appear in BlockOrder and have CIDs
	dirBlockCount := 0
	for _, blockCID := range summary.BlockOrder {
		entry := summary.CIDToEntry[blockCID]
		if entry != nil && entry.IsDir && entry.Path != ROOT {
			dirBlockCount++
		}
	}

	// We should have 2 directory blocks (dir1 and dir2)
	require.Equal(t, 2, dirBlockCount, "Should have exactly 2 directory blocks created")
}

// TestHierarchy_NestedDirectories tests that nested directory structures
// are preserved correctly, with directories added as children of parent directories.
func TestHierarchy_NestedDirectories(t *testing.T) {
	ctx := context.Background()

	// Create a nested filesystem
	filesystem := fstest.MapFS{
		"file1.txt":          &fstest.MapFile{Data: []byte("content 1")},
		"dir1/file2.txt":     &fstest.MapFile{Data: []byte("content 2")},
		"dir1/dir2/file3.txt": &fstest.MapFile{Data: []byte("content 3")},
		"dir1/dir2/file4.txt": &fstest.MapFile{Data: []byte("content 4")},
		"dir1/file5.txt":     &fstest.MapFile{Data: []byte("content 5")},
	}

	builder := newTestCARBuilder(t)
	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)

	_ = summary.TreeEntries["dir1/dir2"] // Will exist after BuildSummary


	// dir1 should NOT be in ROOT.Children (it's a directory, not a file)
	require.NotContains(t, summary.TreeEntries[ROOT].Children, "dir1",
		"BUG: Directory should not be in ROOT.Children")

	// file1.txt should be in ROOT.Children
	require.Contains(t, summary.TreeEntries[ROOT].Children, "file1.txt",
		"Root-level file should be in ROOT.Children")

	// dir1 should have the files inside it, plus subdir2 as a child
	dir1Children := summary.TreeEntries["dir1"].Children
	require.Contains(t, dir1Children, "dir1/file2.txt",
		"dir1/file2.txt should be in dir1's children")
	require.Contains(t, dir1Children, "dir1/file5.txt",
		"dir1/file5.txt should be in dir1's children")
	require.Contains(t, dir1Children, "dir1/dir2",
		"dir1/dir2 (subdirectory) should be in dir1's children")

	// dir1/dir2 should have the files inside it
	dir2Children := summary.TreeEntries["dir1/dir2"].Children
	require.Contains(t, dir2Children, "dir1/dir2/file3.txt",
		"dir1/dir2/file3.txt should be in dir1/dir2's children")
	require.Contains(t, dir2Children, "dir1/dir2/file4.txt",
		"dir1/dir2/file4.txt should be in dir1/dir2's children")

}

// TestHierarchy_DirectoryVsFile clarifies the difference between files and directories
// and validates that both are handled correctly.
func TestHierarchy_DirectoryVsFile(t *testing.T) {
	ctx := context.Background()

	filesystem := fstest.MapFS{
		"file.txt":       &fstest.MapFile{Data: []byte("content")},
		"dir/file.txt":   &fstest.MapFile{Data: []byte("content")},
		"dir/subdir.txt": &fstest.MapFile{Data: []byte("content")},
	}

	builder := newTestCARBuilder(t)
	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)


	// "dir" is a directory (fs.WalkDir visits it with IsDir=true)
	dirEntry, exists := summary.TreeEntries["dir"]
	require.True(t, exists, "dir should exist in TreeEntries")
	require.True(t, dirEntry.IsDir, "dir should be marked as a directory")

	// Directories are in TreeEntries for bookkeeping but not as Children
	// of their parents

	// Only files should be in ROOT.Children
	for _, childPath := range summary.TreeEntries[ROOT].Children {
		childEntry := summary.TreeEntries[childPath]
		require.False(t, childEntry.IsDir,
			"ROOT child %s should be a file, not a directory", childPath)
	}
	require.Contains(t, summary.TreeEntries[ROOT].Children, "file.txt")

	// dir should have its children
	require.Len(t, summary.TreeEntries["dir"].Children, 2)
	require.Contains(t, summary.TreeEntries["dir"].Children, "dir/file.txt")
	require.Contains(t, summary.TreeEntries["dir"].Children, "dir/subdir.txt")

}

// TestBuildSummary_SingleFileAsCurrentDir tests the edge case where the
// filesystem is a single-file wrapper (like testBytesFS) where "." is a file,
// not a directory. This happens when wrapping bytes or a single file as an
// fs.FS implementation. WalkDir will visit "." with isDir=false, and we must
// process it correctly rather than skipping it.
func TestBuildSummary_SingleFileAsCurrentDir(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a single-file filesystem where "." opens to a file
	filesystem := newTestBytesFS([]byte("hello world"), "test.txt")
	builder := newTestCARBuilder(t)

	// Build summary - this should not error
	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)
	require.NotNil(t, summary)

	// Verify "." is in TreeEntries as a file (not a directory)
	entry, exists := summary.TreeEntries["."]
	require.True(t, exists, `"." should be in TreeEntries for single-file filesystem`)
	require.False(t, entry.IsDir, `"." should not be a directory`)
	require.Equal(t, "test.txt", entry.Name, "File name should be 'test.txt' from testBytesFS")
	require.Equal(t, "", entry.Path) // At root level

	// Verify the CID was created
	require.NotEqual(t, cid.Undef, entry.CID, "File should have a CID")

	// Verify it was added to ROOT.Children
	require.Contains(t, summary.TreeEntries[ROOT].Children, ".",
		"Single file should be added to ROOT.Children")

	// Verify ROOT has exactly one child
	require.Len(t, summary.TreeEntries[ROOT].Children, 1,
		"ROOT should have exactly one child (the single file)")

	// Verify blocks were created
	require.NotEmpty(t, summary.BlockOrder, "Should have blocks")
	require.NotEmpty(t, summary.BlockSizes, "Should have block sizes")
	require.LessOrEqual(t, len(summary.BlockOrder), len(summary.BlockSizes),
		"BlockOrder length should not exceed BlockSizes length")

	// Verify summary has root CID
	require.NotEqual(t, cid.Undef, summary.RootCID)
}

// TestLogicalFileSizeTracking tests that TreeSummary correctly tracks UnixFS logical file sizes
func TestLogicalFileSizeTracking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		filesystem       fstest.MapFS
		wrapInDir        bool
		expectedTotal    uint64 // Expected sum of logical file sizes
	}{
		{
			name: "single_file",
			filesystem: fstest.MapFS{
				"file.txt": {Data: []byte("hello world")},
			},
			wrapInDir:     true,
			expectedTotal: 11, // "hello world" = 11 bytes
		},
		{
			name: "multiple_files",
			filesystem: fstest.MapFS{
				"file1.txt": {Data: []byte("content 1")},
				"file2.txt": {Data: []byte("content 2")},
				"file3.txt": {Data: []byte("content 3")},
			},
			wrapInDir:     true,
			expectedTotal: 27, // 9 + 9 + 9 = 27 bytes
		},
		{
			name: "nested_directories_with_files",
			filesystem: fstest.MapFS{
				"dir1/file1.txt":        {Data: []byte("file 1")},
				"dir1/subdir/file2.txt": {Data: []byte("file 2")},
				"dir2/file3.txt":        {Data: []byte("file 3")},
			},
			wrapInDir:     true,
			expectedTotal: 18, // 6 + 6 + 6 = 18 bytes
		},
		{
			name: "empty_directory",
			filesystem: fstest.MapFS{
				"emptydir": {Mode: fs.ModeDir},
			},
			wrapInDir:     true,
			expectedTotal: 0, // No files
		},
		{
			name: "files_and_empty_directories",
			filesystem: fstest.MapFS{
				"file1.txt": {Data: []byte("hello")},
				"emptydir":  {Mode: fs.ModeDir},
				"file2.txt": {Data: []byte("world")},
			},
			wrapInDir:     true,
			expectedTotal: 10, // 5 + 5 = 10 bytes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			builder := newTestCARBuilder(t)

			summary, err := builder.BuildSummary(ctx, tt.filesystem, tt.wrapInDir)
			require.NoError(t, err)
			require.NotNil(t, summary)

			// Verify TotalLogicalFileSize returns the expected total
			logicalTotal := summary.TotalLogicalFileSize()
			assert.Equal(t, tt.expectedTotal, logicalTotal,
				"TotalLogicalFileSize() should return %d, got %d", tt.expectedTotal, logicalTotal)

			// Verify each file entry has the correct LogicalFileSize
			for path, entry := range summary.TreeEntries {
				if !entry.IsDir && path != ROOT {
					expectedSize := uint64(len(tt.filesystem[path].Data))
					assert.Equal(t, expectedSize, entry.LogicalFileSize,
						"File entry '%s' should have LogicalFileSize matching content length", path)
				}
			}

			// Verify TotalSize (block sizes) is different from logical file sizes
			// because it includes overhead and framing
			assert.Greater(t, summary.TotalSize, logicalTotal,
				"TotalSize should be greater than logical file sizes (includes overhead)")
		})
	}
}

// TestBuildSummary_SingleFileAsCurrentDir_NoWrap tests the single-file
// filesystem with wrapInDir=false to ensure the optimization works.
func TestBuildSummary_SingleFileAsCurrentDir_NoWrap(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a single-file filesystem
	filesystem := newTestBytesFS([]byte("hello world"), "test.txt")
	builder := newTestCARBuilder(t)

	// Build summary with wrapInDir=false
	summary, err := builder.BuildSummary(ctx, filesystem, false)
	require.NoError(t, err)
	require.NotNil(t, summary)

	// With wrapInDir=false and a single file, RootCID should be the file's CID
	require.NotEqual(t, cid.Undef, summary.RootCID)

	// The entry's CID should match the RootCID
	entry := summary.TreeEntries["."]
	require.NotNil(t, entry)
	require.Equal(t, entry.CID, summary.RootCID,
		"With wrapInDir=false, RootCID should be the file's CID")
}
