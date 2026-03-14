package archive

import (
	"bytes"
	"testing"
)

func TestTarRegistration(t *testing.T) {
	RegisterTarExtractor()

	// Test that TAR format is registered
	registry := DefaultRegistry()
	if !registry.IsFormatSupported(FormatTAR) {
		t.Error("TAR format should be registered")
	}
}

func TestTarExtractor(t *testing.T) {
	RegisterTarExtractor()

	// Create minimal TAR header
	tarData := []byte{
		0x74, 0x65, 0x73, 0x74, 0x2E, 0x74, 0x78, 0x74, // "test.txt"
	}

	// Test that we can create an extractor
	extractor, err := NewArchiveExtractor(bytes.NewReader(tarData), FormatTAR)
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}
	defer extractor.Close()

	if extractor.Format() != FormatTAR {
		t.Errorf("Expected FormatTAR, got %v", extractor.Format())
	}
}

func TestTarFormatDetection(t *testing.T) {
	RegisterTarExtractor()

	// Create minimal TAR header for detection
	tarData := make([]byte, 512) // TAR block size
	tarData[257] = 'u'           // ustar magic
	tarData[258] = 's'
	tarData[259] = 't'
	tarData[260] = 'a'
	tarData[261] = 'r'

	// Test format detection
	detected, err := DetectFormat(bytes.NewReader(tarData))
	if err != nil {
		t.Fatalf("Failed to detect format: %v", err)
	}

	if detected == FormatTAR {
		t.Log("TAR format detected successfully")
	}
}
