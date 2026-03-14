// Package archive provides archive format detection and extraction.
//
// This package supports multiple archive formats including ZIP, TAR, TAR.GZ,
// TAR.BZ2, RAR, and 7Z, with automatic format detection and streaming extraction.
//
// The package uses a registry pattern for format detection and extractor creation,
// making it extensible for new archive formats. All archive paths are validated
// to prevent path traversal attacks (zip-slip).
//
// Basic Usage:
//
//	import "go.lumeweb.com/ipfs-content/archive"
//
//	// Detect archive format
//	format, err := archive.DetectFormat(file)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Create extractor
//	extractor, err := archive.CreateExtractor(file)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer extractor.Close()
//
//	// Browse archive as filesystem
//	fsys, err := extractor.Filesystem(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// The ArchiveFileEntry interface provides streaming access to individual files
// within an archive through ContentReader(), allowing efficient processing without
// loading entire contents into memory.
package archive
