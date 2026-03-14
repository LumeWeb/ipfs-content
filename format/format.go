package format

import (
	"fmt"
)

// Format represents a unified file format type used across the library
// This provides a common type system for both archive detection and CAR streaming
type Format int

const (
	// FormatUnknown represents an unknown or unrecognized format
	FormatUnknown Format = iota

	// FormatCAR represents an IPFS Content Addressable Archive file
	FormatCAR

	// FormatZIP represents a zip archive
	FormatZIP

	// FormatFile represents a regular single file (not an archive container)
	FormatFile

	// FormatRAR represents a RAR archive
	FormatRAR

	// FormatTAR represents a tar archive
	FormatTAR

	// FormatTAR_GZ represents a gzip-compressed tar archive
	FormatTAR_GZ

	// FormatTAR_BZ2 represents a bzip2-compressed tar archive
	FormatTAR_BZ2

	// Format7Z represents a 7z archive
	Format7Z
)

// IsUploadFormat returns true if this format is supported for direct upload
func (f Format) IsUploadFormat() bool {
	return f == FormatCAR
}

// IsArchiveFormat returns true if this format is an archive container that can be extracted
// Note: FormatFile is a single file, not an archive container
// FormatCAR is a CAR file that doesn't need extraction in the traditional sense
func (f Format) IsArchiveFormat() bool {
	return f != FormatUnknown && f != FormatFile && f != FormatCAR
}

// String returns the human-readable string representation of Format
func (f Format) String() string {
	switch f {
	case FormatCAR:
		return "car"
	case FormatFile:
		return "file"
	case FormatZIP:
		return "zip"
	case FormatRAR:
		return "rar"
	case FormatTAR:
		return "tar"
	case FormatTAR_GZ:
		return "tar.gz"
	case FormatTAR_BZ2:
		return "tar.bz2"
	case Format7Z:
		return "7z"
	default:
		return "unknown"
	}
}

// ParseFormat parses a string into a Format
// Returns FormatUnknown if the string is not a recognized format
func ParseFormat(s string) Format {
	switch s {
	case "car":
		return FormatCAR
	case "file":
		return FormatFile
	case "zip":
		return FormatZIP
	case "rar":
		return FormatRAR
	case "tar":
		return FormatTAR
	case "tar.gz":
		return FormatTAR_GZ
	case "tar.bz2":
		return FormatTAR_BZ2
	case "7z":
		return Format7Z
	default:
		return FormatUnknown
	}
}

// MarshalText implements encoding.TextMarshaler for Format
func (f Format) MarshalText() ([]byte, error) {
	return []byte(f.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler for Format
func (f *Format) UnmarshalText(text []byte) error {
	*f = ParseFormat(string(text))
	if *f == FormatUnknown {
		return fmt.Errorf("unknown format: %s", string(text))
	}
	return nil
}
