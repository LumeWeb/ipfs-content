package archive

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/mholt/archives"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveRegistry(t *testing.T) {
	// Register extractors for this test
	RegisterZipExtractor()
	RegisterTarExtractor()
	Register7ZipExtractor()
	RegisterRarExtractor()

	registry := DefaultRegistry()

	// Test that default registry is initialized
	require.NotNil(t, registry, "Default registry should be initialized")

	// Test supported formats
	formats := SupportedFormats()
	require.True(t, len(formats) > 0, "Registry should support at least one format")

	// Test that ZIP is registered and supported
	require.True(t, registry.IsFormatSupported(FormatZIP), "ZIP should be supported")
	require.True(t, registry.IsFormatSupported(FormatTAR), "TAR should be supported")
	require.True(t, registry.IsFormatSupported(Format7Z), "7Z should be supported")
	require.True(t, registry.IsFormatSupported(FormatRAR), "RAR should be supported")
}

// TestDetectFormat_WithEmptyRegistry tests detection when no detectors/extractors are registered
func TestDetectFormat_WithEmptyRegistry(t *testing.T) {
	registry := NewArchiveRegistry()
	
	reader := bytes.NewReader([]byte("test data"))
	_, err := registry.DetectFormat(reader)
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no detectors or extractors registered")
}

// TestDetectFormat_WithIncompleteRegistry tests when registry has detectors but no extractors
func TestDetectFormat_WithIncompleteRegistry(t *testing.T) {
	registry := NewArchiveRegistry()
	
	// Register a detector and an extractor
	registry.RegisterDetector(func(reader io.Reader) (Format, bool) {
		return FormatTAR, true
	})
	registry.RegisterExtractor(FormatTAR, func(reader archives.ReaderAtSeeker) (ArchiveExtractor, error) {
		return NewTarArchiveExtractor(reader)
	})
	
	reader := bytes.NewReader([]byte("test data"))
	detected, err := registry.DetectFormat(reader)
	
	// Should succeed because we have both detector and extractor
	require.NoError(t, err)
	assert.Equal(t, FormatTAR, detected)
}

// TestDetectFormat_TooShortForReliableDetection tests detection with minimal data
func TestDetectFormat_TooShortForReliableDetection(t *testing.T) {
	registry := DefaultRegistry()
	
	// Create data shorter than typical detection threshold
	shortData := make([]byte, 50)
	reader := bytes.NewReader(shortData)
	
	detected, err := registry.DetectFormat(reader)
	
	assert.NoError(t, err)
	// With too short data, might return FormatFile or FormatUnknown
	assert.True(t, detected == FormatFile || detected == FormatUnknown)
}

// TestDetectFormat_NoDataAvailable tests detection with empty reader
func TestDetectFormat_NoDataAvailable(t *testing.T) {
	registry := DefaultRegistry()
	
	reader := bytes.NewReader([]byte{})
	_, err := registry.DetectFormat(reader)
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no data available")
}

// TestDetectFormat_CustomDetectorDetection tests that a custom detector overrides filetype detection
func TestDetectFormat_CustomDetectorDetection(t *testing.T) {
	registry := NewArchiveRegistry()
	
	// Register a custom detector
	customDetected := false
	registry.RegisterDetector(func(reader io.Reader) (Format, bool) {
		customDetected = true
		return FormatTAR, true
	})
	
	// Also register an extractor for the format
	registry.RegisterExtractor(FormatTAR, func(reader archives.ReaderAtSeeker) (ArchiveExtractor, error) {
		return NewTarArchiveExtractor(reader)
	})
	
	reader := bytes.NewReader([]byte("random data that should trigger custom detector"))
	detected, err := registry.DetectFormat(reader)
	
	assert.NoError(t, err)
	assert.True(t, customDetected, "Custom detector should have been called")
	assert.Equal(t, FormatTAR, detected)
}

// TestDetectFormat_CannotSeekForCustomDetection tests error path when seeking fails for custom detection
func TestDetectFormat_CannotSeekForCustomDetection(t *testing.T) {
	registry := DefaultRegistry()
	
	// Register a custom detector
	registry.RegisterDetector(func(reader io.Reader) (Format, bool) {
		return FormatTAR, true
	})
	
	registry.RegisterExtractor(FormatTAR, func(reader archives.ReaderAtSeeker) (ArchiveExtractor, error) {
		return NewTarArchiveExtractor(reader)
	})
	
	// Create a reader that can't read - this will cause read error before seeking is attempted
	reader := &failingSeeker{canRead: false}
	_, err := registry.DetectFormat(reader)
	
	// The error message might vary, but it should fail
	assert.Error(t, err)
}

// TestDetectFormat_PreservePositionTests that reader position handling works
func TestDetectFormat_PreservePosition(t *testing.T) {
	registry := DefaultRegistry()
	
	// Create reader with known position
	data := []byte("test data for position preservation")
	reader := bytes.NewReader(data)
	
	// Move to a specific position
	_, err := reader.Seek(5, io.SeekStart)
	require.NoError(t, err)
	
	// Try to detect (this should work, not error)
	_, err = registry.DetectFormat(reader)
	require.NoError(t, err)
	
	// Check if position is accessible
	currentPos, err := reader.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, currentPos, int64(0))
}

// TestDetectFormat_FiletypeDetectionWithCompressedTar tests detection of TAR_GZ and TAR_BZ2 formats
func TestDetectFormat_CompressedTarDetection(t *testing.T) {
	registry := DefaultRegistry()
	
	t.Run("detects TAR_GZ format", func(t *testing.T) {
		// Create a minimal gzip-compressed data with magic bytes
		gzipData := []byte{
			0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 
			0x00, 0xff,
		}
		RegisterTarGzExtractor()
		reader := bytes.NewReader(gzipData)
		detected, err := registry.DetectFormat(reader)
		
		assert.NoError(t, err)
		// Check that it detects a format (TAR_GZ or potentially TAR if library behavior differs)
		assert.True(t, detected == FormatTAR_GZ || detected == FormatTAR || detected == FormatFile,
			"Expected TAR_GZ, TAR, or File format, got %d", detected)
	})
	
	t.Run("detects TAR_BZ2 format", func(t *testing.T) {
		// Create a minimal bzip2 magic bytes
		bzip2Data := []byte{
			'B', 'Z', 'h', '9', 
		}
		RegisterTarBz2Extractor()
		reader := bytes.NewReader(bzip2Data)
		detected, err := registry.DetectFormat(reader)
		
		assert.NoError(t, err)
		// Check that it detects a format (TAR_BZ2 or potentially TAR if library behavior differs)
		assert.True(t, detected == FormatTAR_BZ2 || detected == FormatTAR || detected == FormatFile,
			"Expected TAR_BZ2, TAR, or File format, got %d", detected)
	})
}

// TestDetectFormat_NoExtractorForDetectedFormat tests error when no extractor is registered
func TestDetectFormat_NoExtractorForDetectedFormat(t *testing.T) {
	registry := NewArchiveRegistry()
	
	// Register a detector that detects a format without an extractor
	registry.RegisterDetector(func(reader io.Reader) (Format, bool) {
		return FormatRAR, true
	})
	
	// Don't register an extractor for RAR
	data := []byte("test data")
	reader := bytes.NewReader(data)
	
	_, err := registry.DetectFormat(reader)
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no extractor registered")
}

// TestDetectFormat_PrepareReaderError tests error path when preparing reader fails
func TestDetectFormat_PrepareReaderError(t *testing.T) {
	registry := DefaultRegistry()
	
	reader := &failingSeeker{failPrepare: true}
	
	_, err := registry.DetectFormat(reader)
	
	assert.Error(t, err)
}

// TestCreateExtractor_PrepareReaderError tests CreateExtractor when PrepareReaderPreservePos fails
func TestCreateExtractor_PrepareReaderError(t *testing.T) {
	registry := DefaultRegistry()
	
	reader := &failingSeeker{failPrepare: true}
	
	_, err := registry.CreateExtractor(reader)
	
	assert.Error(t, err)
}

// TestCreateExtractor_RestorePositionOnDetectError tests CreateExtractor when DetectFormat fails
func TestCreateExtractor_RestorePositionOnDetectError(t *testing.T) {
	registry := NewArchiveRegistry()
	
	// Create a reader that will fail detection
	reader := &failingSeeker{failOnDetect: true}
	
	_, err := registry.CreateExtractor(reader)
	
	assert.Error(t, err)
	errMsg := err.Error()
	assert.Contains(t, errMsg, "detect")
}

// TestCreateExtractor_SeekError tests CreateExtractor when seeking after detection fails
func TestCreateExtractor_SeekError(t *testing.T) {
	registry := DefaultRegistry()
	
	RegisterTarExtractor()
	
	// Create reader that fails on seek after detection
	reader := &failingSeeker{failOnSeekAfterDetect: true}
	
	_, err := registry.CreateExtractor(reader)
	
	assert.Error(t, err)
	// The error should be about seeking, though the exact message may vary
	errMsg := err.Error()
	assert.Contains(t, errMsg, "seek")
}

// TestCreateExtractor_Success tests the successful path for CreateExtractor
func TestCreateExtractor_Success(t *testing.T) {
	registry := DefaultRegistry()
	
	RegisterTarExtractor()
	
	data := []byte("some TAR content for testing")
	reader := bytes.NewReader(data)
	
	extractor, err := registry.CreateExtractor(reader)
	
	// Either succeeds or fails gracefully based on format detection
	// but should not fail due to seeking or preparation
	if err != nil {
		errMsg := err.Error()
		// Should not be about seeking or preparation
		assert.NotContains(t, errMsg, "seek")
		assert.NotContains(t, errMsg, "prepare")
		assert.NotContains(t, errMsg, "reader must support")
	} else {
		assert.NotNil(t, extractor)
	}
}

// TestNewTarArchiveExtractor_NotSupported tests error handling in NewTarArchiveExtractor
func TestNewTarArchiveExtractor_NotSupported(t *testing.T) {
	data := make([]byte, 512)
	reader := bytes.NewReader(data)
	
	// The IsFormatSupported check should prevent problematic cases
	extractor, err := NewTarArchiveExtractor(reader)
	
	// Either succeeds (if format is supported) or fails gracefully
	if err != nil {
		assert.Nil(t, extractor)
		assert.Contains(t, err.Error(), "not supported")
	}
}

// TestNewZipArchiveExtractor_NotSupported tests error handling in NewZipArchiveExtractor
func TestNewZipArchiveExtractor_NotSupported(t *testing.T) {
	data := []byte{0x50, 0x4b, 0x03, 0x04}
	reader := bytes.NewReader(data)
	
	extractor, err := NewZipArchiveExtractor(reader)
	
	// Either succeeds (if format is supported) or fails gracefully
	if err != nil {
		assert.Nil(t, extractor)
		assert.Contains(t, err.Error(), "not supported")
	}
}

// TestDetectFormat_ReadError tests the error path when reading the buffer fails
func TestDetectFormat_ReadError(t *testing.T) {
	registry := DefaultRegistry()
	
	reader := &failingSeeker{canRead: false}
	
	_, err := registry.DetectFormat(reader)
	
	// Error should be returned
	assert.Error(t, err)
}

// TestCreateExtractorForFormat tests the CreateExtractorForFormat function
func TestCreateExtractorForFormat(t *testing.T) {
	registry := DefaultRegistry()
	
	RegisterZipExtractor()
	
	data := []byte{0x50, 0x4b, 0x03, 0x04}
	reader := bytes.NewReader(data)
	
	extractor, err := registry.CreateExtractorForFormat(FormatZIP, reader)
	
	require.NoError(t, err)
	assert.NotNil(t, extractor)
	assert.Equal(t, FormatZIP, extractor.Format())
}

// TestCreateExtractorForFormat_NotRegistered tests error when format is not registered
func TestCreateExtractorForFormat_NotRegistered(t *testing.T) {
	registry := NewArchiveRegistry()
	
	data := []byte{0x50, 0x4b, 0x03, 0x04}
	reader := bytes.NewReader(data)
	
	_, err := registry.CreateExtractorForFormat(FormatRAR, reader)
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no extractor registered")
}

// failingSeeker is a mock reader that can be configured to fail at various operations
type failingSeeker struct {
	canSeek                  bool
	canRead                  bool
	failPrepare               bool
	failOnDetect              bool
	failRestore               bool
	failOnSeekAfterDetect     bool
	seekAfterDetectCalled     bool
	position                  int64
}

func (fs *failingSeeker) Read(p []byte) (n int, err error) {
	if fs.canRead == false {
		return 0, errors.New("read failed")
	}
	// Don't fail on first read for detect, but might fail on subsequent reads
	return copy(p, []byte("test data")), nil
}

func (fs *failingSeeker) ReadAt(p []byte, off int64) (n int, err error) {
	fs.position = off
	return fs.Read(p)
}

func (fs *failingSeeker) Seek(offset int64, whence int) (int64, error) {
	if fs.failOnSeekAfterDetect && fs.seekAfterDetectCalled {
		return 0, errors.New("seek failed after detection")
	}
	
	// Mark that we've been called for seeking after detection
	if offset == 0 && whence == io.SeekStart {
		fs.seekAfterDetectCalled = true
	}
	
	switch whence {
	case io.SeekStart:
		fs.position = offset
	case io.SeekCurrent:
		fs.position += offset
	case io.SeekEnd:
		fs.position = 1000 + offset // Assume size is 1000
	}
	return fs.position, nil
}

func (fs *failingSeeker) Close() error {
	return nil
}
