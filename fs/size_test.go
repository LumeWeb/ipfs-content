package fs

import (
	"context"
	"os"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDAGSizeFromFS(t *testing.T) {
	ctx := context.Background()

	t.Run("single file", func(t *testing.T) {
		testFS := fstest.MapFS{
			"test.txt": &fstest.MapFile{Data: []byte("Hello, World!")},
		}

		size, err := GetDAGSizeFromFS(ctx, testFS, false)
		require.NoError(t, err)
		assert.Greater(t, size, uint64(0), "DAG size should be positive")

		// DAG size should be larger than raw file size due to UnixFS overhead
		assert.Greater(t, size, uint64(13), "DAG size includes overhead beyond raw data")
	})

	t.Run("wrap in directory", func(t *testing.T) {
		testFS := fstest.MapFS{
			"test.txt": &fstest.MapFile{Data: []byte("Hello!")},
		}

		size, err := GetDAGSizeFromFS(ctx, testFS, true)
		require.NoError(t, err)
		assert.Greater(t, size, uint64(0), "DAG size should be positive")

		// Wrapped in directory should be larger than not wrapped
		sizeNotWrapped, err := GetDAGSizeFromFS(ctx, testFS, false)
		require.NoError(t, err)
		assert.Greater(t, size, sizeNotWrapped, "Directory wrapping adds overhead")
	})

	t.Run("empty filesystem", func(t *testing.T) {
		testFS := fstest.MapFS{}

		size, err := GetDAGSizeFromFS(ctx, testFS, false)
		require.NoError(t, err)
		// Empty filesystem still has some structure overhead
		assert.GreaterOrEqual(t, size, uint64(0))
	})

	t.Run("multiple files", func(t *testing.T) {
		testFS := fstest.MapFS{
			"file1.txt": &fstest.MapFile{Data: []byte("Content 1")},
			"file2.txt": &fstest.MapFile{Data: []byte("Content 2")},
			"file3.txt": &fstest.MapFile{Data: []byte("Content 3")},
		}

		size, err := GetDAGSizeFromFS(ctx, testFS, true)
		require.NoError(t, err)
		assert.Greater(t, size, uint64(100), "Multiple files should have significant DAG size")
	})

	t.Run("context cancellation", func(t *testing.T) {
		testFS := fstest.MapFS{
			"test.txt": &fstest.MapFile{Data: make([]byte, 10*1024*1024)}, // 10MB
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := GetDAGSizeFromFS(ctx, testFS, false)
		assert.Error(t, err, "Should error on context cancellation")
	})
}

func TestGetLogicalFileSizeFromFS(t *testing.T) {
	ctx := context.Background()

	t.Run("single file", func(t *testing.T) {
		testFS := fstest.MapFS{
			"test.txt": &fstest.MapFile{Data: []byte("Hello, World!")},
		}

		size, err := GetLogicalFileSizeFromFS(ctx, testFS, false)
		require.NoError(t, err)
		assert.Equal(t, uint64(13), size, "Logical size should match raw file size")
	})

	t.Run("multiple files sum", func(t *testing.T) {
		testFS := fstest.MapFS{
			"file1.txt": &fstest.MapFile{Data: []byte("Hello")},
			"file2.txt": &fstest.MapFile{Data: []byte("World")},
			"file3.txt": &fstest.MapFile{Data: []byte("!")},
		}

		size, err := GetLogicalFileSizeFromFS(ctx, testFS, true)
		require.NoError(t, err)
		assert.Equal(t, uint64(11), size, "Logical size should sum all files")
	})

	t.Run("empty file", func(t *testing.T) {
		testFS := fstest.MapFS{
			"empty.txt": &fstest.MapFile{Data: []byte{}},
		}

		size, err := GetLogicalFileSizeFromFS(ctx, testFS, false)
		require.NoError(t, err)
		assert.Equal(t, uint64(0), size, "Empty file should have zero logical size")
	})

	t.Run("large file", func(t *testing.T) {
		largeData := make([]byte, 1024*1024) // 1MB
		testFS := fstest.MapFS{
			"large.bin": &fstest.MapFile{Data: largeData},
		}

		size, err := GetLogicalFileSizeFromFS(ctx, testFS, false)
		require.NoError(t, err)
		assert.Equal(t, uint64(1024*1024), size, "Logical size should match large file size")
	})

	t.Run("wrapInDir false", func(t *testing.T) {
		testFS := fstest.MapFS{
			"test.txt": &fstest.MapFile{Data: []byte("test")},
		}

		// wrapInDir should not affect logical size
		sizeWrapped, err := GetLogicalFileSizeFromFS(ctx, testFS, true)
		require.NoError(t, err)

		sizeNotWrapped, err := GetLogicalFileSizeFromFS(ctx, testFS, false)
		require.NoError(t, err)

		assert.Equal(t, sizeWrapped, sizeNotWrapped, "Logical size should be the same regardless of wrapping")
	})

	t.Run("context cancellation", func(t *testing.T) {
		testFS := fstest.MapFS{
			"test.txt": &fstest.MapFile{Data: make([]byte, 10*1024*1024)}, // 10MB
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := GetLogicalFileSizeFromFS(ctx, testFS, false)
		assert.Error(t, err, "Should error on context cancellation")
	})
}

func TestSizeCalculationsConsistency(t *testing.T) {
	ctx := context.Background()

	t.Run("DAG size always larger than logical size", func(t *testing.T) {
		testFS := fstest.MapFS{
			"test.txt": &fstest.MapFile{Data: []byte("test content")},
		}

		dagSize, err := GetDAGSizeFromFS(ctx, testFS, false)
		require.NoError(t, err)

		logicalSize, err := GetLogicalFileSizeFromFS(ctx, testFS, false)
		require.NoError(t, err)

		assert.Greater(t, dagSize, logicalSize, "DAG size includes overhead beyond logical size")
	})

	t.Run("proportional growth", func(t *testing.T) {
		// Create filesystems of different sizes
		smallFS := fstest.MapFS{
			"test.txt": &fstest.MapFile{Data: []byte("small")},
		}

		largeFS := fstest.MapFS{
			"test.txt": &fstest.MapFile{Data: []byte("this is much larger content")},
		}

		smallDAGSize, err := GetDAGSizeFromFS(ctx, smallFS, false)
		require.NoError(t, err)

		largeDAGSize, err := GetDAGSizeFromFS(ctx, largeFS, false)
		require.NoError(t, err)

		smallLogicalSize, err := GetLogicalFileSizeFromFS(ctx, smallFS, false)
		require.NoError(t, err)

		largeLogicalSize, err := GetLogicalFileSizeFromFS(ctx, largeFS, false)
		require.NoError(t, err)

		// Larger content should have larger sizes
		assert.Greater(t, largeLogicalSize, smallLogicalSize, "Logical size should be larger for larger content")
		assert.Greater(t, largeDAGSize, smallDAGSize, "DAG size should be larger for larger content")
	})
}

func TestGetDAGSizeFromFSWithSingleFileFS(t *testing.T) {
	ctx := context.Background()

	t.Run("SingleFileFS integration", func(t *testing.T) {
		// Create a temporary file
		tmpFile := createTestFile(t, "SingleFileFS test content")
		file, err := os.Open(tmpFile)
		require.NoError(t, err)
		defer file.Close()

		// Wrap in SingleFileFS
		singleFS := NewSingleFileFS(file, "test.txt")

		size, err := GetDAGSizeFromFS(ctx, singleFS, false)
		require.NoError(t, err)
		assert.Greater(t, size, uint64(0), "Should calculate DAG size from SingleFileFS")
	})
}
