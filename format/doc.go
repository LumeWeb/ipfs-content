// Package format provides a unified file format type system.
//
// This package defines a Format enum for classifying different file types
// across the library, including upload formats (CAR) and archive formats
// (ZIP, TAR, TAR.GZ, TAR.BZ2, RAR, 7Z).
//
// The Format type provides methods to determine if a format is suitable for
// direct upload (IsUploadFormat) or is an archive that needs extraction
// (IsArchiveFormat).
//
// Basic Usage:
//
//	import "go.lumeweb.com/ipfs-content/format"
//
//	f := format.FormatZIP
//	if f.IsArchiveFormat() {
//	    // Extract archive contents
//	}
//
//	if f.IsUploadFormat() {
//	    // Ready for IPFS upload
//	}
//
// Supported Formats:
//   - FormatUnknown: Unknown or unrecognized format
//   - FormatCAR: IPFS Content Addressable Archive
//   - FormatFile: Regular single file
//   - FormatZIP, FormatRAR, FormatTAR, FormatTAR_GZ, FormatTAR_BZ2, Format7Z: Archive formats
package format
