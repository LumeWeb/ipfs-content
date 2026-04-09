package car

import (
	"context"
	"fmt"
	"io"

	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/exchange/offline"
	"github.com/ipfs/boxo/ipld/merkledag"
	carv2 "github.com/ipld/go-car/v2"

	"go.lumeweb.com/ipfs-content/blockstore"
	internalio "go.lumeweb.com/ipfs-content/internal/io"
)

// ReadCAR reads a CAR file and reconstructs the directory hierarchy tree structure.
//
// This function performs CAR file parsing and tree reconstruction to build a TreeSummary
// that matches the structure created by BuildSummary, enabling lossless round-trip
// operations: BuildSummary → WriteCAR → ReadCAR → identical TreeSummary.
//
// **Reader Requirements:**
//
// The reader must implement `io.ReadSeeker` (not just io.Reader) because:
//
//  1. CAR files are indexed with block positions via `IndexAll()` which requires
//     random access for efficient block seeking and regeneration
//  2. When the LRU cache evicts blocks, they must be re-read from their original
//     positions in the CAR file
//  3. Reading the CAR header initially requires seeking back to the start after
//     indexing
//
// Note: If you have an io.Reader that doesn't support seeking, first copy it to
// a file or buffer that supports random access.
//
// **Memory Boundedness:**
//
// Memory usage is bounded by the `memoryLimit` parameter through the IndexedBlockstore
// LRU cache:
//
//   - Block data is loaded from CAR into an LRU cache bounded by `memoryLimit`
//   - When the cache fills, least-recently-used blocks are evicted
//   - Evicted blocks can be re-read on-demand using indexed positions
//   - This enables reading arbitrarily large CAR files with fixed memory bounds
//
// Recommended:
//   - For small CARs: 10-100 MB (faster loading)
//   - For large CARs: 100-1000 MB (good balance)
//   - For very large CARs: Minimum to hold frequently-accessed blocks
//
// **Reconstruction Process:**
//
// The reconstruction follows these steps:
//
//  1. **Indexing:** IndexedBlockstore scans the entire CAR file, recording the
//     position (byte offset) and size of each block. This enables O(1) seeking
//     to any block.
//
//  2. **Header Reading:** The CARv1 header is read to extract the root CID. This
//     requires seeking back to the start of the file after indexing.
//
//  3. **DAG Service Setup:** A blockservice is created from the indexed blockstore
//     with offline exchange (no P2P fetches). The merkledag DAGService provides
//     traversal and node access.
//
//  4. **Tree Reconstruction:** `reconstructTree()` walks the DAG starting from the
//     root CID, building TreeSummary entries and establishing parent-child
//     relationships. Path synthesis occurs during traversals since CAR files
//     don't store full paths (only immediate child names).
//
//  5. **Block Metadata Collection:** `collectBlockMetadata()` performs a BFS
//     traversal to collect all block CIDs, sizes, and order for deterministic
//     CAR reconstruction.
//
// **Round-Trip Compatibility:**
//
// This function is designed to produce TreeSummary objects identical to those
// created by BuildSummary, with these critical compatibility rules:
//
//   - **ROOT.Children = Files Only:** ROOT contains only root-level files in its
//     Children array, matching the builder's behavior. Root-level directories
//     exist in TreeEntries but are NOT in ROOT.Children.
//
//   - **Path Synthesis:** Paths are synthesized during DAG traversal using the
//     pattern: `entryKey = (parentPath == ROOT) ? nodeName : parentPath + "/" + nodeName`.
//     This produces identical path keys to BuildSummary.
//
//   - **Virtual ROOT:** ROOT entry is created as a virtual directory wrapper
//     matching the builder's structure, with Path="" (or "ROOT" in builder,
//     though reconstruction uses "" for consistency).
//
// **Root Type Handling:**
//
// The function handles two distinct CAR structures:
//
//  1. **Root is Directory (wrapInDir=true):**
//     - Root CID points to a UnixFS directory node
//     - ROOT entry created with IsDir=true, Path="", CID=rootCID
//     - Root-level files added to ROOT.Children
//     - Root-level directories NOT added to ROOT.Children (but in TreeEntries)
//     - Default behavior from StreamCAR with wrapInDir=true
//
//  2. **Root is File (wrapInDir=false):**
//     - Root CID points directly to a UnixFS file node
//     - ROOT wrapper created around the file entry
//     - Filename is derived from root CID (original filename not in CAR)
//     - ROOT.Children contains the single file path
//     - Optimization from StreamCAR with wrapInDir=false for single files
//
// **Path Synthesis Details:**
//
// Since CAR files don't store full hierarchical paths, paths are synthesized
// during traversal by joining parent paths with child names:
//
//   - ROOT children: `entryKey = nodeName` (e.g., "file.txt")
//   - Nested children: `entryKey = parentPath + "/" + nodeName` (e.g., "dir1/file.txt")
//   - Deeply nested: `entryKey = "dir1/subdir1/file2.txt"`
//
// This mechanism ensures that reconstructed paths exactly match the paths used
// during BuildSummary, enabling perfect round-trip compatibility.
//
// **Cycle Detection:**
//
// DAG traversal includes cycle detection via the CIDToEntry map to handle
// malformed or corrupted CARs that might contain circular references. If a
// CID has already been processed, it's skipped to prevent infinite loops.
//
// **Error Handling:**
//
// Errors are wrapped with context to aid debugging:
//
//   - "index CAR blocks" - CAR parsing or indexing error
//   - "seek to start for header" - Reader doesn't support seeking
//   - "read CAR header" - Invalid CAR format or missing header
//   - "CAR has no root blocks" - CAR header missing roots array
//   - "reconstruct tree" - DAG traversal or node analysis error
//
// **Example Usage:**
//
// Read a CAR file and inspect the reconstructed tree:
//
//	file, err := os.Open("archive.car")
//	if err != nil {
//	    return err
//	}
//	defer file.Close()
//
//	summary, err := car.ReadCAR(file, 100*1024*1024) // 100MB memory limit
//	if err != nil {
//	    return err
//	}
//
//	// Access root information
//	fmt.Println("Root CID:", summary.RootCID)
//
//	// Iterate through tree entries
//	for path, entry := range summary.TreeEntries {
//	    if entry.IsDir {
//	        fmt.Printf("Directory: %s (CID: %s)\n", path, entry.CID)
//	    } else {
//	        fmt.Printf("File: %s (CID: %s, Size: %d)\n", path, entry.CID, entry.LogicalFileSize)
//	    }
//	}
//
// **Performance Characteristics:**
//
//   - **Time:** O(N) indexing where N is CAR file size, plus O(V) traversal where V
//     is the number of blocks (visited blocks once each)
//   - **Memory:** Bounded by `memoryLimit` parameter (typically 100MB) regardless of
//     CAR size
//   - **I/O:** Initial full CAR scan for indexing, then random access for blocks
//
// **Parameters:**
//
//   - r: io.ReadSeeker - CAR file reader (must support seeking)
//   - memoryLimit: uint64 - Maximum bytes to cache in LRU memory
//
// **Returns:**
//
//   - *TreeSummary: Reconstructed tree structure with directory hierarchy,
//     block metadata, and file information
//   - error: Error if CAR is invalid, reader doesn't support seeking, or
//     reconstruction fails
func ReadCAR(ctx context.Context, r io.ReadSeeker, memoryLimit uint64) (*TreeSummary, error) {
	// Create indexed blockstore wrapper
	ibs := blockstore.NewIndexedBlockstore(memoryLimit, r)
	defer ibs.Close()

	// Index all blocks and load into LRU cache
	if err := ibs.IndexAll(ctx); err != nil {
		return nil, fmt.Errorf("index CAR blocks: %w", err)
	}

	// Get root CID from CAR header
	// Seek back to start to read header
	_, err := r.Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("seek to start for header: %w", err)
	}

	// Use carv2.NewReader to support both CARv1 and CARv2
	cr, err := carv2.NewReader(internalio.ToReaderAt(r))
	if err != nil {
		return nil, fmt.Errorf("read CAR header: %w", err)
	}
	roots, err := cr.Roots()
	if err != nil {
		return nil, fmt.Errorf("get root CID: %w", err)
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("CAR has no root blocks")
	}
	rootCID := roots[0]

	// Create DAG service from blockstore
	bsvc := blockservice.New(ibs, offline.Exchange(ibs))
	dagService := merkledag.NewDAGService(bsvc)

	// Reconstruct tree from root CID
	summary, err := reconstructTree(ctx, dagService, rootCID, ibs)
	if err != nil {
		return nil, fmt.Errorf("reconstruct tree: %w", err)
	}

	return summary, nil
}
