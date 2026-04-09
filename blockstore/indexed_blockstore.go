package blockstore

import (
	"context"
	"fmt"
	"io"
	"sync"

	boxoblockstore "github.com/ipfs/boxo/blockstore"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	carv2 "github.com/ipld/go-car/v2"
	carv2index "github.com/ipld/go-car/v2/index"

	carv2util "go.lumeweb.com/ipfs-content/internal/carv2/util"
	internalio "go.lumeweb.com/ipfs-content/internal/io"
)

// IndexedBlockstore wraps an LRU blockstore and provides reloading from
// a seekable source (like a CAR file) using an index.
// It enables memory-bounded CAR reading with the ability to reload evicted blocks
// by seeking to their recorded positions in the original source.
// Supports both CARv1 and CARv2 formats.
type IndexedBlockstore struct {
	lru    *LRUBlockstore
	idx    *carv2index.InsertionIndex
	reader io.ReadSeeker
	mu     sync.RWMutex
}

// NewIndexedBlockstore creates a new IndexedBlockstore with the specified memory limit.
// The reader must support seeking to enable block reloading on cache eviction.
// Supports both CARv1 and CARv2 formats.
func NewIndexedBlockstore(memoryLimit uint64, reader io.ReadSeeker) *IndexedBlockstore {
	return &IndexedBlockstore{
		lru:    NewLRUBlockstore(memoryLimit),
		reader: reader,
	}
}

// IndexAll reads through the entire CAR stream and builds the offset index
// while loading all blocks into the LRU cache. This is a one-time operation
// that must be called before using the blockstore.
// Supports both CARv1 and CARv2 formats.
// For CARv2 files with a pre-built index, it will use that instead of regenerating.
func (ibs *IndexedBlockstore) IndexAll(ctx context.Context) error {
	ibs.mu.Lock()
	defer ibs.mu.Unlock()

	// Use InsertionIndex for efficient in-memory lookups
	ibs.idx = carv2index.NewInsertionIndex()

	// Generate index using LoadIndex with our InsertionIndex
	err := carv2.LoadIndex(ibs.idx, ibs.reader)
	if err != nil {
		return fmt.Errorf("generate index: %w", err)
	}

	// Create CAR reader for data reading (requires ReaderAt)
	// Note: after ReadOrGenerateIndex, the reader position is at the beginning
	readerAt := internalio.ToReaderAt(ibs.reader)
	cr, err := carv2.NewReader(readerAt)
	if err != nil {
		return fmt.Errorf("create CAR reader: %w", err)
	}

	// Get data reader (works for both CARv1 and CARv2)
	dr, err := cr.DataReader()
	if err != nil {
		return fmt.Errorf("get data reader: %w", err)
	}

	// Create block reader from data reader
	br, err := carv2.NewBlockReader(dr)
	if err != nil {
		return fmt.Errorf("create block reader: %w", err)
	}

	// Load all blocks into LRU cache
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled: %w", err)
		}

		blk, err := br.Next()
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
	}

	return nil
}

// Get retrieves a block from the blockstore.
// If the block is not in the LRU cache, it is reloaded from the CAR file
// by seeking to the recorded offset found in the index.
func (ibs *IndexedBlockstore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	ibs.mu.RLock()
	if ibs.idx == nil {
		ibs.mu.RUnlock()
		return nil, fmt.Errorf("index not initialized; call IndexAll first")
	}
	ibs.mu.RUnlock()

	// Try cache first
	blk, err := ibs.lru.Get(ctx, c)
	if err == nil {
		return blk, nil
	}

	// Cache miss - look up offsets in index
	var lastError error
	found := false
	ibs.idx.GetAll(c, func(offset uint64) bool {
		found = true
		if lastError != nil {
			return true // Continue to next offset if previous failed
		}

		// Seek to block position
		_, err := ibs.reader.Seek(int64(offset), io.SeekStart)
		if err != nil {
			lastError = fmt.Errorf("seek to offset %d: %w", offset, err)
			return true // Try next offset
		}

		// Read block at this position
		// Use a large maxReadBytes to allow reading large blocks
		cidRead, data, err := carv2util.ReadNode(ibs.reader, false, 10*1024*1024)
		if err != nil {
			lastError = fmt.Errorf("read block at offset %d: %w", offset, err)
			return true // Try next offset
		}

		// Verify CID matches what we expected
		if !cidRead.Equals(c) {
			lastError = fmt.Errorf("CID mismatch at offset %d: expected %s, got %s", offset, c, cidRead)
			return true // Try next offset
		}

		// Successfully loaded block - create and cache it
		blk, err = blocks.NewBlockWithCid(data, cidRead)
		if err != nil {
			lastError = fmt.Errorf("create block: %w", err)
			return true // Try next offset
		}

		ibs.mu.RLock()
		err = ibs.lru.Put(ctx, blk)
		ibs.mu.RUnlock()
		if err != nil {
			lastError = fmt.Errorf("add to cache: %w", err)
			return true // Try next offset
		}

		lastError = nil // Success
		return false    // Stop trying offsets
	})

	if !found {
		return nil, fmt.Errorf("block not found in index: %s", c)
	}
	if lastError != nil {
		return nil, fmt.Errorf("failed to reload block %s from any offset: %w", c, lastError)
	}

	return blk, nil
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
	defer ibs.mu.RUnlock()

	if ibs.idx == nil {
		return false, nil
	}

	return ibs.idx.HasExactCID(c)
}

// GetSize returns the size of a block in bytes.
// It loads the block to get its size, then caches it.
// This is less efficient than v1 which stored sizes, but maintains simplicity.
func (ibs *IndexedBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	if err := ctx.Err(); err != nil {
		return -1, fmt.Errorf("context cancelled: %w", err)
	}

	ibs.mu.RLock()
	if ibs.idx == nil {
		ibs.mu.RUnlock()
		return -1, fmt.Errorf("index not initialized; call IndexAll first")
	}
	ibs.mu.RUnlock()

	// Check if block is in cache
	if size, err := ibs.lru.GetSize(ctx, c); err == nil {
		return size, nil
	}

	// Block not in cache - need to load it
	blk, err := ibs.Get(ctx, c)
	if err != nil {
		return -1, err
	}

	return len(blk.RawData()), nil
}

// DeleteBlock removes a block from the blockstore.
// It removes from the cache. Note that it does NOT remove from the index
// since the index is derived from the CAR file and should remain consistent with it.
func (ibs *IndexedBlockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	ibs.mu.Lock()
	defer ibs.mu.Unlock()
	return ibs.lru.DeleteBlock(ctx, c)
}

// AllKeysChan returns a channel with all block CIDs in the blockstore.
func (ibs *IndexedBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	result := make(chan cid.Cid, 100)

	ibs.mu.RLock()
	if ibs.idx == nil {
		ibs.mu.RUnlock()
		close(result)
		return result, nil
	}
	ibs.mu.RUnlock()

	go func() {
		defer close(result)
		
		// Use InsertionIndex's ForEachCid for efficient iteration
		ibs.idx.ForEachCid(func(c cid.Cid, offset uint64) error {
			select {
			case result <- c:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()

	return result, nil
}

// Close cleans up resources.
func (ibs *IndexedBlockstore) Close() error {
	ibs.mu.Lock()
	defer ibs.mu.Unlock()
	ibs.idx = nil
	return nil
}

// GetIndex returns the underlying insertion index for external inspection.
func (ibs *IndexedBlockstore) GetIndex() *carv2index.InsertionIndex {
	ibs.mu.RLock()
	defer ibs.mu.RUnlock()
	return ibs.idx
}

// compile-time check that IndexedBlockstore implements blockstore.Blockstore
var _ boxoblockstore.Blockstore = (*IndexedBlockstore)(nil)
