package car

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/ipld/unixfs/importer/helpers"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/samber/lo"

	"go.lumeweb.com/ipfs-content/dagnode"
	"go.lumeweb.com/ipfs-content/encoding"
	"go.lumeweb.com/ipfs-content/internal/carv2"
	carv2util "go.lumeweb.com/ipfs-content/internal/carv2/util"
	internalio "go.lumeweb.com/ipfs-content/internal/io"
	"go.lumeweb.com/ipfs-content/unixfs"
)

const (
	ROOT       = "ROOT"
	CurrentDir = "."
	ParentDir  = ".."
)

// CARBuilder performs two-pass CAR generation:
// Pass 1: Walk filesystem and build summary with metadata
// Pass 2: Write CARv1 using the summary, regenerating blocks on demand
type CARBuilder struct {
	bs         blockstore.Blockstore
	dagService format.DAGService
	generator  unixfs.UnixFSNodeGenerator
	filesystem fs.FS
	wrapInDir  bool
	summary    *TreeSummary
	chunkSize  int64
}

// TreeSummary contains metadata collected during pass 1 of CAR generation.
//
// This structure enables **two-pass CAR generation** where the first pass builds
// metadata without retaining all blocks, and the second pass writes the CAR with
// on-demand block regeneration. This architecture allows size pre-calculation for
// TUS uploads while bounding memory usage.
//
// **Structure Overview:**
//
//   - RootCID: Top-level IPFS content identifier pointing to the directory tree root
//   - TotalSize: Sum of all block sizes in bytes (UnixFS file + directory blocks)
//   - BlockOrder: Deterministic block ordering (implementation-specific, per CAR spec)
//   - BlockSizes: Parallel array to BlockOrder with size in bytes for each block
//   - TreeEntries: Path-based index of all filesystem entries (files and directories)
//   - CIDToEntry: CID-based index for O(1) entry lookup during block regeneration
//   - CARSize: Total file size including CARv1 header and all block framing overhead
//
// **Key Design Principles:**
//
//  1. **Dual Indexing:** TreeEntries (path-based) and CIDToEntry (CID-based) enable
//     different access patterns during different phases. TreeEntries is used during
//     BuildSummary for tree construction, while CIDToEntry is used during WriteCAR
//     for block regeneration.
//
//  2. **No Block Retention:** Blocks are NOT stored in TreeSummary after BuildSummary.
//     They are regenerated during WriteCAR using the filesystem reference stored
//     in CARBuilder. This bounds memory usage regardless of CAR file size.
//
//  3. **Deterministic Order:** BlockOrder ensures blocks are written in a consistent
//     order for each operation, but the specific order may differ between BuildSummary
//     and ReadCAR due to different traversal algorithms. This is acceptable per the CAR
//     specification, which does not mandate a specific block order.
//
//  4. **Virtual ROOT:** TreeEntries always contains a special "ROOT" entry that acts
//     as a virtual root directory for wrapInDir=true mode. This enables directory
//     wrapping without double-nesting.
//
// **Validity Rules and Invariants:**
//
//  1. **ROOT Entry Required:** TreeEntries["ROOT"] must always exist. It is a virtual
//     directory entry with Path="ROOT") that serves as the tree root in wrapInDir mode.
//
//  2. **BlockOrder/BlockSizes Alignment:** Length(BlockOrder) must equal Length(BlockSizes).
//     These parallel arrays must be updated together whenever blocks are added or modified.
//
//  3. **No Undef Root CID:** RootCID must not be cid.Undef for a valid CAR file structure.
//     The only exception is during BuildSummary before the root directory block is created.
//
//  4. **Path Consistency:** For each TreeEntry:
//     - Path is the parent directory path (empty string "" for root-level entries)
//     - Name is the basename of the entry
//     - The TreeEntries map key is the full path (e.g., "file.txt" for root-level files,
//     "dir1/file.txt" for nested files)
//
//  5. **ROOT.Children Only Files:** ROOT.Children array contains ONLY root-level files,
//     not directories. Root-level directories exist in TreeEntries but are NOT added
//     to ROOT.Children. This is a critical design decision for round-trip compatibility.
//     Note: While root-level directories are not in ROOT.Children, they ARE added as links
//     in the ROOT directory block's children list during block creation for proper UnixFS
//     structure.
//
//  6. **Non-ROOT Directories Contain All Children:** Unlike ROOT, non-ROOT directories
//     contain both files AND subdirectories in their Children arrays.
//
//  7. **Directory CID Lifecycle:** Directory entries start with CID.Undef during the
//     filesystem walk. CIDs are assigned after all child CIDs are known (deep-first
//     processing required by UnixFS Merkle DAG).
//
// **Directory Representation Rules:**
//
// Directories are represented differently based on their position in the tree:
//
//   - **ROOT (Virtual):** Path="ROOT", IsDir=true, Children=root-level files only
//   - **Root-Level Directories:** Path="", IsDir=true, NOT in ROOT.Children, Children contain nested items
//   - **Non-Root Directories:** Path=parent path, IsDir=true, in parent's Children array
//
// Example directory structure and representation:
//
//	Filesystem:
//
//	  file.txt                  (root-level file)
//	  dir1/                     (root-level directory)
//	  dir1/file1.txt           (nested file)
//	  dir1/subdir1/             (nested directory)
//	  dir1/subdir1/file2.txt   (deeply nested file)
//
//	TreeEntries map:
//
//	  "ROOT" -> {Path: "ROOT", IsDir: true, Children: ["file.txt"]}
//	  "file.txt" -> {Path: "", IsDir: false}
//	  "dir1" -> {Path: "", IsDir: true, Children: ["dir1/file1.txt", "dir1/subdir1"]}
//	  "dir1/file1.txt" -> {Path: "dir1", IsDir: false}
//	  "dir1/subdir1" -> {Path: "dir1", IsDir: true, Children: ["dir1/subdir1/file2.txt"]}
//	  "dir1/subdir1/file2.txt" -> {Path: "dir1/subdir1", IsDir: false}
//
// Note that "dir1" is NOT in ROOT.Children, but "file.txt" is.
//
// **CIDToEntry Multiplicity:**
//
//   - **Files:** Multiple block CIDs (from UnixFS chunking) map to the same TreeEntry.
//     This enables block regeneration when the LRU cache evicts blocks - any file chunk
//     CID can be used to locate the TreeEntry containing the original file path.
//   - **Directories:** Single CID maps to the directory's TreeEntry
//   - **Root CID:** Maps to ROOT entry (wrapInDir=true) or file entry (wrapInDir=false)
//
// When directory CIDs change (e.g., after empty directory pruning), CIDToEntry must be
// updated: delete(oldCID), entry.CID = newCID, CIDToEntry[newCID] = entry.
//
// **Empty Directory Pruning:**
//
// Empty directories (directories with no file descendants, direct or indirect) are
// removed during BuildSummary. Pruning follows these rules:
//
//  1. **Recursive Detection:** isDirectoryEmpty() recursively checks all descendants
//     for any file. A directory with empty subdirectories only is considered empty.
//
//  2. **Parent Cleanup:** When a directory is pruned, it is removed from its parent's
//     Children array.
//
//  3. **Block Removal:** The pruned directory's block CID is removed from BlockOrder,
//     BlockSizes, and CIDToEntry.
//
//  4. **CID Regeneration:** ALL parent directories must have their blocks regenerated
//     because removing a child changes the directory's content hash. This requires a
//     second pass through directories sorted deep-first.
//
//  5. **ROOT Exclusion:** The ROOT entry is explicitly excluded from pruning consideration,
//     even if empty, to preserve the virtual root structure.
//
// **Block Order:**
//
// BlockOrder contains all CIDs in the order they must be written to the CAR file:
//
//  1. File blocks are added during filesystem walk (in arbitrary order)
//  2. Directory blocks are added after all directories are sorted by depth descending
//     (deepest first) so that children CIDs are known before parents
//  3. After pruning, all remaining directory blocks are regenerated (CIDs may change)
//
// The deep-first order is required because UnixFS directory block CIDs depend on
// their children's CIDs (Merkle DAG property).
//
// **Single File Mode (wrapInDir=false):**
//
// When wrapInDir=false and the filesystem contains exactly one file with no directories:
//
//   - RootCID is assigned directly to the file's CID (no root directory wrapping)
//   - No directory blocks are created
//   - CAR header roots: [fileCID] instead of [directoryCID]
//   - TreeEntries still contains ROOT wrapper with the file in its Children array
//
// This optimization simplifies single-file CAR handling and matches the behavior of
// `ipfs add` without the `--wrap-with-directory` flag.
//
// **Path Handling:**
//
// Paths are always constructed with forward slash "/" separators, even on Windows:
//
//   - No leading slash (relative paths from filesystem root)
//   - No trailing slash on directories
//   - No double slashes
//   - Special path values: "" (root level), "ROOT" (virtual root), "." (current dir)
//
// The "." (CurrentDir) special case occurs with single-file filesystem wrappers like
// testBytesFS where "." opens directly to a file. Normally "." is skipped as a directory,
// but in this case it's processed as the sole file.
//
// **Round-Trip Compatibility:**
//
// TreeSummary structure is designed to enable lossless round-trip operations:
//
//	BuildSummary → WriteCAR → ReadCAR → reconstructTree = structurally identical TreeSummary
//
// **Round-Trip Guarantees:**
//
// The following fields are guaranteed to be identical after round-trip:
//   - RootCID: The same root content identifier
//   - TreeEntries: Same entries with same structure
//   - CIDToEntry: Same CID → Entry mappings
//   - TotalSize: Same total block size
//
// **BlockOrder and BlockSizes:**
//
// BlockOrder and BlockSizes may differ between BuildSummary and ReadCAR due to
// different traversal algorithms:
//   - BuildSummary uses filesystem walk (files first, then directories depth-first)
//   - ReadCAR uses BFS traversal starting from root
//
// This difference is **acceptable per the CAR specification**, which does not mandate
// a specific block order. Different deterministic orderings are valid.
//
// The ROOT.Children = files-only rule, root-level directory handling, and deep-first
// directory block creation are all designed to ensure that a CAR written from this
// structure can be reconstructed back to the same structure via ReadCAR.
//
// **Memory Usage:**
//
// TreeSummary itself is compact (O(number of entries) memory). The memory-intensive
// operation is block creation during BuildSummary, which uses a bounded blockstore
// (default 100MB limit via NewDAGServiceWithMemoryLimit). Blocks are not stored in
// TreeSummary after creation.
type TreeSummary struct {
	RootCID     cid.Cid
	TotalSize   uint64
	BlockOrder  []cid.Cid
	BlockSizes  []uint64
	TreeEntries map[string]*TreeEntry
	CIDToEntry  map[cid.Cid]*TreeEntry // CID -> Entry map for O(1) lookups
	CARSize     uint64
}

// TotalLogicalFileSize returns the sum of all logical file sizes in the summary.
// This represents the total size of all files in the CAR before chunking.
func (ts *TreeSummary) TotalLogicalFileSize() uint64 {
	var total uint64
	for _, entry := range ts.TreeEntries {
		if !entry.IsDir && entry.LogicalFileSize > 0 {
			total += entry.LogicalFileSize
		}
	}
	return total
}

// Equal compares two TreeSummary instances for equality.
// It performs deep comparison of all fields, treating BlockOrder+BlockSizes as sets
// (not ordered) to account for different valid block orderings per CAR specification.
//
// Time complexity: O(n + m) where n = blocks, m = tree entries
// Space complexity: O(k) where k = unique blocks after deduplication
func (ts *TreeSummary) Equal(other *TreeSummary) bool {
	if ts == nil && other == nil {
		return true
	}
	if ts == nil || other == nil {
		return false
	}

	// Fast-path: compare simple scalar fields first
	if !ts.RootCID.Equals(other.RootCID) {
		return false
	}
	if ts.TotalSize != other.TotalSize {
		return false
	}
	if ts.CARSize != other.CARSize {
		return false
	}

	// Check mapping counts for early rejection (invariant consistency)
	if len(ts.CIDToEntry) != len(other.CIDToEntry) {
		return false
	}

	// Compare BlockOrder and BlockSizes as sets (CID → size mapping)
	if !ts.equalBlocks(other) {
		return false
	}

	// Compare TreeEntries maps
	if !ts.equalTreeEntries(other) {
		return false
	}

	return true
}

// equalBlocks compares BlockOrder and BlockSizes as a set of CID→blockSize mappings.
// This allows for different valid block orderings (CAR specification does not mandate order).
//
// Time complexity: O(n) where n is the number of blocks
// Space complexity: O(k) where k is the number of unique blocks (after deduplication)
//
// Optimization: Single map builds CID→(size,count) mapping once, then verifies
// the other side by lookup and count decrement. Tracking both size and count handles
// duplicate CIDs correctly (multiple files can share the same block content).
func (ts *TreeSummary) equalBlocks(other *TreeSummary) bool {
	// Early rejection on length mismatches
	if len(ts.BlockOrder) != len(other.BlockOrder) {
		return false
	}
	if len(ts.BlockSizes) != len(other.BlockSizes) {
		return false
	}
	if len(ts.BlockOrder) != len(ts.BlockSizes) {
		return false
	}
	if len(other.BlockOrder) != len(other.BlockSizes) {
		return false
	}

	// Build map from ts: CID → count and size
	blockMap := make(map[cid.Cid]blockInfo, len(ts.BlockOrder))
	for i, block := range ts.BlockOrder {
		info := blockMap[block]
		info.size = ts.BlockSizes[i]
		info.count++
		blockMap[block] = info
	}

	// Verify blocks in other match
	for i, block := range other.BlockOrder {
		info, exists := blockMap[block]
		if !exists || info.size != other.BlockSizes[i] {
			return false
		}
		info.count--
		if info.count == 0 {
			delete(blockMap, block)
		} else {
			blockMap[block] = info
		}
	}

	// All entries should have been matched
	return len(blockMap) == 0
}

// equalTreeEntries compares TreeEntries maps for equality.
//
// Time complexity: O(m) where m is the number of tree entries
// Space complexity: O(1) - no additional allocations
//
// Early exits on structural mismatches before doing deep comparisons.
func (ts *TreeSummary) equalTreeEntries(other *TreeSummary) bool {
	// Early rejection on different entry counts
	if len(ts.TreeEntries) != len(other.TreeEntries) {
		return false
	}

	// Check CIDToEntry consistency as an additional invariant
	// These maps should have the same length as they contain the same CIDs
	if len(ts.CIDToEntry) != len(other.CIDToEntry) {
		return false
	}

	// Compare each tree entry
	for path, entry := range ts.TreeEntries {
		otherEntry, exists := other.TreeEntries[path]
		if !exists || !entry.Equal(otherEntry) {
			return false
		}
	}

	return true
}

// TreeEntry represents a single entry (file or directory) in the filesystem tree.
//
// Each TreeEntry is stored in the TreeEntries map, keyed by its full path relative
// to the filesystem root. The entry contains enough information to reconstruct the
// directory hierarchy and to regenerate blocks during CAR writing.
//
// **Path vs Name Semantics:**
//
//   - **Name** is the basename of the entry (e.g., "file.txt" for "dir1/file.txt")
//   - **Path** is the parent directory path (e.g., "dir1" for "dir1/file.txt")
//   - **TreeEntries Key** is the full path (e.g., "dir1/file.txt")
//
// This separation enables efficient parent-child relationship building and path
// construction during filesystem walks and DAG traversals.
//
// Example:
//
//	For entry "dir1/subdir1/file2.txt":
//	  - Name = "file2.txt"
//	  - Path = "dir1/subdir1"
//	  - TreeEntries key = "dir1/subdir1/file2.txt"
//
//	For root-level entry "file.txt":
//	  - Name = "file.txt"
//	  - Path = "" (empty string indicates no parent)
//	  - TreeEntries key = "file.txt"
//
//	For ROOT entry:
//	  - Name = "ROOT"
//	  - Path = "ROOT"
//	  - TreeEntries key = "ROOT"
//
// **Path Values and Meanings:**
//
//   - "": Root level (no parent directory)
//   - "ROOT": Special virtual root (only for the ROOT entry itself)
//   - "dir1/dir2": Nested directory path (parent is "dir1/dir2")
//   - ".": Current directory (only seen with single-file wrappers; normally skipped)
//
// Note: Paths are always constructed with "/" separators, never with backslashes
// (even on Windows), to ensure cross-platform consistency.
//
// **Directory vs File Representation:**
//
//   - **Files:** IsDir=false, have a non-undef CID (UnixFS node)
//   - **Directories:** IsDir=true, CID may be Undef initially (assigned during block creation)
//
// **Children Array:**
//
//   - Contains **path strings** (not TreeEntry references) to avoid circular references
//   - For directories: Full paths of children (e.g., ["dir1/file.txt", "dir1/subdir1"])
//   - For files: Always empty (files are leaves in the tree)
//
// **Children Construction Rules:**
//
//  1. **ROOT (virtual):** Children contains ONLY root-level files, NOT root-level directories.
//     Root-level directories exist in TreeEntries but are NOT added to ROOT.Children.
//     This is a critical design rule for round-trip compatibility.
//
//  2. **Root-Level Directories (Path=""):** Children contain nested items but are NOT
//     added to ROOT.Children. Example: "dir1" Children = ["dir1/file.txt", "dir1/subdir1"]
//
//  3. **Non-ROOT Directories:** Children contain BOTH files AND subdirectories.
//     Example: "dir1/subdir1" Children = ["dir1/subdir1/file2.txt"]
//
//  4. **Files:** Children array is empty ([]string{})
//
// **CID Lifecycle:**
//
//   - **Files:** CID is assigned immediately during UnixFS node creation and never changes
//   - **Directories:** CID starts as cid.Undef during filesystem walk, then is assigned
//     after the directory block is created. Directory CIDs can change when children are
//     modified (e.g., after empty directory pruning), requiring regeneration.
//
// **LogicalFileSize vs ChunkSize:**
//
//   - **LogicalFileSize:** Original file size in bytes as stored in UnixFS metadata.
//     This is the actual file content size before any chunking. Only set for files
//     (not directories).
//
//   - **ChunkSize:** UnixFS chunking boundary size in bytes (default 1MB). Controls how
//     a file is split into individual blocks during UnixFS node creation. Larger chunks
//     result in fewer blocks. Only set for files.
//
// Relationship example:
//
//	LogicalFileSize: 5,242,880 bytes (5MB)
//	ChunkSize: 1,048,576 bytes (1MB)
//	Result: File is split into 5 UnixFS blocks (4 full + 1 partial)
//
// **Usage in CAR Generation Phases:**
//
//	**Phase 1 (BuildSummary):**
//	  - Created during fs.WalkDir with Path and Name extracted from filesystem paths
//	  - Files get CID immediately from UnixFS node creation
//	  - Directories get CID assigned later during directory block creation
//	  - Parent-child relationships built via Children arrays
//
//	**Phase 2 (WriteCAR):**
//	  - Path+Name used to reopen files from filesystem for block regeneration
//	  - Children are used to reconstruct directory blocks on demand
//	  - CID used to identify which block to regenerate (via CIDToEntry map)
//
//	**Phase 3 (ReadCAR/reconstructTree):**
//	  - Path and Name reconstructed during DAG traversal (CAR doesn't store paths)
//	  - Children arrays rebuilt by walking UnixFS directory link structures
//	  - CID extracted from IPFS blocks directly
//
// **Special Cases:**
//
//  1. **ROOT Entry (Virtual):**
//     Always exists with Path="ROOT", Name="ROOT", IsDir=true. This is a synthetic
//     entry that acts as the tree root in wrapInDir=true mode. It enables directory
//     wrapping without creating an extra physical directory in IPFS.
//
//  2. **Single-File Mode (CurrentDir="."):**
//     For single-file filesystem wrappers like testBytesFS, the path "." is processed
//     as a file (not skipped as a directory). The entry has Name from the filesystem
//     (e.g., "test.txt") and Path="" to treat it as a root-level entry.
//
//  3. **Empty Directory Pruning:**
//     When a directory is removed (no file descendants), it is removed from its
//     parent's Children array. The TreeEntry itself remains in TreeEntries map
//     (for bookkeeping) but is not included in final CAR output.
//
// **Validity Rules:**
//
//  1. **Path Consistency:** If Path == "ROOT", then Name must also be "ROOT"
//  2. **Directory CID:** Directory entries may have cid.Undef only before block creation
//  3. **File CID:** File entries must have non-undefined CID at all times
//  4. **Children Type:** For directories, Children contains path strings; for files, empty
//  5. **Non-emptiness:** All fields except CID, Children, and Path must be non-zero/empty.
//     Path may be empty string "" for root-level entries
type TreeEntry struct {
	Path            string
	Name            string
	IsDir           bool
	CID             cid.Cid
	Children        []string
	ChunkSize       int64
	LogicalFileSize uint64 // UnixFS logical file size (actual file size before chunking)
}

// Equal compares two TreeEntry instances for equality.
func (te *TreeEntry) Equal(other *TreeEntry) bool {
	if te == nil && other == nil {
		return true
	}
	if te == nil || other == nil {
		return false
	}

	// Compare simple fields
	if te.Path != other.Path {
		return false
	}
	if te.Name != other.Name {
		return false
	}
	if te.IsDir != other.IsDir {
		return false
	}
	if !te.CID.Equals(other.CID) {
		return false
	}
	if te.ChunkSize != other.ChunkSize {
		return false
	}
	if te.LogicalFileSize != other.LogicalFileSize {
		return false
	}

	// Compare Children arrays
	// Explicitly check for nil vs non-nil to distinguish between uninitialized and empty
	if (te.Children == nil) != (other.Children == nil) {
		return false
	}
	if len(te.Children) != len(other.Children) {
		return false
	}
	for i, child := range te.Children {
		if child != other.Children[i] {
			return false
		}
	}

	return true
}

// blockInfo tracks block size and occurrence count for CID equality comparison.
// Used by TreeSummary.equalBlocks to handle duplicate CIDs correctly.
type blockInfo struct {
	size  uint64
	count int
}

// CARBuilderOption is a function that configures a CARBuilder.
type CARBuilderOption func(*CARBuilder)

// WithChunkSize sets the chunk size (in bytes) for UnixFS file splitting.
// The default is 1MB (1024 * 1024).
func WithChunkSize(chunkSize int64) CARBuilderOption {
	return func(b *CARBuilder) {
		b.chunkSize = chunkSize
	}
}

// NewCARBuilder creates a new CARBuilder with the specified blockstore, DAG service, and UnixFS node generator.
// If bs or dagService is nil, a new LevelBlockStore is created.
func NewCARBuilder(bs blockstore.Blockstore, dagService format.DAGService, generator unixfs.UnixFSNodeGenerator, opts ...CARBuilderOption) *CARBuilder {
	if bs == nil || dagService == nil {
		bs, dagService = NewDAGServiceWithLevelAware()
	}

	b := &CARBuilder{
		bs:         bs,
		dagService: dagService,
		generator:  generator,
		chunkSize:  1024 * 1024,
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

// BuildSummary performs pass 1: walks the filesystem and builds the UnixFS DAG,
// collecting metadata without retaining blocks.
func (b *CARBuilder) BuildSummary(ctx context.Context, filesystem fs.FS, wrapInDir bool) (*TreeSummary, error) {
	b.filesystem = filesystem
	b.wrapInDir = wrapInDir

	summary := &TreeSummary{
		TreeEntries: make(map[string]*TreeEntry),
		CIDToEntry:  make(map[cid.Cid]*TreeEntry),
	}

	summary.TreeEntries[ROOT] = &TreeEntry{
		Name:  ROOT,
		IsDir: true,
		Path:  ROOT,
	}

	if err := fs.WalkDir(filesystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			return err
		}

		if path == "" || path == ROOT || path == ParentDir {
			return nil
		}

		// Handle special case: CurrentDir(".") when it's actually a file.
		// This occurs with single-file filesystem wrappers like testBytesFS.
		// If "." is a directory, skip it (normal filesystem behavior).
		if path == CurrentDir {
			if d.IsDir() {
				return nil
			}
			// If "." is a file, process it as the solitary file at root
		}

		entry := &TreeEntry{
			Name:  d.Name(),
			IsDir: d.IsDir(),
		}

		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			entry.Path = path[:idx]
		} else {
			entry.Path = ""
		}

		if !d.IsDir() {
			file, err := filesystem.Open(path)
			if err != nil {
				return err
			}

			rootCID, blocks, blockSizes, logicalFileSize, err := b.createUnixFSBlocks(ctx, file)
			closeErr := file.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}

			entry.CID = rootCID
			entry.ChunkSize = b.chunkSize
			entry.LogicalFileSize = logicalFileSize

			for i, blockCID := range blocks {
				summary.BlockOrder = append(summary.BlockOrder, blockCID)
				summary.BlockSizes = append(summary.BlockSizes, blockSizes[i])
				summary.TotalSize += blockSizes[i]
				summary.CIDToEntry[blockCID] = entry

			}
		}

		summary.TreeEntries[path] = entry
		if entry.CID != cid.Undef {
			summary.CIDToEntry[entry.CID] = entry
		}

		// Build parent-child relationship during walk
		//
		// Rules:
		// 1. ROOT only contains FILES, not directories at the top level
		// 2. Non-ROOT directories contain BOTH files AND subdirectories
		// 3. Files are added to their parent directory's Children
		// 4. Subdirectories are added to their parent directory's Children

		if entry.Path != "" {
			// Inside a directory - add to that directory's children
			// (both files and subdirectories)
			parent := summary.TreeEntries[entry.Path]
			if parent != nil {
				parent.Children = append(parent.Children, path)
			}
		} else {
			// At root level
			if d.IsDir() {
				// Directory at root level - do NOT add to ROOT.Children
				// ROOT represents the virtual root, not the real filesystem root
			} else {
				// File at root level - add to ROOT.Children
				rootChildren := summary.TreeEntries[ROOT].Children
				summary.TreeEntries[ROOT].Children = append(rootChildren, path)
			}
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk failed: %w", err)
	}

	if !wrapInDir {
		var rootLevelFiles []string
		var hasDirectories bool

		for path, entry := range summary.TreeEntries {
			if path == ROOT {
				continue
			}
			if entry.IsDir {
				hasDirectories = true
				break
			}
			if entry.Path == "" {
				rootLevelFiles = append(rootLevelFiles, path)
			}
		}

		if !hasDirectories && len(rootLevelFiles) == 1 {
			fileEntry := summary.TreeEntries[rootLevelFiles[0]]
			if fileEntry.CID != cid.Undef {
				summary.RootCID = fileEntry.CID
				b.summary = summary
				return summary, nil
			}
		}
	}

	// Build directory blocks - already have parent-child relationships from walk
	// Collect only directories
	dirPaths := make([]string, 0, len(summary.TreeEntries))
	for path, entry := range summary.TreeEntries {
		if entry.IsDir {
			dirPaths = append(dirPaths, path)
		}
	}

	// Sort by depth descending (deepest first) to process children before parents
	sort.Slice(dirPaths, func(i, j int) bool {
		if dirPaths[i] == ROOT {
			return false
		}
		if dirPaths[j] == ROOT {
			return true
		}
		return pathDepth(dirPaths[i]) > pathDepth(dirPaths[j])
	})

	for _, path := range dirPaths {
		entry := summary.TreeEntries[path]

		dirCID, blockSize, err := b.createDirectoryBlock(ctx, entry, summary.TreeEntries)
		if err != nil {
			return nil, err
		}

		// Update CIDToEntry if CID changed
		if entry.CID != cid.Undef {
			delete(summary.CIDToEntry, entry.CID)
		}
		entry.CID = dirCID
		summary.CIDToEntry[dirCID] = entry

		summary.BlockOrder = append(summary.BlockOrder, dirCID)
		summary.BlockSizes = append(summary.BlockSizes, blockSize)
		summary.TotalSize += blockSize
	}

	root := summary.TreeEntries[ROOT]
	if root == nil {
		return nil, fmt.Errorf("root not found")
	}

	summary.RootCID = root.CID

	if err := b.pruneEmptyDirectories(summary); err != nil {
		return nil, fmt.Errorf("prune empty directories: %w", err)
	}

	// After pruning, regenerate all directory blocks from bottom up
	// because removing children from a parent changes its content and thus its CID
	dirPaths = dirPaths[:0] // Reuse existing slice
	for path, entry := range summary.TreeEntries {
		if entry.IsDir {
			dirPaths = append(dirPaths, path)
		}
	}

	sort.Slice(dirPaths, func(i, j int) bool {
		if dirPaths[i] == ROOT {
			return false
		}
		if dirPaths[j] == ROOT {
			return true
		}
		return pathDepth(dirPaths[i]) > pathDepth(dirPaths[j])
	})

	for _, path := range dirPaths {
		entry := summary.TreeEntries[path]
		if entry == nil {
			continue
		}

		oldCID := entry.CID
		dirCID, blockSize, err := b.createDirectoryBlock(ctx, entry, summary.TreeEntries)
		if err != nil {
			return nil, fmt.Errorf("regenerate directory %s: %w", path, err)
		}

		// Update CIDToEntry
		if oldCID != cid.Undef {
			delete(summary.CIDToEntry, oldCID)
		}
		entry.CID = dirCID
		summary.CIDToEntry[dirCID] = entry

		// Update BlockOrder and BlockSizes
		found := false
		for i, blockCID := range summary.BlockOrder {
			if blockCID.Equals(oldCID) {
				summary.BlockOrder[i] = dirCID
				summary.BlockSizes[i] = blockSize
				found = true
				break
			}
		}

		if !found && oldCID != cid.Undef && len(entry.Children) > 0 {
			summary.BlockOrder = append(summary.BlockOrder, dirCID)
			summary.BlockSizes = append(summary.BlockSizes, blockSize)
		}
	}

	// Update RootCID to the final root CID after all regeneration
	summary.RootCID = root.CID

	b.summary = summary
	return summary, nil
}

// WriteCAR performs pass 2: writes CARv1 using the summary,
// regenerating blocks from the filesystem on the fly.
func (b *CARBuilder) WriteCAR(ctx context.Context, w io.Writer) error {
	if b.summary == nil {
		return fmt.Errorf("summary not built, call BuildSummary first")
	}

	v1Header := &carv2.CarHeader{
		Version: 1,
		Roots:   []cid.Cid{b.summary.RootCID},
	}
	if err := carv2.WriteHeader(v1Header, w); err != nil {
		return fmt.Errorf("write CARv1 header: %w", err)
	}

	for _, blockCID := range b.summary.BlockOrder {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := b.writeBlockToCAR(ctx, blockCID, w); err != nil {
			return fmt.Errorf("write block %s: %w", blockCID, err)
		}
	}

	return nil
}

// BuildAndWrite is a convenience method that performs both passes in sequence.
func (b *CARBuilder) BuildAndWrite(ctx context.Context, filesystem fs.FS, w io.Writer, wrapInDir bool) (cid.Cid, error) {
	summary, err := b.BuildSummary(ctx, filesystem, wrapInDir)
	if err != nil {
		return cid.Cid{}, err
	}

	rootCID := encoding.NormalizeCid(summary.RootCID)

	if err := b.WriteCAR(ctx, w); err != nil {
		return cid.Cid{}, err
	}

	return rootCID, nil
}

// GetSummary returns the tree summary after BuildSummary has been called.
func (b *CARBuilder) GetSummary() *TreeSummary {
	return b.summary
}

// collectAllBlocks performs a BFS traversal to collect all unique CIDs in the DAG tree.
// This ensures CAR deduplication works correctly: blocks with identical content are
// stored only once, even if referenced multiple times in the DAG.
func (b *CARBuilder) collectAllBlocks(ctx context.Context, rootCID cid.Cid) ([]cid.Cid, []uint64, error) {
	queue := []cid.Cid{rootCID}
	seen := make(map[cid.Cid]bool)
	var allCIDs []cid.Cid
	var allSizes []uint64

	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}

		currentCID := queue[0]
		queue = queue[1:]

		if seen[currentCID] {
			continue
		}
		seen[currentCID] = true

		blk, err := b.bs.Get(ctx, currentCID)
		if err != nil {
			return nil, nil, fmt.Errorf("get block %s: %w", currentCID, err)
		}

		allCIDs = append(allCIDs, currentCID)
		allSizes = append(allSizes, uint64(len(blk.RawData())))

		// Decode links from the block using encoding.DecodeBlock
		// This handles both dag-pb (ProtoNode) and raw leaf blocks
		node, err := encoding.DecodeBlock(ctx, blk)
		if err != nil {
			return nil, nil, fmt.Errorf("decode block %s: %w", currentCID, err)
		}

		// Add child CIDs to queue for traversal
		for _, link := range node.Links() {
			if link != nil && !seen[link.Cid] {
				queue = append(queue, link.Cid)
			}
		}
	}

	return allCIDs, allSizes, nil
}

func (b *CARBuilder) createUnixFSBlocks(ctx context.Context, r io.Reader) (cid.Cid, []cid.Cid, []uint64, uint64, error) {
	if err := ctx.Err(); err != nil {
		return cid.Cid{}, nil, nil, 0, err
	}

	nd, err := b.generator.CreateUnixFSNode(ctx, internalio.NewReadSeekCloser(r), helpers.DefaultLinksPerBlock, b.chunkSize)
	if err != nil {
		return cid.Cid{}, nil, nil, 0, fmt.Errorf("create unixfs node: %w", err)
	}

	rootCID := nd.Cid()

	// Extract UnixFS logical file size using dagnode.AnalyzeNode
	// format.Node embeds blocks.Block, so we can pass nd directly
	nodeInfo, err := dagnode.AnalyzeNode(ctx, nd)
	if err != nil {
		return cid.Cid{}, nil, nil, 0, fmt.Errorf("analyze node: %w", err)
	}

	logicalFileSize := nodeInfo.FileSize

	allCIDs, allSizes, err := b.collectAllBlocks(ctx, rootCID)
	if err != nil {
		return cid.Cid{}, nil, nil, 0, fmt.Errorf("collect all blocks: %w", err)
	}

	return rootCID, allCIDs, allSizes, logicalFileSize, nil
}

func (b *CARBuilder) createDirectoryBlock(ctx context.Context, entry *TreeEntry, entries map[string]*TreeEntry) (cid.Cid, uint64, error) {
	if err := ctx.Err(); err != nil {
		return cid.Cid{}, 0, err
	}

	children := lo.FilterMap(entry.Children, func(childPath string, _ int) (unixfs.DirectoryChild, bool) {
		child := entries[childPath]
		if child == nil || child.CID == cid.Undef {
			return unixfs.DirectoryChild{}, false
		}
		return unixfs.DirectoryChild{
			Name: child.Name,
			CID:  child.CID,
			Size: uint64(child.CID.ByteLen()),
		}, true
	})

	// SPECIAL CASE: For ROOT, add root-level directories
	// Root-level directories have Path="" and are not in ROOT.Children
	// They must be added as links in ROOT's UnixFS directory block for round-trip compatibility
	if entry.Name == ROOT {
		// Collect root-level directories first to ensure deterministic ordering
		var rootDirs []string
		for path, child := range entries {
			// Skip if not a directory, is ROOT itself, or is nested (Path != "")
			if !child.IsDir || path == ROOT || child.Path != "" {
				continue
			}
			rootDirs = append(rootDirs, path)
		}
		// Sort to ensure deterministic CID generation
		sort.Strings(rootDirs)
		
		for _, path := range rootDirs {
			child := entries[path]
			
			// Check for duplicates (in case directory is already in Children)
			var alreadyInChildren bool
			for _, dc := range children {
				if dc.Name == child.Name {
					alreadyInChildren = true
					break
				}
			}

			// Add directory link if not already present
			if !alreadyInChildren {
				children = append(children, unixfs.DirectoryChild{
					Name: child.Name,
					CID:  child.CID,
					Size: uint64(child.CID.ByteLen()),
				})
			}
		}
	}

	node, err := b.generator.CreateDirectoryWithLinks(ctx, children)
	if err != nil {
		return cid.Cid{}, 0, err
	}

	return node.Cid(), uint64(len(node.RawData())), nil
}

func (b *CARBuilder) writeBlockToCAR(ctx context.Context, blockCID cid.Cid, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Try to fetch from blockstore first (LRU might have evicted it)
	blk, err := b.bs.Get(ctx, blockCID)
	if err == nil {
		return carv2util.WriteBlock(w, blockCID, blk.RawData())
	}

	// Block not in blockstore, regenerate it using the standard builders
	entry := b.summary.CIDToEntry[blockCID]
	if entry == nil {
		return fmt.Errorf("block not found in summary: %s", blockCID)
	}

	// Regenerate the block using the standard path (adds to blockstore)
	var file fs.File
	if entry.IsDir {
		_, _, err = b.createDirectoryBlock(ctx, entry, b.summary.TreeEntries)
	} else {
		filePath := filepath.Join(entry.Path, entry.Name)
		file, err = b.filesystem.Open(filePath)
		if err != nil {
			return err
		}
		defer file.Close()
		_, _, _, _, err = b.createUnixFSBlocks(ctx, file)
	}
	if err != nil {
		return fmt.Errorf("regenerate block %s: %w", blockCID, err)
	}

	// Fetch the regenerated block from blockstore
	blk, err = b.bs.Get(ctx, blockCID)
	if err != nil {
		return fmt.Errorf("fetch regenerated block %s: %w", blockCID, err)
	}

	return carv2util.WriteBlock(w, blockCID, blk.RawData())
}

func (b *CARBuilder) pruneEmptyDirectories(summary *TreeSummary) error {
	emptyDirs := make(map[string]bool)

	for path, entry := range summary.TreeEntries {
		if !entry.IsDir || path == ROOT {
			continue
		}

		if b.isDirectoryEmpty(summary, path) {
			emptyDirs[path] = true
		}
	}

	for path := range emptyDirs {
		entry := summary.TreeEntries[path]
		parentPath := entry.Path
		if parentPath == "" {
			parentPath = ROOT
		}

		parentEntry := summary.TreeEntries[parentPath]
		if parentEntry == nil {
			continue
		}

		parentEntry.Children = removeString(parentEntry.Children, path)

		if entry.CID != cid.Undef {
			for i, blockCID := range summary.BlockOrder {
				if blockCID.Equals(entry.CID) {
					summary.BlockOrder = append(summary.BlockOrder[:i], summary.BlockOrder[i+1:]...)
					summary.BlockSizes = append(summary.BlockSizes[:i], summary.BlockSizes[i+1:]...)
					delete(summary.CIDToEntry, entry.CID)
					break
				}
			}
		}
	}

	return nil
}

func (b *CARBuilder) isDirectoryEmpty(summary *TreeSummary, path string) bool {
	entry := summary.TreeEntries[path]
	if entry == nil || !entry.IsDir {
		return false
	}

	if len(entry.Children) == 0 {
		return true
	}

	for _, childPath := range entry.Children {
		child := summary.TreeEntries[childPath]
		if child == nil {
			continue
		}

		if !child.IsDir {
			return false
		}

		if !b.isDirectoryEmpty(summary, childPath) {
			return false
		}
	}

	return true
}

func removeString(slice []string, s string) []string {
	for i, item := range slice {
		if item == s {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func pathDepth(path string) int {
	return strings.Count(path, "/")
}
