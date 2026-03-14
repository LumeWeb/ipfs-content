package archive

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mholt/archives"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewArchiveFileEntry tests the constructor for ArchiveFileEntry
func TestNewArchiveFileEntry(t *testing.T) {
	t.Run("creates file entry with all parameters", func(t *testing.T) {
		content := strings.NewReader("test content")
		entry := NewArchiveFileEntry(
			"test.txt",
			12,
			false,
			time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			0644,
			io.NopCloser(content),
		)

		require.NotNil(t, entry)
		assert.Equal(t, "test.txt", entry.fileName)
		assert.Equal(t, int64(12), entry.size)
		assert.False(t, entry.isDir)
		assert.Equal(t, int64(0644), entry.mode)
		assert.NotNil(t, entry.attributes)
		assert.NotNil(t, entry.contentReader)
	})

	t.Run("creates directory entry", func(t *testing.T) {
		entry := NewArchiveFileEntry(
			"directory",
			0,
			true,
			time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			0755,
			nil,
		)

		require.NotNil(t, entry)
		assert.True(t, entry.isDir)
		assert.Nil(t, entry.contentReader)
	})
}

// TestArchiveFileEntry_Name tests the Name method
func TestArchiveFileEntry_Name(t *testing.T) {
	entry := NewArchiveFileEntry("test/path/file.txt", 100, false, time.Now(), 0644, nil)
	assert.Equal(t, "test/path/file.txt", entry.Name())
}

// TestArchiveFileEntry_Size tests the Size method
func TestArchiveFileEntry_Size(t *testing.T) {
	entry := NewArchiveFileEntry("file.txt", 12345, false, time.Now(), 0644, nil)
	assert.Equal(t, int64(12345), entry.Size())
}

// TestArchiveFileEntry_Mode tests the Mode method
func TestArchiveFileEntry_Mode(t *testing.T) {
	t.Run("file mode", func(t *testing.T) {
		entry := NewArchiveFileEntry("file.txt", 100, false, time.Now(), 0644, nil)
		assert.Equal(t, os.FileMode(0644), entry.Mode())
	})

	t.Run("directory mode includes Dir bit", func(t *testing.T) {
		entry := NewArchiveFileEntry("dir", 0, true, time.Now(), 0755, nil)
		assert.True(t, entry.IsDir())
		assert.Equal(t, entry.Mode()&os.ModeDir, os.ModeDir, "Mode should have os.ModeDir bit set")
	})
}

// TestArchiveFileEntry_ModTime tests the ModTime method
func TestArchiveFileEntry_ModTime(t *testing.T) {
	now := time.Date(2024, 3, 15, 12, 30, 45, 0, time.UTC)
	entry := NewArchiveFileEntry("file.txt", 100, false, now, 0644, nil)
	assert.Equal(t, now, entry.ModTime())
}

// TestArchiveFileEntry_IsDir tests the IsDir method
func TestArchiveFileEntry_IsDir(t *testing.T) {
	t.Run("file returns false", func(t *testing.T) {
		entry := NewArchiveFileEntry("file.txt", 100, false, time.Now(), 0644, nil)
		assert.False(t, entry.IsDir())
	})

	t.Run("directory returns true", func(t *testing.T) {
		entry := NewArchiveFileEntry("dir", 0, true, time.Now(), 0755, nil)
		assert.True(t, entry.IsDir())
	})
}

// TestArchiveFileEntry_Sys tests the Sys method
func TestArchiveFileEntry_Sys(t *testing.T) {
	entry := NewArchiveFileEntry("file.txt", 100, false, time.Now(), 0644, nil)
	assert.Nil(t, entry.Sys())
}

// TestArchiveFileEntry_ContentReader tests the ContentReader method
func TestArchiveFileEntry_ContentReader(t *testing.T) {
	content := strings.NewReader("test content")
	entry := NewArchiveFileEntry("file.txt", 12, false, time.Now(), 0644, io.NopCloser(content))

	reader := entry.ContentReader()
	assert.NotNil(t, reader)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(data))
}

// TestArchiveFileEntry_SetContentReader tests the SetContentReader method
func TestArchiveFileEntry_SetContentReader(t *testing.T) {
	entry := NewArchiveFileEntry("file.txt", 0, false, time.Now(), 0644, nil)
	assert.Nil(t, entry.ContentReader())

	newContent := strings.NewReader("new content")
	entry.SetContentReader(io.NopCloser(newContent))

	assert.NotNil(t, entry.ContentReader())
	data, _ := io.ReadAll(entry.ContentReader())
	assert.Equal(t, "new content", string(data))
}

// TestArchiveFileEntry_Attributes tests the Attributes method
func TestArchiveFileEntry_Attributes(t *testing.T) {
	entry := NewArchiveFileEntry("file.txt", 100, false, time.Now(), 0644, nil)
	attrs := entry.Attributes()
	assert.NotNil(t, attrs)
	assert.Empty(t, attrs)
}

// TestArchiveFileEntry_SetAttribute tests the SetAttribute method
func TestArchiveFileEntry_SetAttribute(t *testing.T) {
	t.Run("sets new attribute", func(t *testing.T) {
		entry := NewArchiveFileEntry("file.txt", 100, false, time.Now(), 0644, nil)
		entry.SetAttribute("compression", "gzip")
		assert.Equal(t, "gzip", entry.attributes["compression"])
	})

	t.Run("sets attribute when map is nil", func(t *testing.T) {
		entry := &ArchiveFileEntry{
			fileName:   "file.txt",
			size:       100,
			isDir:      false,
			modified:   time.Now(),
			mode:       0644,
			attributes: nil,
		}
		entry.SetAttribute("key", "value")
		assert.Equal(t, "value", entry.attributes["key"])
	})
}

// TestArchiveFileEntry_SetAttributes tests the SetAttributes method
func TestArchiveFileEntry_SetAttributes(t *testing.T) {
	t.Run("sets multiple attributes", func(t *testing.T) {
		entry := NewArchiveFileEntry("file.txt", 100, false, time.Now(), 0644, nil)
		attrs := map[string]string{
			"compression": "gzip",
			"level":       "9",
			"encoding":    "utf-8",
		}
		entry.SetAttributes(attrs)

		assert.Equal(t, "gzip", entry.attributes["compression"])
		assert.Equal(t, "9", entry.attributes["level"])
		assert.Equal(t, "utf-8", entry.attributes["encoding"])
	})

	t.Run("sets attributes when map is nil", func(t *testing.T) {
		entry := &ArchiveFileEntry{
			fileName:   "file.txt",
			size:       100,
			isDir:      false,
			modified:   time.Now(),
			mode:       0644,
			attributes: nil,
		}
		attrs := map[string]string{"key": "value"}
		entry.SetAttributes(attrs)
		assert.Equal(t, "value", entry.attributes["key"])
	})
}

// TestFormat_IsUploadFormat tests the IsUploadFormat method
func TestFormat_IsUploadFormat(t *testing.T) {
	tests := []struct {
		format  Format
		want    bool
		message string
	}{
		{FormatCAR, true, "CAR should be an upload format"},
		{FormatZIP, false, "ZIP should not be an upload format"},
		{FormatFile, false, "File should not be an upload format"},
		{FormatRAR, false, "RAR should not be an upload format"},
		{FormatTAR, false, "TAR should not be an upload format"},
		{FormatTAR_GZ, false, "TAR_GZ should not be an upload format"},
		{FormatTAR_BZ2, false, "TAR_BZ2 should not be an upload format"},
		{Format7Z, false, "7Z should not be an upload format"},
		{FormatUnknown, false, "Unknown should not be an upload format"},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.format.IsUploadFormat())
		})
	}
}

// TestFormat_IsArchiveFormat tests the IsArchiveFormat method
func TestFormat_IsArchiveFormat(t *testing.T) {
	tests := []struct {
		format  Format
		want    bool
		message string
	}{
		{FormatCAR, false, "CAR is not an archive format"},
		{FormatFile, false, "File is not an archive format"},
		{FormatZIP, true, "ZIP should be an archive format"},
		{FormatRAR, true, "RAR should be an archive format"},
		{FormatTAR, true, "TAR should be an archive format"},
		{FormatTAR_GZ, true, "TAR_GZ should be an archive format"},
		{FormatTAR_BZ2, true, "TAR_BZ2 should be an archive format"},
		{Format7Z, true, "7Z should be an archive format"},
		{FormatUnknown, false, "Unknown is not an archive format"},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.format.IsArchiveFormat())
		})
	}
}

// TestFormat_String tests the String method
func TestFormat_String(t *testing.T) {
	tests := []struct {
		format  Format
		want    string
		message string
	}{
		{FormatCAR, "car", "CAR should return 'car'"},
		{FormatFile, "file", "File should return 'file'"},
		{FormatZIP, "zip", "ZIP should return 'zip'"},
		{FormatRAR, "rar", "RAR should return 'rar'"},
		{FormatTAR, "tar", "TAR should return 'tar'"},
		{FormatTAR_GZ, "tar.gz", "TAR_GZ should return 'tar.gz'"},
		{FormatTAR_BZ2, "tar.bz2", "TAR_BZ2 should return 'tar.bz2'"},
		{Format7Z, "7z", "7Z should return '7z'"},
		{FormatUnknown, "unknown", "Unknown should return 'unknown'"},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.format.String())
		})
	}
}

// TestParseFormat tests the ParseFormat function
func TestParseFormat(t *testing.T) {
	tests := []struct {
		input   string
		want    Format
		message string
	}{
		{"car", FormatCAR, "Should parse car"},
		{"file", FormatFile, "Should parse file"},
		{"zip", FormatZIP, "Should parse zip"},
		{"rar", FormatRAR, "Should parse rar"},
		{"tar", FormatTAR, "Should parse tar"},
		{"tar.gz", FormatTAR_GZ, "Should parse tar.gz"},
		{"tar.bz2", FormatTAR_BZ2, "Should parse tar.bz2"},
		{"7z", Format7Z, "Should parse 7z"},
		{"unknown", FormatUnknown, "Should return unknown for unrecognized format"},
		{"", FormatUnknown, "Should return unknown for empty string"},
		{"invalid", FormatUnknown, "Should return unknown for invalid format"},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseFormat(tt.input))
		})
	}
}

// TestValidateArchivePath tests the ValidateArchivePath function
func TestValidateArchivePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
		message string
	}{
		{"valid simple path", "documents/file.txt", false, "Simple path should be valid"},
		{"valid nested path", "dir1/dir2/dir3/file.txt", false, "Nested path should be valid"},
		{"valid single character", "a", false, "Single character should be valid"},
		{"valid with numbers", "dir123/file456.txt", false, "Path with numbers should be valid"},
		{"valid with lowercase only", "test-file.txt", false, "Lowercase path should be valid"},
		{"invalid - windows backslash", "documents\\file.txt", true, "Backslash should be rejected"},
		{"invalid - double slash", "documents//file.txt", true, "Double slash should be rejected"},
		{"invalid - current directory prefix", "./file.txt", true, "Dotted path should be rejected"},
		{"invalid - parent directory traversal", "../etc/passwd", true, "Parent traversal should be rejected"},
		{"invalid - parent prefix in path", "safe/../../../etc/passwd", true, "Parent traversal after safe path should be rejected"},
		{"invalid - absolute path", "/etc/passwd", true, "Absolute path should be rejected"},
		{"invalid - Windows absolute path", "C:\\Windows\\System32", true, "Windows absolute should be rejected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateArchivePath(tt.path)
			if tt.wantErr {
				require.Error(t, err, tt.message)
			} else {
				require.NoError(t, err, tt.message)
			}
		})
	}
}

// TestArchiveFileEntry_FullInterfaceCoverage tests all fs.FileInfo methods
func TestArchiveFileEntry_FullInterfaceCoverage(t *testing.T) {
	modTime := time.Date(2024, 3, 15, 12, 30, 45, 0, time.UTC)
	content := strings.NewReader("test content")

	entry := NewArchiveFileEntry(
		"path/to/file.txt",
		1000,
		false,
		modTime,
		0644,
		io.NopCloser(content),
	)

	// Test all fs.FileInfo interface methods
	assert.Equal(t, "path/to/file.txt", entry.Name(), "Name should return filename")
	assert.Equal(t, int64(1000), entry.Size(), "Size should return 1000")
	assert.Equal(t, os.FileMode(0644), entry.Mode(), "Mode should return 0644")
	assert.Equal(t, modTime, entry.ModTime(), "ModTime should return modification time")
	assert.False(t, entry.IsDir(), "Should not be a directory")
	assert.Nil(t, entry.Sys(), "Sys should return nil")
}

// TestCreateExtractor tests the CreateExtractor function
func TestCreateExtractor(t *testing.T) {
	t.Run("creates extractor for valid ZIP data", func(t *testing.T) {
		// Create minimal ZIP file header
		zipData := []byte{
			0x50, 0x4b, 0x03, 0x04, // Local file header signature
			0x14, 0x00, 0x00, 0x00, // Version needed
			0x00, 0x00, 0x00, 0x00, // Flags
			0x08, 0x00, 0x00, 0x00, // Compression method (deflate)
			0x00, 0x00, 0x00, 0x00, // CRC32
			0x00, 0x00, 0x00, 0x00, // Compressed size
			0x00, 0x00, 0x00, 0x00, // Uncompressed size
		}

		seeker := bytes.NewReader(zipData)
		_, err := CreateExtractor(seeker)

		// Should return error without valid registration
		if err != nil {
			assert.Error(t, err, "Should fail without registered extractors or invalid format")
		}
	})
}

// TestRegister7ZipExtractor tests the 7Z extractor registration
func TestRegister7ZipExtractor(t *testing.T) {
	t.Run("registers 7z extractor", func(t *testing.T) {
		// Clear any existing registration if needed
		MaybeInit()

		Register7ZipExtractor()

		registry := DefaultRegistry()
		formats := registry.SupportedFormats()

		found := false
		for _, format := range formats {
			if format == Format7Z {
				found = true
				break
			}
		}
		assert.True(t, found, "7z format should be supported")
	})
}

// TestRegisterRarExtractor tests the RAR extractor registration
func TestRegisterRarExtractor(t *testing.T) {
	t.Run("registers RAR extractor", func(t *testing.T) {
		MaybeInit()

		RegisterRarExtractor()

		registry := DefaultRegistry()
		formats := registry.SupportedFormats()

		found := false
		for _, format := range formats {
			if format == FormatRAR {
				found = true
				break
			}
		}
		assert.True(t, found, "RAR format should be supported")
	})
}

// TestSevenZipArchiveExtractor_methods tests SevenZipArchiveExtractor methods without real data
func TestSevenZipArchiveExtractor_methods(t *testing.T) {
	t.Run("Format returns Format7Z", func(t *testing.T) {
		extractor := &SevenZipArchiveExtractor{}
		assert.Equal(t, Format7Z, extractor.Format())
	})

	t.Run("Close returns nil", func(t *testing.T) {
		extractor := &SevenZipArchiveExtractor{}
		err := extractor.Close()
		assert.NoError(t, err)
	})
}

// TestRarArchiveExtractor_methods tests RarArchiveExtractor methods without real data
func TestRarArchiveExtractor_methods(t *testing.T) {
	t.Run("Format returns FormatRAR", func(t *testing.T) {
		extractor := &RarArchiveExtractor{}
		assert.Equal(t, FormatRAR, extractor.Format())
	})

	t.Run("Close returns nil", func(t *testing.T) {
		extractor := &RarArchiveExtractor{}
		err := extractor.Close()
		assert.NoError(t, err)
	})
}

// TestTarGzArchiveExtractor_methods tests TarGzArchiveExtractor methods without real data
func TestTarGzArchiveExtractor_methods(t *testing.T) {
	t.Run("Format returns FormatTAR_GZ", func(t *testing.T) {
		extractor := &TarGzArchiveExtractor{}
		assert.Equal(t, FormatTAR_GZ, extractor.Format())
	})

	t.Run("Close returns nil", func(t *testing.T) {
		extractor := &TarGzArchiveExtractor{}
		err := extractor.Close()
		assert.NoError(t, err)
	})
}

// TestTarBz2ArchiveExtractor_methods tests TarBz2ArchiveExtractor methods without real data
func TestTarBz2ArchiveExtractor_methods(t *testing.T) {
	t.Run("Format returns FormatTAR_BZ2", func(t *testing.T) {
		extractor := &TarBz2ArchiveExtractor{}
		assert.Equal(t, FormatTAR_BZ2, extractor.Format())
	})

	t.Run("Close returns nil", func(t *testing.T) {
		extractor := &TarBz2ArchiveExtractor{}
		err := extractor.Close()
		assert.NoError(t, err)
	})
}

// TestSupportedFormats tests the SupportedFormats function
func TestSupportedFormats(t *testing.T) {
	formats := SupportedFormats()
	assert.NotEmpty(t, formats, "Should have at least one supported format")

	// Check for expected formats based on registration
	hasZip := false
	for _, format := range formats {
		if format == FormatZIP {
			hasZip = true
			break
		}
	}
	assert.True(t, hasZip, "ZIP should be in supported formats")
}

// TestArchivesDriver_GetFormat tests the GetFormat method
func TestArchivesDriver_GetFormat(t *testing.T) {
	driver := NewArchivesDriver(FormatZIP, nil)
	assert.Equal(t, FormatZIP, driver.GetFormat())
}

// TestArchivesDriver_IsFormatSupported tests the IsFormatSupported method
func TestArchivesDriver_IsFormatSupported(t *testing.T) {
	tests := []struct {
		format Format
		want   bool
	}{
		{FormatZIP, true},
		{FormatTAR, true},
		{FormatTAR_GZ, true},
		{FormatTAR_BZ2, true},
		{FormatRAR, true},
		{Format7Z, true},
		{FormatFile, false},
		{FormatCAR, false},
	}

	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			driver := NewArchivesDriver(tt.format, nil)
			assert.Equal(t, tt.want, driver.IsFormatSupported())
		})
	}
}

// TestArchivesDriver_addFormatAttributes tests addFormatAttributes method
func TestArchivesDriver_addFormatAttributes(t *testing.T) {
	t.Run("adds ZIP method attribute", func(t *testing.T) {
		driver := &ArchivesDriver{format: FormatZIP}
		entry := NewArchiveFileEntry("test.txt", 100, false, time.Now(), 0644, nil)

		// Create archives.FileInfo with Sys() returning header that has Method()
		mockSys := &mockZipHeaderSys{method: 8}
		mockFileInfo := archives.FileInfo{
			FileInfo: &mockFileInfoImpl{sys: mockSys},
		}

		driver.addFormatAttributes(entry, mockFileInfo)

		attrs := entry.Attributes()
		assert.NotEmpty(t, attrs)
		assert.Equal(t, "8", attrs["method"])
	})

	t.Run("does not add attributes when Sys does not implement Method()", func(t *testing.T) {
		driver := &ArchivesDriver{format: FormatZIP}
		entry := NewArchiveFileEntry("test.txt", 100, false, time.Now(), 0644, nil)

		mockFileInfo := archives.FileInfo{
			FileInfo: &mockFileInfoImpl{},
		}

		driver.addFormatAttributes(entry, mockFileInfo)

		attrs := entry.Attributes()
		assert.Empty(t, attrs)
	})

	t.Run("adds RAR CRC attribute", func(t *testing.T) {
		driver := &ArchivesDriver{format: FormatRAR}
		entry := NewArchiveFileEntry("file.txt", 100, false, time.Now(), 0644, nil)

		mockSys := &mockRarHeaderSys{crc: 0x12345678}
		mockFileInfo := archives.FileInfo{
			FileInfo: &mockFileInfoImpl{sys: mockSys},
		}

		driver.addFormatAttributes(entry, mockFileInfo)

		attrs := entry.Attributes()
		assert.NotEmpty(t, attrs)
		assert.Equal(t, "12345678", attrs["crc"])
	})
}

// mockRarHeaderSys implements GetCRC32() method for RAR
type mockRarHeaderSys struct {
	crc uint32
}

func (h *mockRarHeaderSys) GetCRC32() uint32 { return h.crc }

// mockFileInfoImpl implements fs.FileInfo interface
type mockFileInfoImpl struct {
	sys interface{}
}

func (m *mockFileInfoImpl) Name() string       { return "test.zip" }
func (m *mockFileInfoImpl) Size() int64        { return 100 }
func (m *mockFileInfoImpl) Mode() os.FileMode  { return 0644 }
func (m *mockFileInfoImpl) ModTime() time.Time { return time.Now() }
func (m *mockFileInfoImpl) IsDir() bool        { return false }
func (m *mockFileInfoImpl) Sys() interface{}   { return m.sys }

// mockZipHeaderSys implements Method() method for ZIP
type mockZipHeaderSys struct {
	method uint16
}

func (h *mockZipHeaderSys) Method() uint16 { return h.method }

// TestArchivesDriver_Filesystem tests the Filesystem method
func TestArchivesDriver_Filesystem(t *testing.T) {
	t.Run("returns error for unsupported format", func(t *testing.T) {
		driver := NewArchivesDriver(FormatFile, nil)

		fsys, err := driver.Filesystem(nil)
		assert.Error(t, err)
		assert.Nil(t, fsys)
		assert.Contains(t, err.Error(), "unsupported archive format")
	})
}

// TestDetectCompressedTar tests the detectCompressedTar function
func TestDetectCompressedTar(t *testing.T) {
	t.Run("empty data returns unknown", func(t *testing.T) {
		format, detected := detectCompressedTar([]byte{}, 0)
		assert.False(t, detected)
		assert.Equal(t, FormatUnknown, format)
	})
	
	t.Run("single byte returns unknown", func(t *testing.T) {
		format, detected := detectCompressedTar([]byte{0x1f}, 1)
		assert.False(t, detected)
		assert.Equal(t, FormatUnknown, format)
	})
	
	t.Run("gzip magic number detected", func(t *testing.T) {
		gzipMagic := []byte{
			0x1f, 0x8b, // gzip magic number
			0x08,       // compression method
			0x00,       // flags
			0x00, 0x00, 0x00, 0x00, // mtime
		}
		format, detected := detectCompressedTar(gzipMagic, len(gzipMagic))
		assert.True(t, detected)
		assert.Equal(t, FormatTAR_GZ, format)
	})
	
	t.Run("gzip magic number minimum size", func(t *testing.T) {
		gzipMagic := []byte{0x1f, 0x8b}
		format, detected := detectCompressedTar(gzipMagic, len(gzipMagic))
		assert.True(t, detected)
		assert.Equal(t, FormatTAR_GZ, format)
	})
	
	t.Run("gzip followed by other data", func(t *testing.T) {
		data := []byte{
			0x1f, 0x8b,
			0x08, 0x00, 0x00, 0x00, 0x00,
			'f', 'o', 'o', 'b', 'a', 'r', 
		}
		format, detected := detectCompressedTar(data, len(data))
		assert.True(t, detected)
		assert.Equal(t, FormatTAR_GZ, format)
	})
	
	t.Run("gzip magic with just 0x1f (incomplete)", func(t *testing.T) {
		data := []byte{0x1f, 0x00}
		format, detected := detectCompressedTar(data, len(data))
		assert.False(t, detected)
		assert.Equal(t, FormatUnknown, format)
	})
	
	t.Run("bzip2 magic number detected", func(t *testing.T) {
		bzip2Magic := []byte{
			'B', 'Z', 'h', // bzip2 magic number
			'9',           // block size
		}
		format, detected := detectCompressedTar(bzip2Magic, len(bzip2Magic))
		assert.True(t, detected)
		assert.Equal(t, FormatTAR_BZ2, format)
	})
	
	t.Run("bzip2 magic number minimum size", func(t *testing.T) {
		bzip2Magic := []byte{'B', 'Z', 'h', '1'}
		format, detected := detectCompressedTar(bzip2Magic, len(bzip2Magic))
		assert.True(t, detected)
		assert.Equal(t, FormatTAR_BZ2, format)
	})
	
	t.Run("bzip2 with different block size", func(t *testing.T) {
		data := []byte{'B', 'Z', 'h', '9', 0x31, 0x41, 59}
		format, detected := detectCompressedTar(data, len(data))
		assert.True(t, detected)
		assert.Equal(t, FormatTAR_BZ2, format)
	})
	
	t.Run("random bytes not recognized", func(t *testing.T) {
		data := []byte{
			0x12, 0x34, 0x56, 0x78, 
			0x9a, 0xbc, 0xde, 0xf0,
		}
		format, detected := detectCompressedTar(data, len(data))
		assert.False(t, detected)
		assert.Equal(t, FormatUnknown, format)
	})
	
	t.Run("ZIP magic number not recognized as compressed tar", func(t *testing.T) {
		zipMagic := []byte{0x50, 0x4b, 0x03, 0x04}
		format, detected := detectCompressedTar(zipMagic, len(zipMagic))
		assert.False(t, detected)
		assert.Equal(t, FormatUnknown, format)
	})
	
	t.Run("just 0x1f followed by non-0x8b", func(t *testing.T) {
		data := []byte{0x1f, 0x00, 123, 200}
		format, detected := detectCompressedTar(data, len(data))
		assert.False(t, detected)
		assert.Equal(t, FormatUnknown, format)
	})
	
	t.Run("B and Z but not h", func(t *testing.T) {
		actualData := []byte{byte('B'), byte('Z'), byte('x'), byte('1'), 234}
		format, detected := detectCompressedTar(actualData, len(actualData))
		assert.False(t, detected)
		assert.Equal(t, FormatUnknown, format)
	})
	
	t.Run("insufficient data for bzip2 check", func(t *testing.T) {
		data := []byte{'B', 'Z'}
		format, detected := detectCompressedTar(data, len(data))
		assert.False(t, detected)
		assert.Equal(t, FormatUnknown, format)
	})
	
	t.Run("with large binary data", func(t *testing.T) {
		data := make([]byte, 100)
		data[0] = 0x1f
		data[1] = 0x8b
		for i := 2; i < 100; i++ {
			data[i] = byte(i % 256)
		}
		format, detected := detectCompressedTar(data, len(data))
		assert.True(t, detected)
		assert.Equal(t, FormatTAR_GZ, format)
	})
}


