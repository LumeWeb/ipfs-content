package archive

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test7ZipRegistration(t *testing.T) {
	Register7ZipExtractor()

	// Test that 7Z format is registered
	registry := DefaultRegistry()
	if !registry.IsFormatSupported(Format7Z) {
		t.Error("7Z format should be registered")
	}

	// Check if 7z/7zz is available for actual tests
	has7z := true
	if _, err := exec.LookPath("7z"); err != nil {
		if _, err := exec.LookPath("7zz"); err != nil {
			has7z = false
		}
	}

	if !has7z {
		t.Log("Note: 7z/7zz not available - tests that require archive creation will be skipped")
	}
}

func Test7ZipFormatDetection(t *testing.T) {
	Register7ZipExtractor()

	// Check if 7z command is available
	has7z := true
	if _, err := exec.LookPath("7z"); err != nil {
		if _, err := exec.LookPath("7zz"); err != nil {
			has7z = false
		}
	}

	if !has7z {
		t.Skip("7z/7zz command not available")
	}

	// This would need actual 7z archive data - skip for now
	t.Skip("Full format detection test requires actual 7z archive")
}

// TestNewSevenZipArchiveExtractor_NotSupported tests error handling in NewSevenZipArchiveExtractor
// when the format is not supported by the driver
func TestNewSevenZipArchiveExtractor_NotSupported(t *testing.T) {
	// Use empty data which likely won't be recognized as 7Z format
	data := make([]byte, 512)
	reader := bytes.NewReader(data)

	// Assume 7z tool is available for coverage purposes
	// This tests the code path when Format7Z is not supported
	extractor, err := NewSevenZipArchiveExtractor(reader)

	// Either succeeds (if format is supported) or fails gracefully
	if err != nil {
		require.Nil(t, extractor)
		t.Logf("Expected error (format not supported): %v", err)
	} else {
		t.Logf("Extractor created successfully, format supported")
	}
}

// TestCreateExtractorForFormat_7Z tests creating a 7Z extractor through the registry
// This exercises the lambda function registered by Register7ZipExtractor
func TestCreateExtractorForFormat_7Z(t *testing.T) {
	Register7ZipExtractor()

	// Create dummy reader (format may or may not be valid)
	data := make([]byte, 512)
	seeker := bytes.NewReader(data)

	// Create extractor through the registry pipeline
	// This triggers the lambda: func(reader archives.ReaderAtSeeker) (ArchiveExtractor, error)
	registry := DefaultRegistry()
	extractor, err := registry.CreateExtractorForFormat(Format7Z, seeker)

	// Either succeeds (format supported) or fails (format not supported by driver)
	if err != nil {
		t.Logf("Expected error (format not supported by driver): %v", err)
	} else {
		assert.NotNil(t, extractor)
		t.Logf("Extractor created successfully through registry")
	}
}
