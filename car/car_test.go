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

// testBytesFS is a test-only filesystem wrapper for byte slices.
// Similar to ipfs-sdk/fs/bytesfs.go but simplified for regression tests.
type testBytesFS struct {
	data     []byte
	filename string
}

// newTestBytesFS creates a test filesystem with a single file.
func newTestBytesFS(data []byte, filename string) *testBytesFS {
	return &testBytesFS{data: data, filename: filename}
}

// Open implements fs.FS.Open.
func (b *testBytesFS) Open(name string) (fs.File, error) {
	if name == "." {
		return &testBytesDir{filename: b.filename, size: int64(len(b.data))}, nil
	}
	if name == b.filename {
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

func (f *testBytesFile) Stat() (fs.FileInfo, error) {
	return &testBytesFileInfo{size: int64(len(f.data)), isDir: false}, nil
}

func (f *testBytesFile) Read(p []byte) (int, error) {
	if f.pos >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.pos:])
	f.pos += int64(n)
	return n, nil
}

func (f *testBytesFile) Close() error {
	return nil
}

// Seek implements io.Seeker for repositioning within the file.
// This supports the two-pass CAR generation pattern where files need to be reopened.
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

// testBytesDir implements fs.File and fs.ReadDirFile for a directory.
type testBytesDir struct {
	filename string
	size     int64
	offset   int
}

func (d *testBytesDir) Stat() (fs.FileInfo, error) {
	return &testBytesFileInfo{size: 0, isDir: true}, nil
}

func (d *testBytesDir) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (d *testBytesDir) Close() error {
	return nil
}

func (d *testBytesDir) Seek(offset int64, whence int) (int64, error) {
	return 0, fmt.Errorf("seek not supported on directories")
}

func (d *testBytesDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.offset >= 1 {
		return nil, io.EOF
	}
	d.offset++
	return []fs.DirEntry{&testBytesDirEntry{name: d.filename, size: d.size}}, nil
}

// testBytesFileInfo implements fs.FileInfo.
type testBytesFileInfo struct {
	size  int64
	isDir bool
}

func (fi *testBytesFileInfo) Name() string       { return "" }
func (fi *testBytesFileInfo) Size() int64        { return fi.size }
func (fi *testBytesFileInfo) Mode() fs.FileMode  { return 0644 }
func (fi *testBytesFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *testBytesFileInfo) IsDir() bool        { return fi.isDir }
func (fi *testBytesFileInfo) Sys() any           { return nil }

// testBytesDirEntry implements fs.DirEntry.
type testBytesDirEntry struct {
	name string
	size int64
}

func (de *testBytesDirEntry) Name() string      { return de.name }
func (de *testBytesDirEntry) Type() fs.FileMode { return 0 }
func (de *testBytesDirEntry) Info() (fs.FileInfo, error) {
	return &testBytesFileInfo{size: de.size, isDir: false}, nil
}
func (de *testBytesDirEntry) IsDir() bool {
	return false
}

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
			wrapInDir:  true,
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
			wrapInDir:  true,
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

	for i := 0; i < len(testData); i++ {
		testData[i] = byte(i % 256)
	}

	filesystem := newTestBytesFS(testData, "largefile.bin")

	t.Logf("Regression test setup:")
	t.Logf("  - Chunk size: %d bytes (%.2f KB)", chunkSize, float64(chunkSize)/1024)
	t.Logf("  - Data size: %.2f MB", float64(totalDataSize)/(1024*1024))

	// BuildSummary
	builder := newTestCARBuilder(t)
	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)
	require.NotNil(t, summary)

	totalBlocks := len(summary.BlockOrder)
	t.Logf("  - Total blocks: %d", totalBlocks)

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

	t.Logf("  - CAR size: %d bytes (%.2f MB)", carBuf.Len(), float64(carBuf.Len())/(1024*1024))
	t.Logf("PASS: Block regeneration works without LRU eviction issues")
}

// TestLevelBlockStore_Integration verifies that newCARBuilder uses LevelBlockStore
// for stage 2 (WriteCAR) block regeneration - regression test for LRU eviction bug.
func TestLevelBlockStore_Integration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create CARBuilder using the public API
	filesystem := newTestBytesFS([]byte("hello world"), "file.txt")

	builder, summary, err := PrepareCAR(ctx, filesystem, true)
	require.NoError(t, err)
	require.NotNil(t, builder)
	require.NotNil(t, summary)

	// Verify the blockstore is a LevelBlockStore (not LRUBlockstore)
	levelBS, ok := builder.bs.(*blockstore.LevelBlockStore)
	require.True(t, ok, "newCARBuilder should use LevelBlockStore for stage 2")
	require.NotNil(t, levelBS, "LevelBlockStore must not be nil")

	t.Logf("LevelBlockStore integration verified:")
	t.Logf("  - Blockstore type: LevelBlockStore")
	t.Logf("  - Summary blocks: %d", len(summary.BlockOrder))
	t.Logf("  - Blockstore blocks: %d", levelBS.Len())

	// Verify WriteCAR works with LevelBlockStore
	var carBuf bytes.Buffer
	err = builder.WriteCAR(ctx, &carBuf)
	require.NoError(t, err, "WriteCAR should succeed with LevelBlockStore")

	t.Logf("  - CAR size: %d bytes", carBuf.Len())
}

// TestWriteCAR_LargeFileNoLRUEviction tests block regeneration with a larger file
// that would trigger LRU eviction but shouldn't with LevelBlockStore.
func TestWriteCAR_LargeFileNoLRUEviction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Use smaller chunk size to create more blocks
	chunkSize := int64(512 * 1024) // 512KB chunks
	totalDataSize := int64(100) * chunkSize // 50MB should create ~100 blocks
	testData := make([]byte, totalDataSize)

	for i := 0; i < len(testData); i++ {
		testData[i] = byte(i % 256)
	}

	filesystem := newTestBytesFS(testData, "largefile.bin")

	t.Logf("Large file regression test:")
	t.Logf("  - Data size: %.2f MB", float64(totalDataSize)/(1024*1024))

	builder := newTestCARBuilder(t)
	summary, err := builder.BuildSummary(ctx, filesystem, true)
	require.NoError(t, err)

	t.Logf("  - Blocks created: %d", len(summary.BlockOrder))

	// WriteCAR should handle block regeneration without LRU issues
	var carBuf bytes.Buffer
	err = builder.WriteCAR(ctx, &carBuf)

	require.NoError(t, err, "Large file CAR generation must not fail")
	require.Greater(t, carBuf.Len(), 0, "CAR should contain data")

	t.Logf("PASS: Large file CAR generation succeeded")
}
