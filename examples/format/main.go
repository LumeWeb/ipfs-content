// Package main demonstrates the Format type system.
//
// This example shows how to use the Format enum to identify and work with
// different file formats across the library.
package main

import (
	"fmt"

	"go.lumeweb.com/ipfs-content/format"
)

func main() {
	// Example 1: Format identification
	fmt.Println("Example 1: Format Identification")
	formats := []format.Format{
		format.FormatUnknown,
		format.FormatCAR,
		format.FormatZIP,
		format.FormatTAR,
		format.FormatRAR,
		format.Format7Z,
		format.FormatTAR_GZ,
		format.FormatTAR_BZ2,
		format.FormatFile,
	}

	for _, f := range formats {
		fmt.Printf("  %s: Upload=%v, Archive=%v\n", f, f.IsUploadFormat(), f.IsArchiveFormat())
	}

	// Example 2: Format checking
	fmt.Println("\nExample 2: Format Checking")

	myFormat := format.FormatZIP
	fmt.Printf("My format: %s\n", myFormat)

	if myFormat.IsUploadFormat() {
		fmt.Println("  This format is supported for direct upload")
	} else {
		fmt.Println("  This format is NOT supported for direct upload")
	}

	if myFormat.IsArchiveFormat() {
		fmt.Println("  This is an archive format (can be extracted)")
	} else {
		fmt.Println("  This is NOT an archive format")
	}

	// Example 3: Format parsing
	fmt.Println("\nExample 3: Format Parsing")
	formatStrings := []string{"car", "zip", "tar.gz", "rar", "7z", "unknown"}

	for _, s := range formatStrings {
		f := format.ParseFormat(s)
		fmt.Printf("  %s -> %s\n", s, f)
	}

	// Example 4: Format comparison
	fmt.Println("\nExample 4: Format Comparison")
	cidFormat := format.ParseFormat("CAR")
	unknownFormat := format.ParseFormat("unknown")

	if cidFormat == format.FormatCAR {
		fmt.Println("  Correctly identified CAR format")
	}

	if unknownFormat == format.FormatUnknown {
		fmt.Println("  Correctly identified unknown format")
	}

	fmt.Println("\nDone!")
}
