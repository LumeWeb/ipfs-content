package car

import (
	"context"
	"fmt"
	"slices"

	boxoblockstore "github.com/ipfs/boxo/blockstore"
	pb "github.com/ipfs/boxo/ipld/unixfs/pb"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"

	"go.lumeweb.com/ipfs-content/dagnode"
)

// reconstructTree walks the IPFS DAG from the root CID and reconstructs the complete
// directory hierarchy tree structure.
//
// This function is responsible for reading a CAR file's DAG (Directed Acyclic Graph)
// and building a TreeSummary that matches the structure created by BuildSummary, ensuring
// perfect round-trip compatibility: BuildSummary → WriteCAR → ReadCAR = identical TreeSummary.
//
// **Architecture Overview:**
//
// Reconstruction operates on the premise that CAR files don't store full hierarchical
// paths; only immediate child names are preserved in UnixFS directory link structures.
// This function synthesizes full paths during DAG traversal using the parent context
// and link names to reconstruct the complete tree structure.
//
// The function operates in two phases:
//
//  1. **Tree Structure Phase:** Walk the DAG from rootCID, building TreeEntries and
//     establishing parent-child relationships. Path synthesis occurs here.
//
//  2. **Metadata Collection Phase:** Call collectBlockMetadata() to traverse the DAG
//     again (BFS), collecting block CIDs, sizes, and total size in deterministic order.
//
// **Root Type Detection:**
//
// The function first analyzes the root node to determine the CAR structure:
//
//  1. **Root is Directory (wrapInDir=true):** The root CID points to a UnixFS directory
//     node (UnixFS type = pb.Data_Directory). This is the default structure from
//     StreamCAR with wrapInDir=true.
//
//     ROOT entry is initialized with:
//     - Name = "ROOT"
//     - Path = "" (not "ROOT" like builder, for reconstruction consistency)
//     Path is set to empty string "" for ROOT entry (unlike builder which uses
//     "ROOT"). This ensures consistency with how root-level entries (Path="")
//     are handled throughout the reconstruction process. The virtual ROOT is
//     still identified by its Name="ROOT" constant.
//     - IsDir = true
//     - CID = root CID
//
//     Root-level files are added to ROOT.Children. Root-level directories are NOT
//     added to ROOT.Children (matching BuildSummary's critical "files only" rule).
//
//  2. **Root is File (wrapInDir=false):** The root CID points directly to a UnixFS
//     file node. This is the optimization from StreamCAR with wrapInDir=false for
//     single-file mode.
//
//     File entry is created with:
//     - Name = rootCID.String() (original filename not stored in CAR)
//     - Path = ""
//     - IsDir = false
//     - CID = rootCID
//     - LogicalFileSize from UnixFS metadata
//
//     ROOT wrapper entry is created around this file with Children containing the
//     file's CID-derived filename.
//
// **Root Type Detection Logic:**
//
// The function uses `dagnode.AnalyzeNode()` to determine UnixFS node type:
//
//	if nodeInfo.UnixFSType == pb.Data_Directory {
//	    // Root is directory
//	} else {
//	    // Root is file (or other UnixFS type)
//	}
//
// **Path Synthesis Algorithm:**
//
// Since CAR files don't store full paths, paths are synthesized during DAG traversal
// using the following algorithm:
//
//   - **ROOT Children:** `entryKey = nodeName` (e.g., "file.txt", "dir1")
//   - **Nested Children:** `entryKey = parentPath + "/" + nodeName` (e.g., "dir1/file.txt")
//   - **Repeated Pattern:** Each level concatenates parent path with child name
//
// Entry Path field is set to the parent's entry key (or "" for root-level).
//
// Example for "dir1/subdir1/file2.txt":
//
//   - Initial: parentPath = ROOT, nodeName = "dir1"
//     → entryKey = "dir1", entry.Path = ""
//
//   - Next: parentPath = "dir1", nodeName = "subdir1"
//     → entryKey = "dir1/subdir1", entry.Path = "dir1"
//
//   - Final: parentPath = "dir1/subdir1", nodeName = "file2.txt"
//     → entryKey = "dir1/subdir1/file2.txt", entry.Path = "dir1/subdir1"
//
// This produces identical path keys to BuildSummary, enabling perfect round-trip.
//
// **Parent-Child Relationship Construction:**
//
// Parent-child relationships are built during traversal by:
//
//   - **ROOT Children:** Only files are added to ROOT.Children (directories skipped)
//   - **Non-ROOT Children:** All children (files AND directories) are added to parent's
//     Children array
//
// This matches BuildSummary's behavior where ROOT.Children contains only root-level files.
//
// **Cycle Detection:**
//
// DAG traversal includes cycle detection to handle malformed or corrupted CARs:
//
//   - CIDToEntry map tracks processed CIDs
//   - At each node, check: `if _, processed := summary.CIDToEntry[cid]; processed`
//   - Already-processed CIDs are skipped to prevent infinite loops
//
// This is essential for robustness as CAR files should be DAGs (acyclic), but malformed
// files or hardlinks could create cycles.
//
// **Root-Level Directory Handling:**
//
// When root is a directory, root-level directories have special handling:
//
//   - They are created as TreeEntries with entryKey = dirName (e.g., "dir1")
//   - Path is set to "" (same as root-level files)
//   - They have Children arrays containing nested items
//   - They are NOT added to ROOT.Children (critical for round-trip compatibility)
//
// This matches the builder's rule: "ROOT.Children contains ONLY files, not directories".
//
// However, during UnixFS directory block creation (not in this function), root-level
// directories ARE added as links in the ROOT directory block's children list. The
// separation between ROOT.Children (for tree structure) and UnixFS directory blocks
// (for IPFS structure) enables this dual behavior.
//
// **Virtual ROOT Entry:**
//
// The ROOT entry is always created as a virtual directory wrapper, regardless of whether
// the CAR's root is actually a directory or file:
//
//   - **When root is directory (wrapInDir=true):**
//     ROOT contains root-level files in its Children array. Root-level directories
//     exist in TreeEntries but are NOT added to ROOT.Children.
//
//   - **When root is file (wrapInDir=false):**
//     ROOT wraps the single file entry as a virtual directory. ROOT.Children contains
//     the file's CID-derived filename.
//
// Note: The ROOT entry itself is NOT a real IPFS node. It's a virtual construct in
// TreeSummary that enables consistent tree structure representation matching BuildSummary.
// The IPFS-level root CID (summary.RootCID) remains the actual root from the CAR file.
//
// **Tree Initialization:**
//
// TreeSummary is initialized with empty maps:
//
//	summary := &TreeSummary{
//	    RootCID:     rootCID,
//	    TreeEntries: make(map[string]*TreeEntry),
//	    CIDToEntry:  make(map[cid.Cid]*TreeEntry),
//	}
//
// BlockOrder, BlockSizes, TotalSize, and CARSize are populated later by
// collectBlockMetadata().
//
// **Node Analysis:**
//
// Each node is analyzed using `dagnode.AnalyzeNode()` which extracts:
//
//   - UnixFSType: File type (pb.Data_File, pb.Data_Directory, etc.)
//   - FileSize: Original file size (for UnixFS file nodes)
//   - Links: Child CIDs with names for directory nodes
//
// This metadata populates TreeEntry fields:
//
//   - IsDir = (UnixFSType == pb.Data_Directory)
//   - LogicalFileSize = FileSize (for files, 0 for directories)
//   - Children array built from Links (for directories)
//
// **Error Propagation:**
//
// Errors are wrapped with context for debugging:
//
//   - "get root node": Failed to fetch root CID from DAG service
//   - "analyze root node": Failed to analyze UnixFS structure
//   - "walk root child": Failed to walk a child of root
//
// **Post-Processing:**
//
// After tree structure is built, the function calls collectBlockMetadata() to:
//
//   - Perform BFS traversal for deterministic BlockOrder
//   - Collect BlockSizes for each block
//   - Calculate TotalSize (sum of all block sizes)
//
// The summary is returned with complete tree structure and block metadata.
//
// **Root-is-File Case Example:**
//
// For a CAR created with wrapInDir=false (single file):
//
//	// CAR root is a file node with CID: QmSomeFile
//	fileName := rootCID.String()  // e.g., "QmSomeFile"
//
//	// File entry for the CAR content
//	fileEntry := &TreeEntry{
//	    Name:            "QmSomeFile",
//	    IsDir:           false,
//	    Path:            "",
//	    CID:             rootCID,
//	    LogicalFileSize: fileNode.FileSize,
//	}
//	summary.TreeEntries["QmSomeFile"] = fileEntry
//
//	// ROOT wrapper (virtual)
//	rootEntry := &TreeEntry{
//	    Name:     ROOT,
//	    IsDir:    true,
//	    Path:     "",
//	    CID:      rootCID,
//	    Children: []string{"QmSomeFile"},
//	}
//	summary.TreeEntries[ROOT] = rootEntry
//
// Note: The original filename is lost (not stored in CAR), so the CID string is used.
//
// **Parameters:**
//
//   - ctx: context.Context - Context for cancellation and deadlines
//   - dagService: format.DAGService - DAG service for node access and traversal
//   - rootCID: cid.Cid - Root content identifier to start traversal from
//   - bs: boxoblockstore.Blockstore - Blockstore for block access (used by collectBlockMetadata)
//
// **Returns:**
//
//   - *TreeSummary: Reconstructed tree structure complete with entries, paths, and
//     block metadata (populated by collectBlockMetadata)
//   - error: Error if root node fetch fails, analysis fails, or traversal errors occur
func reconstructTree(ctx context.Context, dagService format.DAGService, rootCID cid.Cid, bs boxoblockstore.Blockstore) (*TreeSummary, error) {
	summary := &TreeSummary{
		RootCID:     rootCID,
		TreeEntries: make(map[string]*TreeEntry),
		CIDToEntry:  make(map[cid.Cid]*TreeEntry),
	}

	// Walk the DAG and build tree structure starting from root
	nd, err := dagService.Get(ctx, rootCID)
	if err != nil {
		return nil, fmt.Errorf("get root node: %w", err)
	}

	// Check if root is a directory (which it should be for wrapped structures)
	nodeInfo, err := dagnode.AnalyzeNode(ctx, nd)
	if err != nil {
		return nil, fmt.Errorf("analyze root node: %w", err)
	}

	isRootDir := nodeInfo.UnixFSType == pb.Data_Directory



	if isRootDir {
		// Root is a directory - initialize ROOT directory entry
		summary.TreeEntries[ROOT] = &TreeEntry{
			Name:  ROOT,
			IsDir: true,
			Path:  "",
			CID:   rootCID,
		}
		summary.CIDToEntry[rootCID] = summary.TreeEntries[ROOT]

		// Process children as root-level entries
		links := nd.Links()

		for _, link := range links {
			if link == nil || link.Cid == cid.Undef {
				continue
			}
			childName := link.Name
			if childName == "" {
				childName = link.Cid.String()
			}

			if err := walkDAG(ctx, dagService, link.Cid, ROOT, childName, summary); err != nil {
				return nil, fmt.Errorf("walk root child %s: %w", link.Cid, err)
			}
		}

	} else {
		// Root is a file (single file, no directory wrapping)


		// Use CID as filename since original filename is not stored in CAR
		fileName := rootCID.String()



		fileEntry := &TreeEntry{
			Name:            fileName,
			IsDir:           false,
			Path:            "",
			CID:             rootCID,
			LogicalFileSize: nodeInfo.FileSize,
		}
		summary.TreeEntries[fileName] = fileEntry
		summary.CIDToEntry[rootCID] = fileEntry

		// Initialize ROOT directory wrapper around this file
		rootEntry := &TreeEntry{
			Name:     ROOT,
			IsDir:    true,
			Path:     "",
			CID:      rootCID,
			Children: []string{fileName},
		}
		summary.TreeEntries[ROOT] = rootEntry


	}
	// Collect block order and sizes
	if err := collectBlockMetadata(ctx, dagService, bs, rootCID, summary); err != nil {
		return nil, fmt.Errorf("collect block metadata: %w", err)
	}

	return summary, nil
}

// walkDAG recursively walks the IPFS DAG from a given CID, building the tree structure
// and synthesizing paths.
//
// This function is called by reconstructTree for each node in the DAG, performing
// path synthesis, creating TreeEntries, and establishing parent-child relationships.
// The recursion only continues for directories; files are leaves in the tree.
//
// **Path Synthesis:**
//
// Since CAR files don't store full hierarchical paths (only immediate child names),
// this function synthesizes full paths during traversal using the parent context:
//
//   - **For ROOT Children (parentPath == ROOT):**
//     entryKey = nodeName (e.g., "file.txt", "dir1")
//     entryPath = "" (root-level entries have no parent)
//
//   - **For Nested Children (parentPath != ROOT):**
//     entryKey = parentPath + "/" + nodeName (e.g., "dir1/file.txt", "dir1/subdir1/file2.txt")
//     entryPath = parentPath (e.g., "dir1", "dir1/subdir1")
//
// This synthesis algorithm produces paths identical to those generated by BuildSummary,
// ensuring perfect round-trip compatibility.
//
// Examples:
//
//	Root-level file "file.txt":
//	  parentPath = ROOT, nodeName = "file.txt"
//	  → entryKey = "file.txt", entry.Path = ""
//
//	Root-level directory "dir1":
//	  parentPath = ROOT, nodeName = "dir1"
//	  → entryKey = "dir1", entry.Path = ""
//
//	Nested file "dir1/file.txt":
//	  parentPath = "dir1", nodeName = "file.txt"
//	  → entryKey = "dir1/file.txt", entry.Path = "dir1"
//
//	Deeply nested "dir1/subdir1/file2.txt":
//	  parentPath = "dir1/subdir1", nodeName = "file2.txt"
//	  → entryKey = "dir1/subdir1/file2.txt", entry.Path = "dir1/subdir1"
//
// **Path Separator Consistency:**
//
// Paths are always constructed with "/" (forward slash), regardless of platform.
// This ensures cross-platform consistency and matches the builder's path generation.
//
// **Entry Creation vs Update:**
//
// The function first checks if an entry with entryKey already exists:
//
//	if entry, exists := summary.TreeEntries[entryKey]; exists {
//	    // Update existing entry
//	} else {
//	    // Create new entry
//	}
//
// While new entries are typically created during traversal, the existence check
// provides robustness for edge cases where entries might be created elsewhere.
//
// **Node Type Detection:**
//
// The function determines node type using UnixFS metadata analysis:
//
//	nodeInfo, err := dagnode.AnalyzeNode(ctx, nd)
//	isDir := nodeInfo.UnixFSType == pb.Data_Directory
//
// UnixFS types include:
//   - pb.Data_File: Regular file (isDir = false)
//   - pb.Data_Directory: Directory (isDir = true)
//   - pb.Data_Symlink: Symbolic link (isDir = false)
//   - pb.Data_Raw: Raw data block (isDir = false)
//
// Only directories have recursive child traversal; other types are leaves.
//
// **Parent-Child Relationship Construction:**
//
// Parent-child relationships follow BuildSummary's critical "files only" rule:
//
//   - **ROOT Children (parentPath == ROOT):**
//
//   - Files are added to ROOT.Children
//
//   - Directories are NOT added to ROOT.Children (they exist in TreeEntries but
//     are excluded from ROOT.Children)
//
//   - This matches the builder's behavior for round-trip compatibility
//
//   - **Non-ROOT Children (parentPath != ROOT):**
//
//   - ALL children are added to parent's Children array (files AND directories)
//
//   - This enables proper tree structure reconstruction for all non-root levels
//
// Example:
//
//	For filesystem: file.txt (file), dir1/ (directory), dir1/file2.txt (file)
//
//	ROOT.Children = ["file.txt"]  // Only root-level files
//	"dir1" exists in TreeEntries with Children = ["dir1/file2.txt"]
//	"dir1/file2.txt" is a file entry (leaf)
//
// **Cycle Detection:**
//
// The function includes cycle detection to handle malformed or corrupted CARs
// that might contain circular references:
//
//	if _, processed := summary.CIDToEntry[link.Cid]; processed {
//	    continue  // Skip already-processed CIDs
//	}
//
// This is essential for robustness. While valid UnixFS DAGs should be acyclic,
// hardlinks or corrupted data could create cycles that would cause infinite loops
// without this check.
//
// **Recursive Traversal:**
//
// Recursion only proceeds for directories (isDir == true):
//
//	if isDir {
//	    links := nd.Links()
//	    for _, link := range links {
//	        // Recursively walk children
//	        walkDAG(ctx, dagService, link.Cid, entryKey, link.Name, summary)
//	    }
//	}
//
// Files and other node types terminate recursion (they are leaves in the tree).
//
// **Link Name Handling:**
//
// UnixFS directory links have names specifying the child basename:
//
//	childName := link.Name
//	if childName == "" {
//	    childName = link.Cid.String()  // Fallback
//	}
//
// The empty string fallback handles edge cases where UnixFS metadata doesn't
// include link names (shouldn't occur in well-formed CARs but provides robustness).
//
// **Entry Metadata Population:**
//
// TreeEntry fields are populated during traversal:
//
//   - Name: nodeName (basename from link)
//   - Path: entryPath (parent's entryKey or "" for root-level)
//   - IsDir: isDir (from UnixFS type analysis)
//   - CID: nodeCID (extracted from IPFS block)
//   - LogicalFileSize: nodeInfo.FileSize (if file, 0 for directories)
//
// Children array is populated separately when adding entries to their parents.
// CIDToEntry map is updated for cycle detection and block metadata lookup.
//
// **CIDToEntry Map Updates:**
//
// Each processed CID is added to CIDToEntry for two purposes:
//
//  1. **Cycle Detection:** Check if CID was already processed to avoid infinite loops
//  2. **Block Metadata:** collectBlockMetadata() uses this map to find entry metadata
//
// Note: For files with multiple chunks, only the file's top-level CID (UnixFS file node)
// is mapped to the entry. Each chunk CID is a separate IPFS block tracked by the
// IndexedBlockstore's index, but not linked to TreeEntry via CIDToEntry. CIDToEntry
// links top-level CIDs (directory or single-file) to entries for lookups.
//
// **Deduplication:**
//
// Duplicate entries (same entryKey) are handled by the exists check. If an entry
// with the same path already exists, it's updated rather than duplicated. This
// provides robustness for edge cases in malformed CARs.
//
// **Error Handling:**
//
// Errors are wrapped with context:
//
//   - "get node {cid}": Failed to fetch node from DAG service
//   - "analyze node {cid}": Failed to analyze UnixFS structure
//   - "walk child {cid}": Failed to descend into child node
//
// **Recursion Control:**
//
// Recursion is controlled by node type (directories only recurse) and cycle detection
// (prevents processing same CID twice). This ensures the function terminates even
// for malformed CARs with cycles.
//
// **Context Propagation:**
//
// The function checks and respects context cancellation:
//
//	if err := ctx.Err(); err != nil {
//	    return err
//	}
//
// This allows callers to cancel long-running reconstructions or enforce timeouts.
//
// **Parameters:**
//
//   - ctx: context.Context - Context for cancellation and deadlines
//   - dagService: format.DAGService - DAG service for node access
//   - nodeCID: cid.Cid - CID of the node to process
//   - parentPath: string - Parent entry's key (ROOT for root-level, or full path otherwise)
//   - nodeName: string - Basename of this entry (from IPFS link name)
//   - summary: *TreeSummary - TreeSummary being built (mutated during recursion)
//
// **Returns:**
//
//   - error: Error if node fetch fails, analysis fails, or child traversal errors
func walkDAG(ctx context.Context, dagService format.DAGService, nodeCID cid.Cid, parentPath string, nodeName string, summary *TreeSummary) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get the node from DAG service
	nd, err := dagService.Get(ctx, nodeCID)
	if err != nil {
		return fmt.Errorf("get node %s: %w", nodeCID, err)
	}

	// Decode the node to analyze its structure
	nodeInfo, err := dagnode.AnalyzeNode(ctx, nd)
	if err != nil {
		return fmt.Errorf("analyze node %s: %w", nodeCID, err)
	}

	// Determine if this is a directory or file
	isDir := nodeInfo.UnixFSType == pb.Data_Directory

	// Build entry key and path based on parentPath
	var entryKey, entryPathValue string

	switch parentPath {
	case ROOT:
		// Child of ROOT - these are root-level entries
		entryKey = nodeName
		entryPathValue = ""
	default:
		// Nested child - parentPath is the parent's entryKey
		entryKey = parentPath + "/" + nodeName // Use "/" as separator consistently
		entryPathValue = parentPath
	}



	// Create or update tree entry
	entry, exists := summary.TreeEntries[entryKey]
	if !exists {
		entry = &TreeEntry{
			Name:  nodeName,
			IsDir: isDir,
			Path:  entryPathValue,
			CID:   nodeCID,
		}
		summary.TreeEntries[entryKey] = entry
	}

	// Update entry with additional metadata
	entry.CID = nodeCID
	if nodeInfo.FileSize > 0 {
		entry.LogicalFileSize = nodeInfo.FileSize
	}
	summary.CIDToEntry[nodeCID] = entry

	// Add to parent's children list
	if parentPath == ROOT {
		// For ROOT, only add files (not directories) to match TreeSummary structure
		if !isDir {
			rootEntry := summary.TreeEntries[ROOT]
			if rootEntry != nil && !slices.Contains(rootEntry.Children, entryKey) {
				rootEntry.Children = append(rootEntry.Children, entryKey)

			}
		} else {

		}
	} else {
		// For non-ROOT parents, add all children
		parentEntry := summary.TreeEntries[parentPath]
		if parentEntry != nil && !slices.Contains(parentEntry.Children, entryKey) {
			parentEntry.Children = append(parentEntry.Children, entryKey)

		}
	}

	// Recursively walk children (only if this is a directory)
	if isDir {
		links := nd.Links()

		for _, link := range links {
			if link == nil || link.Cid == cid.Undef {
				continue
			}

			// Skip if we've already processed this CID (handle cycles)
			if _, processed := summary.CIDToEntry[link.Cid]; processed {

				continue
			}

			// In UnixFS, directory links have names
			childName := link.Name
			if childName == "" {
				childName = link.Cid.String()
			}

			// Child's parentPath is this entry's entryKey
			childParentPath := entryKey

			// Recursively walk child
			if err := walkDAG(ctx, dagService, link.Cid, childParentPath, childName, summary); err != nil {
				return fmt.Errorf("walk child %s: %w", link.Cid, err)
			}
		}
	}

	return nil
}

// collectBlockMetadata performs a BFS (breadth-first search) traversal to collect all block
// CIDs, sizes, and traversal order for deterministic CAR reconstruction.
//
// This function is called by reconstructTree after the tree structure is built. It traverses
// the entire DAG to populate BlockOrder, BlockSizes, and TotalSize fields in TreeSummary.
//
// **Traversal Algorithm:**
//
// The function uses a queue-based BFS traversal:
//
//  1. Initialize queue with root CID and seen map to track visited CIDs
//  2. Dequeue current CID, skip if already seen (cycle detection)
//  3. Fetch block size via multiple methods (tried in order)
//  4. Add CID to BlockOrder, size to BlockSizes, accumulate TotalSize
//  5. Enqueue all child CIDs from node's links
//  6. Repeat until queue is empty
//
// Pseudocode:
//
//	queue = [rootCID]
//	seen = {}
//	while queue not empty:
//	    current = queue.pop_front()
//	    if seen[current]: continue
//	    seen[current] = true
//	    blockSize = fetch_size(current)
//	    BlockOrder.append(current)
//	    BlockSizes.append(blockSize)
//	    TotalSize += blockSize
//	    queue.extend(get_children(current))
//
// **BFS vs DFS:**
//
// BFS (breadth-first) is used instead of DFS (depth-first) for deterministic traversal
// order that matches CAR writing order:
//
//   - BFS processes levels iteratively: root, then immediate children, then grandchildren
//   - DFS would process entire subtrees before siblings
//   - Both visit all nodes, but BFS provides more consistent ordering
//
// Note: While the BlockOrder produced by BFS may differ from BuildSummary's order,
// this is acceptable because CAR format only requires that all blocks are present
// (order consistency matters for verification but not for parsing).
//
// BuildSummary uses deep-first traversal during the WriteCAR phase (files added during
// filesystem walk, directories sorted deep-first), while collectBlockMetadata uses BFS.
// The resulting BlockOrder may differ but both produce deterministic, complete coverage.
//
// **Cycle Detection:**
//
// The seen map prevents processing the same CID twice:
//
//	if seen[currentCID] {
//	    continue  // Skip already-processed CIDs
//	}
//
// This is essential for robustness. While valid UnixFS DAGs should be acyclic,
// hardlinks or malformed CARs could contain cycles that would cause infinite
// loops without this check.
//
// **Block Size Fallback Strategy:**
//
// Block sizes are fetched via this fallback chain:
//
//  1. **Try blockstore.GetSize():** blockstore.GetSize(ctx, currentCID)
//     - Most efficient: IndexedBlockstore can answer O(1) from index
//     - May fail for blocks not in cache
//
//  2. **Try blockstore.Get + len():** len(blockstore.Get(ctx, currentCID).RawData())
//     - Loads block from cache into memory
//     - Works if block is cached
//
//  3. **Try dagService.Get + len():** len(dagService.Get(ctx, currentCID).RawData())
//     - Fetches block through DAG service (may trigger regeneration)
//     - Fallback for blocks evicted from cache
//
//  4. **Skip on failure:** If all methods fail, blockSize remains 0 (rare)
//
// This multi-strategy approach ensures size metadata is collected even when blocks
// are evicted from the LRU cache, ensuring deterministic reconstruction.
//
// **Deterministic Ordering:**
//
// The BlockOrder array is critical for deterministic CAR reconstruction:
//
//   - All block CIDs are added to BlockOrder in BFS traversal order
//   - BlockSizes is a parallel array with size for each CID (by index)
//   - TotalSize is accumulated as blocks are processed
//
// While the specific order may differ from BuildSummary's order (different traversal
// algorithms), the deterministic nature ensures that repeated reads of the same CAR
// produce identical BlockOrder and BlockSizes.
//
// **BlockOrder and BlockSizes Semantics:**
//
// BlockOrder and BlockSizes are parallel arrays:
//
//	BlockOrder = [cid1, cid2, cid3, ...]
//	BlockSizes = [size1, size2, size3, ...]
//
// These are NOT CAR file block order; they're metadata arrays for:
//
//   - CAR size pre-calculation (CalculateCARSize)
//   - Block deduplication verification
//   - Memory footprint analysis
//
// The actual CAR reading uses IndexedBlockstore's index for random access.
//
// **TotalSize Calculation:**
//
// TotalSize is the sum of all raw block sizes:
//
//	summary.TotalSize += blockSize  // Accumulated for each block
//
// This represents the total size of all blocks in bytes, excluding CARv1 header
// and block framing overhead (varint length prefix). CARSize (if calculated) would
// include header and framing.
//
// **CIDToEntry Utilization:**
//
// The function attempts to use CIDToEntry for size lookup optimization:
//
//	entry := summary.CIDToEntry[currentCID]
//	if entry != nil {
//	    // Try blockstore.GetSize using entry
//	}
//
// However, note that for files with multiple chunks, only the root CID is mapped
// to the entry. Chunk CIDs won't have corresponding entries. This is acceptable
// as the fallback strategies handle all cases.
//
// **Context Respect:**
//
// The function checks and respects context cancellation:
//
//	if err := ctx.Err(); err != nil {
//	    return err
//	}
//
// This allows callers to cancel long-running metadata collection or enforce timeouts.
// Large CARs with many blocks could take significant time to traverse.
//
// **Tree Structure Independence:**
//
// CollectBlockMetadata is independent of the tree structure built earlier:
//
//   - TreeEntries and parent-child relationships are already built
//   - This function only cares about DAG connectivity, not tree hierarchy
//   - It traverses IPFS blocks (level below UnixFS tree structure)
//
// This separation allows tree structure and block metadata to be populated
// independently, with reconstructTree handling both phases.
//
// **Child CID Enqueuing:**
//
// Children are enqueued from the current node's links:
//
//	nd, _ := dagService.Get(ctx, currentCID)
//	for _, link := range nd.Links() {
//	    if link == nil || link.Cid == cid.Undef {
//	        continue
//	    }
//	    if !seen[link.Cid] {
//	        queue = append(queue, link.Cid)
//	    }
//	}
//
// The seen check before enqueueing reduces unnecessary work but doesn't replace
// the seen check during dequeue (which handles cycle detection).
//
// **Empty CAR Handling:**
//
// For an empty CAR (only root directory with no children):
//
//   - Only root CID is processed
//   - BlockOrder = [rootCID] (single block)
//   - BlockSizes = [rootSize] (size of root directory block)
//   - TotalSize = rootSize
//
// This matches the BuildSummary behavior for empty filesystems.
//
// Very rare, but handled correctly without special-casing.
//
// **Performance Characteristics:**
//
//   - **Time:** O(V) where V is the number of blocks (each block visited once)
//   - **Space:** O(V) for seen map + O(W) for queue where W is tree width
//   - **I/O:** Each block may be loaded once for size (or use cached size)
//
// **Parameters:**
//
//   - ctx: context.Context - Context for cancellation and deadlines
//   - dagService: format.DAGService - DAG service for node and block access
//   - bs: boxoblockstore.Blockstore - Blockstore for size queries (IndexedBlockstore)
//   - rootCID: cid.Cid - Root CID to start traversal from
//   - summary: *TreeSummary - TreeSummary being populated (BlockOrder, BlockSizes, TotalSize)
//
// **Returns:**
//
//   - error: Error if context is cancelled, traversal errors occur, or failures
func collectBlockMetadata(ctx context.Context, dagService format.DAGService, bs boxoblockstore.Blockstore, rootCID cid.Cid, summary *TreeSummary) error {
	queue := []cid.Cid{rootCID}
	seen := make(map[cid.Cid]bool)

	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Get current block
		currentCID := queue[0]
		queue = queue[1:]

		// Skip if already seen (avoid cycles)
		if seen[currentCID] {
			continue
		}
		seen[currentCID] = true

		// Get block size
		entry := summary.CIDToEntry[currentCID]
		var blockSize uint64
		if entry != nil {
			var err error
			size, err := bs.GetSize(ctx, currentCID)
			if err == nil {
				blockSize = uint64(size)
			} else {
				// Get the node to get size
				nd, err := bs.Get(ctx, currentCID)
				if err == nil {
					blockSize = uint64(len(nd.RawData()))
				}
			}
		} else {
			// Get the node directly from blockstore
			nd, err := dagService.Get(ctx, currentCID)
			if err == nil {
				blockSize = uint64(len(nd.RawData()))
			}
		}

		// Add to block order and sizes
		summary.BlockOrder = append(summary.BlockOrder, currentCID)
		summary.BlockSizes = append(summary.BlockSizes, blockSize)
		summary.TotalSize += blockSize

		// Add children to queue
		nd, err := dagService.Get(ctx, currentCID)
		if err != nil {
			continue
		}

		for _, link := range nd.Links() {
			if link == nil || link.Cid == cid.Undef {
				continue
			}
			if !seen[link.Cid] {
				queue = append(queue, link.Cid)
			}
		}
	}



	return nil
}

// GetFileTree extracts all file paths from a TreeSummary.
//
// This function iterates through all TreeEntries and returns a list of paths that
// represent files (not directories). The virtual ROOT entry is excluded from results.
//
// **Use Cases:**
//
//  1. **Iteration:** Iterate through all files in a CAR for processing or inspection
//  2. **Counting:** Count the number of files in the tree (len(result))
//  3. **Export:** Export file list for external indexing or cataloging
//
// **Filtering Logic:**
//
// Entries are included in the result if:
//   - entry.IsDir == false (it's a file)
//   - path != ROOT (not the virtual root directory)
//
// This ensures only actual files are returned, excluding:
//   - Directory entries (IsDir == true)
//   - Virtual ROOT entry (path == ROOT)
//
// **Path Format:**
//
// Returned paths are full path strings as used in TreeEntries keys:
//
//   - Root-level files: "file.txt" (for file at filesystem root)
//   - Nested files: "dir1/file.txt" (for file in directory)
//   - Deeply nested: "dir1/subdir1/file2.txt" (for file in nested directory)
//
// These paths can be used to look up entries in summary.TreeEntries map.
//
// **Ordering:**
//
// The returned list order is not deterministic (depends on map iteration order).
// If deterministic order is needed, sort the result:
//
//	paths := GetFileTree(summary)
//	sort.Strings(paths)
//	sort.SliceStable(paths, func(i, j int) bool {
//	    // Custom sort logic
//	})
//
// **Empty Tree Handling:**
//
// For a TreeSummary with no files (e.g., empty CAR), returns nil slice (not empty slice):
//
//	files := GetFileTree(summary) // == nil
//
// This distinguishes "no files" from "empty file list" if needed.
//
// **Relation to BuildSummary:**
//
// When used on a TreeSummary from BuildSummary or ReadCAR, this function returns
// the same file set that would have been encountered during the filesystem walk.
// Round-trip: BuildSummary → WriteCAR → ReadCAR → GetFileTree = original files
//
// **Example Usage:**
//
//	summary, err := car.ReadCAR(carFile, 100*1024*1024)
//	if err != nil {
//	    return err
//	}
//
//	// Get all file paths
//	paths := GetFileTree(summary)
//	for i, path := range paths {
//	    fmt.Printf("File %d: %s\n", i+1, path)
//	}
//
//	// Count total files
//	fmt.Printf("Total files: %d\n", len(paths))
//
//	// Look up each file's metadata
//	for _, path := range paths {
//	    entry := summary.TreeEntries[path]
//	    fmt.Printf("%s: CID=%s, Size=%d\n", path, entry.CID, entry.LogicalFileSize)
//	}
//
// **Parameters:**
//
//   - summary: *TreeSummary - TreeSummary to extract file paths from
//
// **Returns:**
//
//   - []string: List of file paths (full path strings as TreeEntries keys).
//     Returns nil slice if no files exist (not empty list).
func GetFileTree(summary *TreeSummary) []string {
	paths := make([]string, 0)

	for path, entry := range summary.TreeEntries {
		if !entry.IsDir && path != ROOT {
			paths = append(paths, path)
		}
	}

	return paths
}

// GetDirectoryTree extracts all directory paths from a TreeSummary.
//
// This function iterates through all TreeEntries and returns a list of paths that
// represent directories (not files). The virtual ROOT entry is excluded from results.
//
// **Use Cases:**
//
//  1. **Iteration:** Iterate through all directories in a CAR for processing
//  2. **Counting:** Count the number of directories in the tree
//  3. **Export:** Export directory list for external indexing
//  4. **Traversal:** Use paths for custom directory traversal logic
//
// **Filtering Logic:**
//
// Entries are included in the result if:
//   - entry.IsDir == true (it's a directory)
//   - path != ROOT (not the virtual root directory)
//
// This ensures only actual directories are returned, excluding:
//   - File entries (IsDir == false)
//   - Virtual ROOT entry (path == ROOT)
//
// **Path Format:**
//
// Returned paths are full path strings as used in TreeEntries keys:
//
//   - Root-level directories: "dir1" (for directory at filesystem root)
//   - Nested directories: "dir1/subdir1" (for subdirectory)
//   - Deeply nested: "dir1/subdir1/subdir2" (for deeply nested directory)
//
// These paths can be used to look up entries in summary.TreeEntries map.
//
// **ROOT Exclusion:**
//
// The virtual ROOT entry (path == ROOT) is excluded because:
//
//   - ROOT is a synthetic virtual entry, not a real filesystem directory
//   - ROOT doesn't correspond to any directory in the original filesystem
//   - Including ROOT would be confusing for users expecting real paths
//
// **Relation to Builder:**
//
// Root-level directories (Path="" in TreeEntries) ARE included in the result
// with their entry key as the path (e.g., "dir1"). This matches the behavior
// where root-level directories exist in TreeEntries but not in ROOT.Children.
//
// **Ordering:**
//
// The returned list order is not deterministic (depends on map iteration order).
// If deterministic order is needed, sort the result:
//
//	paths := GetDirectoryTree(summary)
//	sort.Strings(paths)
//
// Note: Sorting alphabetically will NOT sort by depth (directory hierarchy).
// For depth-sorted order, custom sort logic is needed:
//
//	sort.Slice(paths, func(i, j int) bool {
//	    depthI := strings.Count(paths[i], "/")
//	    depthJ := strings.Count(paths[j], "/")
//	    if depthI != depthJ {
//	        return depthI < depthJ  // Shallowest first
//	    }
//	    return paths[i] < paths[j]  // Alphabetical tiebreaker
//	})
//
// **Empty Tree Handling:**
//
// For a TreeSummary with no directories (e.g., single-file CAR), returns nil
// slice (not empty slice):
//
//	dirs := GetDirectoryTree(summary) // == nil
//
// This distinguishes "no directories" from "empty directory list" if needed.
//
// **Example Usage:**
//
//	summary, err := car.ReadCAR(carFile, 100*1024*1024)
//	if err != nil {
//	    return err
//	}
//
//	// Get all directory paths
//	paths := GetDirectoryTree(summary)
//	for i, path := range paths {
//	    fmt.Printf("Directory %d: %s\n", i+1, path)
//	}
//
//	// Process each directory
//	for _, dirPath := range paths {
//	    entry := summary.TreeEntries[dirPath]
//	    fmt.Printf("%s: CID=%s, Children=%d\n", dirPath, entry.CID, len(entry.Children))
//	}
//
//	// Custom traversal (depth-first)
//	for _, dirPath := range paths {
//	    // Process directory...
//	}
//
// **Round-Trip Consistency:**
//
// When used on TreeSummary from BuildSummary or ReadCAR, this function returns
// directories consistent across round-trip operations. BuildSummary creates
// directory entries (not pruned), and ReadCAR reconstructs them via walkDAG.
//
// **Empty Directory Exclusion:**
//
// Empty directories are also returned by this function if they exist in TreeEntries.
// However, BuildSummary's pruneEmptyDirectories() removes empty directories from
// the tree before directory block creation, so they typically won't exist in the
// final TreeSummary.
//
// **Parameters:**
//
//   - summary: *TreeSummary - TreeSummary to extract directory paths from
//
// **Returns:**
//
//   - []string: List of directory paths (full path strings as TreeEntries keys).
//     Returns nil slice if no directories exist (not empty list).
func GetDirectoryTree(summary *TreeSummary) []string {
	paths := make([]string, 0)

	for path, entry := range summary.TreeEntries {
		if entry.IsDir && path != ROOT {
			paths = append(paths, path)
		}
	}

	return paths
}
