package archive

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestArchivesDriver_Filesystem_UnsupportedFormat tests the error path when
// attempting to get filesystem for an unsupported archive format
func TestArchivesDriver_Filesystem_UnsupportedFormat(t *testing.T) {
	// Create a driver with an unsupported format (FormatUnknown)
	data := make([]byte, 512)
	reader := bytes.NewReader(data)
	driver := NewArchivesDriver(FormatUnknown, reader)

	ctx := context.Background()
	fsys, err := driver.Filesystem(ctx)

	assert.Error(t, err)
	assert.Nil(t, fsys)
	assert.Contains(t, err.Error(), "unsupported archive format")
}

// TestArchivesDriver_Filesystem_SupportedFormat tests the success path when
// attempting to get filesystem for a supported archive format
func TestArchivesDriver_Filesystem_SupportedFormat(t *testing.T) {
	// Test with ZIP format (supported format)
	data := []byte{0x50, 0x4b, 0x03, 0x04, 0x00} // ZIP file header
	reader := bytes.NewReader(data)
	driver := NewArchivesDriver(FormatZIP, reader)

	ctx := context.Background()
	fsys, err := driver.Filesystem(ctx)

	// May succeed or fail based on whether archive tools are available,
	// but should not fail with "unsupported format" error
	if err != nil {
		assert.Nil(t, fsys)
		assert.NotContains(t, err.Error(), "unsupported archive format")
	} else {
		assert.NotNil(t, fsys)
	}
}

// TestArchivesDriver_Filesystem_TarFormat tests filesystem with TAR format
func TestArchivesDriver_Filesystem_TarFormat(t *testing.T) {
	data := make([]byte, 512) // This may or may not be valid TAR data
	reader := bytes.NewReader(data)
	driver := NewArchivesDriver(FormatTAR, reader)

	ctx := context.Background()
	fsys, err := driver.Filesystem(ctx)

	// May succeed or fail based on data validity,
	// but should not fail with "unsupported format" error
	if err != nil {
		assert.Nil(t, fsys)
		assert.NotContains(t, err.Error(), "unsupported archive format")
	} else {
		assert.NotNil(t, fsys)
	}
}

// TestArchivesDriver_Filesystem_RarFormat tests filesystem with RAR format
func TestArchivesDriver_Filesystem_RarFormat(t *testing.T) {
	data := []byte{0x52, 0x61, 0x72, 0x21, 0x1A, 0x07} // RAR file signature
	reader := bytes.NewReader(data)
	driver := NewArchivesDriver(FormatRAR, reader)

	ctx := context.Background()
	fsys, err := driver.Filesystem(ctx)

	// May succeed or fail based on whether archive tools are available,
	// but should not fail with "unsupported format" error
	if err != nil {
		assert.Nil(t, fsys)
		assert.NotContains(t, err.Error(), "unsupported archive format")
	} else {
		assert.NotNil(t, fsys)
	}
}

// TestArchivesDriver_Filesystem_7ZFormat tests filesystem with 7Z format
func TestArchivesDriver_Filesystem_7ZFormat(t *testing.T) {
	data := []byte{0x37, 0x7A, 0xBC, 0xAF, 0x27, 0x1C} // 7Z file signature
	reader := bytes.NewReader(data)
	driver := NewArchivesDriver(Format7Z, reader)

	ctx := context.Background()
	fsys, err := driver.Filesystem(ctx)

	// May succeed or fail based on whether archive tools are available,
	// but should not fail with "unsupported format" error
	if err != nil {
		assert.Nil(t, fsys)
		assert.NotContains(t, err.Error(), "unsupported format")
	} else {
		assert.NotNil(t, fsys)
	}
}
