// Package car provides CAR (Content Addressable Archive) file streaming.
//
// This package implements two-pass CAR file generation for memory efficiency:
//   - Pass 1: Walk filesystem and build TreeSummary with metadata (block CIDs, sizes, tree structure)
//   - Pass 2: Write CARv1 using the summary, regenerating blocks on demand
//
// This approach allows pre-calculating the CAR size without storing all blocks
// in memory, which is critical for TUS uploads where size must be known upfront.
//
// The package uses an LRU blockstore to limit memory usage with configurable limits.
// For large file trees, memory usage is bounded regardless of the total size.
//
// Basic Usage:
//
//	import "go.lumeweb.com/ipfs-content/car"
//
//	// Simple CAR streaming
//	rootCID, err := car.StreamCAR(ctx, os.DirFS("./content"), writer, 100*1024*1024, true)
//
//	// With size calculation for TUS uploads
//	rootCID, carSize, err := car.StreamCARWithSize(ctx, os.DirFS("./content"), writer, 100*1024*1024, true)
//	fmt.Printf("Root CID: %s, CAR Size: %d\n", rootCID, carSize)
package car
