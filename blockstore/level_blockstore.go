package blockstore

import (
	"context"
	"slices"
	"sync"

	boxoblockstore "github.com/ipfs/boxo/blockstore"
	blockformat "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/boxo/ipld/merkledag"
	legacy "github.com/ipfs/go-ipld-legacy"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/ipld/go-ipld-prime/node/basicnode"
)

const (
	// DefaultMaxLevels is the default maximum number of DAG levels to track
	DefaultMaxLevels = 10
)

// BlockInfo contains level and parent information for a block
type BlockInfo struct {
	level  int
	parent cid.Cid
}

// LevelCohort represents blocks at a specific DAG level (depth)
// Cohorts track level metadata and parent-child relationships
type LevelCohort struct {
	// blockInfo maps CID to block metadata (level and parent)
	blockInfo map[cid.Cid]BlockInfo
	
	// childrenOf tracks direct children relationships for a parent CID
	childrenOf map[cid.Cid][]cid.Cid
	
	// count is the number of blocks at this level
	count int
	
	// level is the DAG depth this cohort represents
	level int
}

// LevelBlockStore provides level-aware block storage with cohort-based tracking
// of DAG relationships. Uses unbounded InMemory blockstore for data storage.
// Cohorts rotate when exceeding maxLevels to limit metadata tracking to DAG depth.
type LevelBlockStore struct {
	// inner is the underlying unbounded blockstore
	inner boxoblockstore.Blockstore
	
	// cohorts maps levels to their cohorts
	cohorts map[int]*LevelCohort
	
	// levelMap is a global level cache for O(1) lookups
	levelMap map[cid.Cid]int
	
	// pendingChildren tracks CIDs that have parents waiting for them to be seen
	pendingChildren map[cid.Cid][]cid.Cid
	
	// Configuration
	maxLevels int
	
	// Tracking
	currentLevel int
	levelsSeen   map[int]bool // Track which levels we've seen
	blockCount   int         // Total blocks stored
	
	mu sync.RWMutex
}

// Global decoder registry for link extraction
var linkDecoder *legacy.Decoder

// init initializes the link decoder
func init() {
	// Initialize decoder for link extraction
	linkDecoder = legacy.NewDecoder()
	linkDecoder.RegisterCodec(cid.DagProtobuf, dagpb.Type.PBNode, merkledag.ProtoNodeConverter)
	linkDecoder.RegisterCodec(cid.Raw, basicnode.Prototype.Bytes, merkledag.RawNodeConverter)
}

// NewLevelBlockStore creates a new level-aware blockstore
func NewLevelBlockStore() *LevelBlockStore {
	return NewLevelBlockStoreWithLevels(DefaultMaxLevels)
}

// NewLevelBlockStoreWithLevels creates a new level-aware blockstore with custom max depth
func NewLevelBlockStoreWithLevels(maxLevels int) *LevelBlockStore {
	if maxLevels <= 0 {
		maxLevels = DefaultMaxLevels
	}
	
	lbs := &LevelBlockStore{
		inner:           NewInMemoryBlockstore(),
		cohorts:         make(map[int]*LevelCohort),
		levelMap:        make(map[cid.Cid]int),
		pendingChildren: make(map[cid.Cid][]cid.Cid),
		maxLevels:       maxLevels,
		levelsSeen:      make(map[int]bool),
	}
	
	return lbs
}

// Put stores a block and tracks its level and parent-child relationships
func (lbs *LevelBlockStore) Put(ctx context.Context, block blockformat.Block) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	
	lbs.mu.Lock()
	defer lbs.mu.Unlock()
	
	blockCID := block.Cid()
	
	// Extract links to identify children
	childrenLinks := extractLinks(block)
	
	// Determine if this is a leaf or parent node
	isLeaf := len(childrenLinks) == 0
	
	var level int
	if isLeaf {
		// Leaf nodes are always level 0
		level = 0
		lbs.currentLevel = level
	} else {
		// Parent nodes: need to calculate level from children
		// Collect child levels
		childLevels := make([]int, 0, len(childrenLinks))
		
		// Check which children we've seen
		for _, childCIDVal := range childrenLinks {
			if childLevel, exists := lbs.levelMap[childCIDVal]; exists {
				childLevels = append(childLevels, childLevel)
			}
		}
		
		// Calculate parent level
		if len(childLevels) == 0 {
			// No children seen yet - assume it's a parent at current level + 1
			level = lbs.currentLevel + 1
		} else {
			// Parent is one level above max child
			level = slices.Max(childLevels) + 1
		}
		
		lbs.currentLevel = level
	}
	
	// Store in global level map (metadata never evicted)
	lbs.levelMap[blockCID] = level
	lbs.levelsSeen[level] = true
	
	// Get or create cohort for this level
	cohort, exists := lbs.cohorts[level]
	if !exists {
		cohort = &LevelCohort{
			blockInfo:  make(map[cid.Cid]BlockInfo),
			childrenOf: make(map[cid.Cid][]cid.Cid),
			level:      level,
		}
		lbs.cohorts[level] = cohort
		
		// Rotate cohorts if we have too many levels
		lbs.rotateCohortsIfNecessary()
	}
	
	// Store in underlying blockstore
	if err := lbs.inner.Put(ctx, block); err != nil {
		return err
	}
	
	// Track block count
	lbs.blockCount++
	
	// Store in current cohort
	cohort.blockInfo[blockCID] = BlockInfo{level: level}
	cohort.count++
	
	// Track children relationship
	cohort.childrenOf[blockCID] = childrenLinks
	for _, childCIDVal := range childrenLinks {
		// O(1) lookup: use levelMap to find child's level, then directly access cohort
		if childLevel, exists := lbs.levelMap[childCIDVal]; exists {
			childCohort := lbs.cohorts[childLevel]
			if childCohort != nil {
				childCohort.blockInfo[childCIDVal] = BlockInfo{
					level:  childLevel,
					parent: blockCID,
				}
			}
		}
	}
	
	return nil
}

// rotateCohortsIfNecessary removes oldest level cohorts if we exceed maxLevels
func (lbs *LevelBlockStore) rotateCohortsIfNecessary() {
	// Check if we've exceeded max levels
	if len(lbs.cohorts) <= lbs.maxLevels {
		return
	}
	
	// Find the oldest level to evict (minimum level number)
	minLevel := -1
	for level := range lbs.cohorts {
		if minLevel == -1 || level < minLevel {
			minLevel = level
		}
	}
	
	if minLevel == -1 {
		return
	}
	
	// Remove the cohort for the oldest level
	oldest := lbs.cohorts[minLevel]
	if oldest != nil {
		// Remove block metadata from this level's cohort
		for cid := range oldest.blockInfo {
			delete(lbs.levelMap, cid)
		}
		delete(lbs.cohorts, minLevel)
		delete(lbs.levelsSeen, minLevel)
	}
}

// GetLevel returns the inferred level for a given CID, or -1 if unknown
func (lbs *LevelBlockStore) GetLevel(cid cid.Cid) int {
	lbs.mu.RLock()
	defer lbs.mu.RUnlock()
	
	if level, exists := lbs.levelMap[cid]; exists {
		return level
	}
	return -1
}

// Get retrieves a block from the blockstore
func (lbs *LevelBlockStore) Get(ctx context.Context, c cid.Cid) (blockformat.Block, error) {
	return lbs.inner.Get(ctx, c)
}

// Has checks if a block exists in the blockstore
func (lbs *LevelBlockStore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	return lbs.inner.Has(ctx, c)
}

// GetSize returns the size of a block in bytes
func (lbs *LevelBlockStore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	return lbs.inner.GetSize(ctx, c)
}

// GetChildren returns the direct children CIDs for a parent CID
func (lbs *LevelBlockStore) GetChildren(ctx context.Context, parentCID cid.Cid) ([]cid.Cid, error) {
	lbs.mu.RLock()
	defer lbs.mu.RUnlock()
	
	for _, cohort := range lbs.cohorts {
		if children, exists := cohort.childrenOf[parentCID]; exists {
			return children, nil
		}
	}
	
	return []cid.Cid{}, nil
}

// GetAncestors returns all ancestor CIDs for a given CID by walking up the parent chain
func (lbs *LevelBlockStore) GetAncestors(ctx context.Context, startCID cid.Cid) ([]cid.Cid, error) {
	lbs.mu.RLock()
	defer lbs.mu.RUnlock()
	
	ancestors := []cid.Cid{}
	currentCID := startCID
	
	for {
		var parent cid.Cid
		var found bool
		
		// Find which cohort has this parent info
		for _, cohort := range lbs.cohorts {
			if info, exists := cohort.blockInfo[currentCID]; exists {
				parent = info.parent
				found = true
				if !parent.Equals(currentCID) && !parent.Equals(cid.Cid{}) {
					ancestors = append(ancestors, parent)
				}
				break
			}
		}
		
		if !found || parent == currentCID {
			break
		}
		
		currentCID = parent
	}
	
	return ancestors, nil
}

// PutMany adds multiple blocks to the blockstore
func (lbs *LevelBlockStore) PutMany(ctx context.Context, blocks []blockformat.Block) error {
	for _, blk := range blocks {
		if err := lbs.Put(ctx, blk); err != nil {
			return err
		}
	}
	return nil
}

// DeleteBlock removes a block from the blockstore
func (lbs *LevelBlockStore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	lbs.mu.Lock()
	defer lbs.mu.Unlock()

	if err := lbs.inner.DeleteBlock(ctx, c); err != nil {
		return err
	}

	// Clean up metadata if exists
	if level, exists := lbs.levelMap[c]; exists {
		delete(lbs.levelMap, c)
		if cohort, ok := lbs.cohorts[level]; ok {
			delete(cohort.blockInfo, c)
			delete(cohort.childrenOf, c)
			cohort.count--
		}
		lbs.blockCount--
	}

	return nil
}

// AllKeysChan returns a channel with all block CIDs
func (lbs *LevelBlockStore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return lbs.inner.AllKeysChan(ctx)
}

// Close is a no-op for this blockstore
func (lbs *LevelBlockStore) Close() error {
	return nil
}

// Len returns the number of blocks in the blockstore
func (lbs *LevelBlockStore) Len() int {
	lbs.mu.RLock()
	defer lbs.mu.RUnlock()
	return lbs.blockCount
}

// GetInnerStore returns the underlying blockstore
func (lbs *LevelBlockStore) GetInnerStore() boxoblockstore.Blockstore {
	return lbs.inner
}

// extractLinks extracts CID links from a merkledag node
// Uses legacy decoder to handle various IPLD node formats (dag-pb, raw, etc.)
// Returns empty slice if the block cannot be decoded
func extractLinks(block blockformat.Block) []cid.Cid {
	ctx := context.Background()
	node, err := linkDecoder.DecodeNode(ctx, block)
	if err != nil {
		return []cid.Cid{}
	}
	
	links := node.Links()
	cidLinks := make([]cid.Cid, 0, len(links))
	
	for _, link := range links {
		if link != nil && len(link.Cid.Bytes()) > 0 {
			cidLinks = append(cidLinks, link.Cid)
		}
	}
	
	return cidLinks
}

// compile-time check that LevelBlockStore implements blockstore.Blockstore
var _ boxoblockstore.Blockstore = (*LevelBlockStore)(nil)
