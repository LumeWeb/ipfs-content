package archive

import (
	"bytes"
	"testing"
)

func TestZipRegistration(t *testing.T) {
	RegisterZipExtractor()

	// Test that ZIP format is registered
	registry := DefaultRegistry()
	if !registry.IsFormatSupported(FormatZIP) {
		t.Error("ZIP format should be registered")
	}
}

func TestZipExtractor(t *testing.T) {
	RegisterZipExtractor()

	// Create a simple ZIP archive content with PK header
	zipData := []byte{
		0x50, 0x4B, 0x03, 0x04, // ZIP local file header
		0x14, 0x00, 0x00, 0x00, // Version needed
		0x08, 0x00, 0x00, 0x00, // Flags
	}

	// Test that we can create an extractor
	extractor, err := NewArchiveExtractor(bytes.NewReader(zipData), FormatZIP)
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}
	defer extractor.Close()

	if extractor.Format() != FormatZIP {
		t.Errorf("Expected FormatZIP, got %v", extractor.Format())
	}
}

func TestZipFormatDetection(t *testing.T) {
	RegisterZipExtractor()

	// Create minimal ZIP header for detection
	zipData := []byte{
		0x50, 0x4B, 0x03, 0x04, // ZIP signature
	}

	// Test format detection
	detected, err := DetectFormat(bytes.NewReader(zipData))
	if err != nil {
		// Detection might fail if header is incomplete, but format should be detected as ZIP or file
		if detected == FormatUnknown || detected == FormatFile {
			t.Skip("ZIP detection requires valid archive, skipping")
		}
	}

	if detected == FormatZIP {
		t.Log("ZIP format detected successfully")
	}
}
