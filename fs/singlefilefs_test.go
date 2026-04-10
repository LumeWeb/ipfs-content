package fs

import (
	"io"
	"io/fs"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestFile(t *testing.T, content string) string {
	t.Helper()
	
	// Create a temporary file
	tmpDir := t.TempDir()
	tmpFile, err := os.CreateTemp(tmpDir, "test-*.txt")
	require.NoError(t, err)
	
	// Write content
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	
	// Close and return the path
	tmpFile.Close()
	return tmpFile.Name()
}

func TestSingleFileFSBasic(t *testing.T) {
	content := "Hello, World!"
	tmpFile := createTestFile(t, content)
	
	// Open the file
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	// Create SingleFileFS
	singleFS := NewSingleFileFS(file, "test.txt")
	
	// Open by name
	openedFile, err := singleFS.Open("test.txt")
	require.NoError(t, err)
	defer openedFile.Close()
	
	// Read content
	buf := make([]byte, len(content))
	n, err := openedFile.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, len(content), n)
	assert.Equal(t, content, string(buf))
}

func TestSingleFileFSRootIsFile(t *testing.T) {
	content := "test content for single file fs"
	tmpFile := createTestFile(t, content)
	
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	singleFS := NewSingleFileFS(file, "test.bin")
	
	// Stat the root (this is what UploadFromFS does in upload.go)
	info, err := fs.Stat(singleFS, ".")
	require.NoError(t, err, "fs.Stat should not fail on root")
	
	// The root should represent a single file, not a directory
	assert.False(t, info.IsDir(), "Root of SingleFileFS should not be a directory, it represents a single file")
	
	// Verify size is correct
	assert.Equal(t, int64(len(content)), info.Size())
}

func TestSingleFileFSSeek(t *testing.T) {
	content := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	tmpFile := createTestFile(t, content)
	
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	singleFS := NewSingleFileFS(file, "test.txt")
	
	// Open file
	openedFile, err := singleFS.Open("test.txt")
	require.NoError(t, err)
	defer openedFile.Close()
	
	// Check that file implements io.Seeker
	seeker, ok := openedFile.(io.Seeker)
	require.True(t, ok, "file should implement io.Seeker")
	
	// Seek to position 10
	offset, err := seeker.Seek(10, io.SeekStart)
	require.NoError(t, err)
	assert.Equal(t, int64(10), offset)
	
	// Read from position 10
	buf := make([]byte, 5)
	n, err := openedFile.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "KLMNO", string(buf))
}

func TestSingleFileFSReOpen(t *testing.T) {
	content := "This is a test file for reopening"
	tmpFile := createTestFile(t, content)
	
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	singleFS := NewSingleFileFS(file, "test.txt")
	
	// First open and read
	file1, err := singleFS.Open("test.txt")
	require.NoError(t, err)
	defer file1.Close()
	
	buf1 := make([]byte, 4)
	n1, err := file1.Read(buf1)
	require.NoError(t, err)
	assert.Equal(t, 4, n1)
	assert.Equal(t, "This", string(buf1))
	
	// Reopen and read from beginning
	file2, err := singleFS.Open("test.txt")
	require.NoError(t, err)
	defer file2.Close()
	
	buf2 := make([]byte, 4)
	n2, err := file2.Read(buf2)
	require.NoError(t, err)
	assert.Equal(t, 4, n2)
	assert.Equal(t, "This", string(buf2))
}

func TestSingleFileFSInvalidPath(t *testing.T) {
	tmpFile := createTestFile(t, "test")
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	singleFS := NewSingleFileFS(file, "test.txt")
	
	// Try to open a non-existent file
	_, err = singleFS.Open("nonexistent.txt")
	assert.Equal(t, fs.ErrNotExist, err)
}

func TestSingleFileFSOpenDirectory(t *testing.T) {
	content := "test content"
	tmpFile := createTestFile(t, content)
	
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	singleFS := NewSingleFileFS(file, "test.txt")
	
	// Opening "." should return the same file (for compatibility)
	openedFile, err := singleFS.Open(".")
	require.NoError(t, err)
	defer openedFile.Close()
	
	// Verify we can read from it
	buf := make([]byte, len(content))
	n, err := openedFile.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, len(content), n)
	assert.Equal(t, content, string(buf))
}

func TestFileFromStat(t *testing.T) {
	content := "test content for FileFromStat"
	tmpFile := createTestFile(t, content)
	
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	// Get Stat
	stat, err := file.Stat()
	require.NoError(t, err)
	
	// Wrap with FileFromStat
	wrapped := FileFromStat(file, stat)
	defer wrapped.Close()
	
	// Verify wrapped file
	info, err := wrapped.Stat()
	require.NoError(t, err)
	
	// Verify IsDir returns false
	assert.False(t, info.IsDir(), "FileFromStat should ensure IsDir returns false")
	
	// Verify other properties
	assert.Equal(t, stat.Name(), info.Name())
	assert.Equal(t, stat.Size(), info.Size())
	assert.Equal(t, stat.Mode(), info.Mode())
}

func TestFileFromStatNil(t *testing.T) {
	tmpFile := createTestFile(t, "")
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	// FileFromStat with nil stat should return the file unchanged
	wrapped := FileFromStat(file, nil)
	defer wrapped.Close()
	
	assert.Equal(t, file, wrapped)
}

func TestSingleFileFSCompatibilityWithUploadPattern(t *testing.T) {
	// Test the pattern used by UploadFromFS in upload.go
	content := "Content for upload pattern testing"
	tmpFile := createTestFile(t, content)
	
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	singleFS := NewSingleFileFS(file, "test-file.bin")
	
	// This is what UploadFromFS does in upload.go line 299-305
	info, err := fs.Stat(singleFS, ".")
	require.NoError(t, err, "cannot access filesystem")
	
	// Check IsDir behavior - single file should not be a directory
	wrapInDir := false
	if info.IsDir() {
		wrapInDir = true
	}
	
	// For a single file, wrapInDir should remain false
	assert.False(t, wrapInDir, "Single file should not be wrapped in a directory")
}

func TestSingleFileFSLargeFile(t *testing.T) {
	// Test with a larger file to buffer boundaries
	// Create content larger than typical buffer size
	content := string(make([]byte, 10000)) // 10KB file
	for i := range content {
		if i >= len("test") {
			continue
		}
		// Fill with some pattern
		currentContent := []rune(content)
		currentContent[i] = rune(i % 256)
		content = string(currentContent)
	}
	
	tmpFile := createTestFile(t, content)
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	singleFS := NewSingleFileFS(file, "large.txt")
	
	// Open and read in chunks
	openedFile, err := singleFS.Open("large.txt")
	require.NoError(t, err)
	defer openedFile.Close()
	
	chunkSize := 4096
	buf := make([]byte, chunkSize)
	totalRead := 0
	
	for {
		n, err := openedFile.Read(buf)
		totalRead += n
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
	}
	
	assert.Equal(t, len(content), totalRead, "Should have read entire file")
}

func TestSingleFileFSZeroLengthFile(t *testing.T) {
	// Test with a zero-length file
	tmpFile := createTestFile(t, "")

	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()

	singleFS := NewSingleFileFS(file, "empty.txt")

	openedFile, err := singleFS.Open("empty.txt")
	require.NoError(t, err)
	defer openedFile.Close()

	// Read should return 0, EOF immediately
	buf := make([]byte, 10)
	n, err := openedFile.Read(buf)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, 0, n)
}

func TestSingleFileFSSeekBeyondEOF(t *testing.T) {
	content := "short"
	tmpFile := createTestFile(t, content)
	
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	singleFS := NewSingleFileFS(file, "test.txt")
	
	openedFile, err := singleFS.Open("test.txt")
	require.NoError(t, err)
	defer openedFile.Close()
	
	seeker, ok := openedFile.(io.Seeker)
	require.True(t, ok)
	
	// Seek beyond file length
	offset, err := seeker.Seek(100, io.SeekStart)
	require.NoError(t, err)
	assert.Equal(t, int64(100), offset)
	
	// Reading should return EOF
	buf := make([]byte, 10)
	n, err := openedFile.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}

func TestSingleFileFSSeekCurrentFromEnd(t *testing.T) {
	content := "test data for seek"
	tmpFile := createTestFile(t, content)
	
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	singleFS := NewSingleFileFS(file, "test.txt")
	
	openedFile, err := singleFS.Open("test.txt")
	require.NoError(t, err)
	defer openedFile.Close()
	
	seeker, ok := openedFile.(io.Seeker)
	require.True(t, ok)
	
	// Read to advance position
	buf := make([]byte, 5)
	_, err = openedFile.Read(buf)
	require.NoError(t, err)
	
	// Seek backward 3 bytes
	offset, err := seeker.Seek(-3, io.SeekCurrent)
	require.NoError(t, err)
	// Position should be 2 (5 - 3)
	assert.Equal(t, int64(2), offset)
	
	// Verify position by reading
	n, err := openedFile.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "st da", string(buf))
}

func TestSingleFileFSSeekEnd(t *testing.T) {
	content := "seek end test"
	tmpFile := createTestFile(t, content)

	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()

	singleFS := NewSingleFileFS(file, "test.txt")

	openedFile, err := singleFS.Open("test.txt")
	require.NoError(t, err)
	defer openedFile.Close()

	seeker, ok := openedFile.(io.Seeker)
	require.True(t, ok)

	// Seek to -5 from end
	offset, err := seeker.Seek(-5, io.SeekEnd)
	require.NoError(t, err)
	assert.Equal(t, int64(len(content)-5), offset)

	// Read and verify
	buf := make([]byte, 10)
	n, err := openedFile.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	// The string is "seek end test" (13 chars), -5 from end is position 8
	// From position 8 we get chars: " test" (space + "test")
	assert.Equal(t, " test", string(buf[:5]))
}

func TestSingleFileFSStatCorrect(t *testing.T) {
	content := "stat test content"
	tmpFile := createTestFile(t, content)
	
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	// Get stat from original file
	origStat, err := file.Stat()
	require.NoError(t, err)
	
	singleFS := NewSingleFileFS(file, "stattest.txt")
	
	// Stat through SingleFileFS
	info, err := fs.Stat(singleFS, ".")
	require.NoError(t, err)
	
	// Verify stats match
	assert.Equal(t, origStat.Name(), info.Name())
	assert.Equal(t, origStat.Size(), info.Size())
	assert.Equal(t, origStat.Mode(), info.Mode())
	assert.False(t, info.IsDir(), "IsDir should be false for SingleFileFS root")
}

func TestSingleFileFSStatMethod(t *testing.T) {
	// Test that SingleFileFS.Stat() method works correctly (implements fs.StatFS)
	content := "test content for Stat method"
	tmpFile := createTestFile(t, content)
	
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	singleFS := NewSingleFileFS(file, "stat-test.txt")

	// Stat the root using the method directly
	rootInfo, err := singleFS.Stat(".")
	require.NoError(t, err, "Stat should not fail on root")
	assert.False(t, rootInfo.IsDir(), "Root should not be a directory")
	assert.Equal(t, int64(len(content)), rootInfo.Size())

	// Stat by filename
	fileInfo, err := singleFS.Stat("stat-test.txt")
	require.NoError(t, err)
	assert.False(t, fileInfo.IsDir(), "File should not be a directory")
	assert.Equal(t, int64(len(content)), fileInfo.Size())

	// Stat invalid path should fail
	_, err = singleFS.Stat("nonexistent.txt")
	assert.Equal(t, fs.ErrNotExist, err)
}

// TestSingleFileFSFilenameWithSlashes tests filename with slashes

func TestSingleFileFSFilenameWithSlashes(t *testing.T) {
	// Test that filename with slashes is handled correctly
	content := "test"
	tmpFile := createTestFile(t, content)
	
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	// Create SingleFileFS with filename that has path separators
	singleFS := NewSingleFileFS(file, "subdir/file.txt")
	
	// Try to open with the full path - should work
	openedFile, err := singleFS.Open("subdir/file.txt")
	require.NoError(t, err)
	defer openedFile.Close()
	
	// Try to open just the base name - should fail (because it's the full path seen as the name)
	_, err = singleFS.Open("file.txt")
	assert.Equal(t, fs.ErrNotExist, err)
}

func TestSingleFileFSReadAt(t *testing.T) {
	// Test if file supports ReadAt (for efficient chunked reads)
	content := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	tmpFile := createTestFile(t, content)
	
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()
	
	singleFS := NewSingleFileFS(file, "test.txt")
	
	openedFile, err := singleFS.Open("test.txt")
	require.NoError(t, err)
	defer openedFile.Close()
	
	// Check if file supports ReadAt - os.File does, but SingleFileFS may not expose it directly
	// This is informational - just to document the behavior
	readAtFile, ok := openedFile.(interface{ ReadAt([]byte, int64) (int, error) })
	if !ok {
		t.Skip("singleFileFS returned file doesn't support ReadAt directly (expected)")
		return
	}
	
	// If ReadAt is supported, test it
	buf := make([]byte, 5)
	n, err := readAtFile.ReadAt(buf, 10)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "KLMNO", string(buf))
}
