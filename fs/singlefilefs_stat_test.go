package fs

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSingleFileFSStatUsesProvidedName verifies that Stat returns the
// caller-supplied filename, not the underlying OS file's name.  This is
// critical for fs.WalkDir-based CAR builders, which derive UnixFS entry
// names from Stat output.  A regression here would leak the OS temp-file
// name into the IPFS DAG.
func TestSingleFileFSStatUsesProvidedName(t *testing.T) {
	// Create a temp file whose OS name is very different from the desired
	// logical name, so a leak is easy to detect.
	// Without the fix, the OS temp-file name would leak into the UnixFS DAG.
	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, "some-random-temp-name-12345.dat")
	err := os.WriteFile(tmpPath, []byte("test content"), 0644)
	require.NoError(t, err)

	file, err := os.Open(tmpPath)
	require.NoError(t, err)
	defer file.Close()

	desiredName := "index.html"
	singleFS := NewSingleFileFS(file, desiredName)

	// Stat(".") is what WalkDir calls first.
	info, err := singleFS.Stat(".")
	require.NoError(t, err)
	assert.Equal(t, desiredName, info.Name(),
		"Stat(\".\") must report the provided filename, not the OS file name")

	// Stat by the provided filename should also work.
	info2, err := singleFS.Stat(desiredName)
	require.NoError(t, err)
	assert.Equal(t, desiredName, info2.Name(),
		"Stat(filename) must report the provided filename")
}

// TestSingleFileFSWalkDirUsesProvidedName is the end-to-end check that
// fs.WalkDir sees the correct filename, which is how the CAR builder
// discovers entry names.
func TestSingleFileFSWalkDirUsesProvidedName(t *testing.T) {
	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, "os-temp-name-xyz.dat")
	err := os.WriteFile(tmpPath, []byte("hello"), 0644)
	require.NoError(t, err)

	file, err := os.Open(tmpPath)
	require.NoError(t, err)
	defer file.Close()

	desiredName := "index.html"
	singleFS := NewSingleFileFS(file, desiredName)

	// SingleFileFS makes "." the file itself (not a directory containing a
	// file).  fs.WalkDir therefore visits only "." with IsDir()==false and
	// never recurses into children.  The callback receives d.Name() which
	// must be the provided filename, not the OS temp-file name.
	var seenName string
	err = fs.WalkDir(singleFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		seenName = d.Name()
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, desiredName, seenName,
		"WalkDir entry name must be the provided filename, not the OS temp file name")
}
