package blockstore

import (
	"context"
	"fmt"
	"io"
	"sync"

	boxoblockstore "github.com/ipfs/boxo/blockstore"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"

	"go.lumeweb.com/ipfs-content/internal/carv1"
)

// IndexedBlockstore wraps an LRU blockstore and provides reloading from
// a seekable source (like a CAR file) using an offset index.
// It enables memory-bounded CAR reading with the ability to reload evicted blocks
// by seeking to their recorded positions in the original source.
type IndexedBlockstore struct {
	lru       *LRUBlockstore
	offsetMap map[string][]int64
	sizeMap   map[string]int64
	reader    io.ReadSeeker
	mu        sync.RWMutex
}

// NewIndexedBlockstore creates a new IndexedBlockstore with the specified memory limit.
// The reader must support seeking to enable block reloading on cache eviction.
func NewIndexedBlockstore(memoryLimit uint64, reader io.ReadSeeker) *IndexedBlockstore {
	return &IndexedBlockstore{
		lru:       NewLRUBlockstore(memoryLimit),
		offsetMap: make(map[string][]int64),
		sizeMap:   make(map[string]int64),
		reader:    reader,
	}
}

// IndexAll reads through the entire CAR stream and builds the offset index
// while loading all blocks into the LRU cache. This is a one-time operation
// that must be called before using the blockstore.
func (ibs *IndexedBlockstore) IndexAll(ctx context.Context) error {
	ibs.mu.Lock()
	defer ibs.mu.Unlock()

	// Create CAR reader
	cr, err := carv1.NewCarReader(ibs.reader)
	if err != nil {
		return fmt.Errorf("create CAR reader: %w", err)
	}

	// Iterate through all blocks
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled: %w", err)
		}

		// Capture offset before reading this block
		offset, err := ibs.reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("get current offset: %w", err)
		}

		// Read next block
		blk, err := cr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read block: %w", err)
		}

		// Store block in LRU cache
		if err := ibs.lru.Put(ctx, blk); err != nil {
			return fmt.Errorf("store block: %w", err)
		}

		// Record offset and size for this block
		cidStr := blk.Cid().String()
		ibs.offsetMap[cidStr] = append(ibs.offsetMap[cidStr], offset)
		ibs.sizeMap[cidStr] = int64(len(blk.RawData()))
	}

	return nil
}

// Get retrieves a block from the blockstore.
// If the block is not in the LRU cache, it is reloaded from the CAR file
// by seeking to the recorded offset.
func (ibs *IndexedBlockstore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	// Try cache first
	blk, err := ibs.lru.Get(ctx, c)
	if err == nil {
		return blk, nil
	}

	// Cache miss - look up offsets in index
	cidStr := c.String()
	ibs.mu.RLock()
	offsets, ok := ibs.offsetMap[cidStr]
	ibs.mu.RUnlock()

	if !ok || len(offsets) == 0 {
		return nil, fmt.Errorf("block not found in index: %s", c)
	}

	ibs.mu.Lock()
	defer ibs.mu.Unlock()

	// Try each offset until we find a valid block
	var lastError error
	for _, offset := range offsets {
		// Seek to block position
		_, err := ibs.reader.Seek(offset, io.SeekStart)
		if err != nil {
			lastError = fmt.Errorf("seek to offset %d: %w", offset, err)
			continue
		}

		// Read block at this position
		// Use a large maxReadBytes to allow reading large blocks
		cidRead, data, err := carv1.ReadNode(ibs.reader, false, 10*1024*1024)
		if err != nil {
			lastError = fmt.Errorf("read block at offset %d: %w", offset, err)
			continue
		}

		// Verify CID matches what we expected
		if !cidRead.Equals(c) {
			lastError = fmt.Errorf("CID mismatch at offset %d: expected %s, got %s", offset, c, cidRead)
			continue
		}

		// Create block from CID and data
		blk, err := blocks.NewBlockWithCid(data, cidRead)
		if err != nil {
			lastError = fmt.Errorf("create block: %w", err)
			continue
		}

		// Add to cache for future access
		if err := ibs.lru.Put(ctx, blk); err != nil {
			lastError = fmt.Errorf("add to cache: %w", err)
			continue
		}

		return blk, nil
	}

	// All offsets failed
	return nil, fmt.Errorf("failed to reload block %s from any offset (tried %d offsets): %w", c, len(offsets), lastError)
}

// Put stores a block in the blockstore.
// It only stores in the LRU cache and does not update the index.
func (ibs *IndexedBlockstore) Put(ctx context.Context, b blocks.Block) error {
	ibs.mu.Lock()
	defer ibs.mu.Unlock()
	return ibs.lru.Put(ctx, b)
}

// PutMany stores multiple blocks in the blockstore.
// It only stores in the LRU cache and does not update the index.
func (ibs *IndexedBlockstore) PutMany(ctx context.Context, blk []blocks.Block) error {
	ibs.mu.Lock()
	defer ibs.mu.Unlock()
	return ibs.lru.PutMany(ctx, blk)
}

// Has checks if a block exists in the blockstore.
// It returns true if the block is in the index (may be cached or can be reloaded).
func (ibs *IndexedBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("context cancelled: %w", err)
	}

	ibs.mu.RLock()
	_, ok := ibs.offsetMap[c.String()]
	ibs.mu.RUnlock()
	return ok, nil
}

// GetSize returns the size of a block in bytes.
// It returns the size from the index without loading the block.
func (ibs *IndexedBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	if err := ctx.Err(); err != nil {
		return -1, fmt.Errorf("context cancelled: %w", err)
	}

	ibs.mu.RLock()
	size, ok := ibs.sizeMap[c.String()]
	ibs.mu.RUnlock()
	if ok {
		return int(size), nil
	}
	return -1, fmt.Errorf("block not found in index: %s", c)
}

// DeleteBlock removes a block from the blockstore.
// It removes from both the cache and the index.
func (ibs *IndexedBlockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	ibs.mu.Lock()
	defer ibs.mu.Unlock()

	delete(ibs.offsetMap, c.String())
	delete(ibs.sizeMap, c.String())
	return ibs.lru.DeleteBlock(ctx, c)
}

// AllKeysChan returns a channel with all block CIDs in the blockstore.
func (ibs *IndexedBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	cidStrings := make(chan string, 100) // Buffer for performance

	go func() {
		ibs.mu.RLock()
		for cidStr := range ibs.offsetMap {
			select {
			case cidStrings <- cidStr:
			case <-ctx.Done():
				close(cidStrings)
				return
			}
		}
		ibs.mu.RUnlock()
		close(cidStrings)
	}()

	// Convert string CIDs to CID objects
	result := make(chan cid.Cid)
	go func() {
		defer close(result)
		for cidStr := range cidStrings {
			c, err := cid.Decode(cidStr)
			if err != nil {
				continue // Skip invalid CIDs
			}
			select {
			case result <- c:
			case <-ctx.Done():
				return
			}
		}
	}()

	return result, nil
}

// Close cleans up resources.
func (ibs *IndexedBlockstore) Close() error {
	ibs.mu.Lock()
	defer ibs.mu.Unlock()
	ibs.offsetMap = nil
	ibs.sizeMap = nil
	return nil
}

// compile-time check that IndexedBlockstore implements blockstore.Blockstore
var _ boxoblockstore.Blockstore = (*IndexedBlockstore)(nil)
