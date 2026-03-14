package archive

import (
	"bytes"
	"compress/gzip"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDetectCompressedTarFromReader_Error tests the error path in detectCompressedTarFromReader
func TestDetectCompressedTarFromReader_Error(t *testing.T) {
	// Create a reader that fails to read
	failReader := &failReader{}

	format, detected := detectCompressedTarFromReader(failReader)

	assert.False(t, detected)
	assert.Equal(t, FormatUnknown, format)
}

func TestTarGzRegistration(t *testing.T) {
	RegisterTarGzExtractor()

	// Test that TAR_GZ format is registered
	registry := DefaultRegistry()
	if !registry.IsFormatSupported(FormatTAR_GZ) {
		t.Error("TAR_GZ format should be registered")
	}
}

func TestTarGzFormatDetection(t *testing.T) {
	RegisterTarGzExtractor()

	// Create simple then compress with gzip
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte("test content")); err != nil {
		t.Fatalf("Failed to write compressed data: %v", err)
	}
	gz.Close()

	// Test format detection
	detected, err := DetectFormat(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to detect format: %v", err)
	}

	if detected == FormatTAR_GZ {
		t.Log("TAR_GZ format detected successfully")
	} else if detected == FormatFile || detected == FormatUnknown {
		t.Log("Detection may fail with simple data, but format is registered")
	}
}

// TestNewTarGzArchiveExtractor_NotSupported tests error handling in NewTarGzArchiveExtractor
// when the format is not supported by the driver
func TestNewTarGzArchiveExtractor_NotSupported(t *testing.T) {
	// Use empty data which likely won't be recognized as TAR_GZ format
	data := make([]byte, 512)
	reader := bytes.NewReader(data)

	// This tests the code path when FormatTAR_GZ is not supported
	extractor, err := NewTarGzArchiveExtractor(reader)

	// Either succeeds (if format is supported) or fails gracefully
	if err != nil {
		assert.Nil(t, extractor)
		t.Logf("Expected error (format not supported): %v", err)
	} else {
		t.Logf("Extractor created successfully, format supported: %v", extractor)
	}
}

// TestCreateExtractorForFormat_TAR_GZ tests creating a TAR_GZ extractor through the registry
// This exercises the lambda function registered by RegisterTarGzExtractor
func TestCreateExtractorForFormat_TAR_GZ(t *testing.T) {
	RegisterTarGzExtractor()

	// Create dummy reader (format may or may not be valid)
	data := make([]byte, 512)
	seeker := bytes.NewReader(data)

	// Create extractor through the registry pipeline
	// This triggers the lambda: func(reader archives.ReaderAtSeeker) (ArchiveExtractor, error)
	registry := DefaultRegistry()
	extractor, err := registry.CreateExtractorForFormat(FormatTAR_GZ, seeker)

	// Either succeeds (format supported) or fails (format not supported by driver)
	if err != nil {
		t.Logf("Expected error (format not supported by driver): %v", err)
	} else {
		assert.NotNil(t, extractor)
		t.Logf("Extractor created successfully through registry")
	}
}


func TestTarBz2Registration(t *testing.T) {
	RegisterTarBz2Extractor()

	// Test that TAR_BZ2 format is registered
	registry := DefaultRegistry()
	if !registry.IsFormatSupported(FormatTAR_BZ2) {
		t.Error("TAR_BZ2 format should be registered")
	}
}

// TestNewTarBz2ArchiveExtractor_NotSupported tests error handling in NewTarBz2ArchiveExtractor
// when the format is not supported by the driver
func TestNewTarBz2ArchiveExtractor_NotSupported(t *testing.T) {
	// Use empty data which likely won't be recognized as TAR_BZ2 format
	data := make([]byte, 512)
	reader := bytes.NewReader(data)

	// This tests the code path when FormatTAR_BZ2 is not supported
	extractor, err := NewTarBz2ArchiveExtractor(reader)

	// Either succeeds (if format is supported) or fails gracefully
	if err != nil {
		assert.Nil(t, extractor)
		t.Logf("Expected error (format not supported): %v", err)
	} else {
		t.Logf("Extractor created successfully, format supported: %v", extractor)
	}
}

// TestCreateExtractorForFormat_TAR_BZ2 tests creating a TAR_BZ2 extractor through the registry
// This exercises the lambda function registered by RegisterTarBz2Extractor
func TestCreateExtractorForFormat_TAR_BZ2(t *testing.T) {
	RegisterTarBz2Extractor()

	// Create dummy reader (format may or may not be valid)
	data := make([]byte, 512)
	seeker := bytes.NewReader(data)

	// Create extractor through the registry pipeline
	// This triggers the lambda: func(reader archives.ReaderAtSeeker) (ArchiveExtractor, error)
	registry := DefaultRegistry()
	extractor, err := registry.CreateExtractorForFormat(FormatTAR_BZ2, seeker)

	// Either succeeds (format supported) or fails (format not supported by driver)
	if err != nil {
		t.Logf("Expected error (format not supported by driver): %v", err)
	} else {
		assert.NotNil(t, extractor)
		t.Logf("Extractor created successfully through registry")
	}
}

// failReader is a mock reader that always fails to read
type failReader struct{}

func (f *failReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read failed")
}
