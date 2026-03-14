// Package main demonstrates archive detection and extraction.
//
// This example shows how to detect an archive format, create an extractor,
// and browse the archive contents as a filesystem.
package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"

	"go.lumeweb.com/ipfs-content/archive"
)

func main() {
	ctx := context.Background()

	// Open archive file
	file, err := os.Open("example.zip")
	if err != nil {
		log.Fatalf("Failed to open archive: %v", err)
	}
	defer file.Close()

	// Detect archive format
	archiveFormat, err := archive.DetectFormat(file)
	if err != nil {
		log.Fatalf("Failed to detect format: %v", err)
	}
	fmt.Printf("Detected format: %s\n", archiveFormat)

	// Reset file pointer for reading
	if _, err := file.Seek(0, 0); err != nil {
		log.Fatalf("Failed to seek file: %v", err)
	}

	// Create extractor
	extractor, err := archive.CreateExtractor(file)
	if err != nil {
		log.Fatalf("Failed to create extractor: %v", err)
	}
	defer extractor.Close()

	// Browse archive contents
	fsys, err := extractor.Filesystem(ctx)
	if err != nil {
		log.Fatalf("Failed to get filesystem: %v", err)
	}

	// List files in the archive
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		log.Fatalf("Failed to read directory: %v", err)
	}

	fmt.Println("\nArchive contents:")
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if entry.IsDir() {
			fmt.Printf("  %s/ (directory)\n", entry.Name())
		} else {
			fmt.Printf("  %s (%d bytes)\n", entry.Name(), info.Size())
		}
	}
}
