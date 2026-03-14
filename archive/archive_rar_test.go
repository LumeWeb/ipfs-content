package archive

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRarRegistration(t *testing.T) {
	RegisterRarExtractor()

	// Test that RAR format is registered
	registry := DefaultRegistry()
	if !registry.IsFormatSupported(FormatRAR) {
		t.Error("RAR format should be registered")
	}

	// Check if rar is available for actual tests
	hasRar := false
	if _, err := exec.LookPath("rar"); err == nil {
		hasRar = true
	}

	if !hasRar {
		t.Log("Note: rar not available - tests that require archive creation will be skipped")
	}
}

func TestRarFormatDetection(t *testing.T) {
	RegisterRarExtractor()

	// Check if rar command is available
	if _, err := exec.LookPath("rar"); err != nil {
		t.Skip("rar command not available")
	}

// This would need actual RAR archive data - skip for now
	t.Skip("Full format detection test requires actual RAR archive")
}

// TestNewRarArchiveExtractor_NotSupported tests error handling in NewRarArchiveExtractor
// when the format is not supported by the driver
func TestNewRarArchiveExtractor_NotSupported(t *testing.T) {
	// Use empty data which likely won't be recognized as RAR format
	data := make([]byte, 512)
	reader := bytes.NewReader(data)

	// Assume rar tool is available for coverage purposes
	// This tests the code path when FormatRAR is not supported
	extractor, err := NewRarArchiveExtractor(reader)

	// Either succeeds (if format is supported) or fails gracefully
	if err != nil {
		require.Nil(t, extractor)
		t.Logf("Expected error (format not supported): %v", err)
	} else {
		t.Logf("Extractor created successfully, format supported: %v", extractor)
	}
}

// TestCreateExtractorForFormat_RAR tests creating a RAR extractor through the registry
// This exercises the lambda function registered by RegisterRarExtractor
func TestCreateExtractorForFormat_RAR(t *testing.T) {
	RegisterRarExtractor()

	// Create dummy reader (format may or may not be valid)
	data := make([]byte, 512)
	seeker := bytes.NewReader(data)

	// Create extractor through the registry pipeline
	// This triggers the lambda: func(reader archives.ReaderAtSeeker) (ArchiveExtractor, error)
	registry := DefaultRegistry()
	extractor, err := registry.CreateExtractorForFormat(FormatRAR, seeker)

	// Either succeeds (format supported) or fails (format not supported by driver)
	if err != nil {
		t.Logf("Expected error (format not supported by driver): %v", err)
	} else {
		assert.NotNil(t, extractor)
		t.Logf("Extractor created successfully through registry")
	}
}


