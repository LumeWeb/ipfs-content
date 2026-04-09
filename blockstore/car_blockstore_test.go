package blockstore

import (
	"context"
	"testing"

	"github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testingutil "go.lumeweb.com/ipfs-content/internal/testing"
)

// newTestBlock creates a test block with a raw CID from string data.
func newTestBlock(t *testing.T, data string) blocks.Block {
	t.Helper()
	c := testingutil.GenerateRawBlockCIDFromString(t, data)
	block, err := blocks.NewBlockWithCid([]byte(data), c)
	require.NoError(t, err)
	return block
}

// TestNewLRUBlockstore tests constructor
func TestNewLRUBlockstore(t *testing.T) {
	store := NewLRUBlockstore(1000)

	assert.NotNil(t, store)
	assert.Equal(t, uint64(0), store.Size())
	assert.Equal(t, 0, store.Len())
}

// TestLRUBlockstore_Put tests basic put operation
func TestLRUBlockstore_Put(t *testing.T) {
	store := NewLRUBlockstore(1000)
	ctx := context.Background()

	data := "test data"
	block := newTestBlock(t, data)

	err := store.Put(ctx, block)
	assert.NoError(t, err)
	assert.Equal(t, uint64(len(data)), store.Size())
	assert.Equal(t, 1, store.Len())
}

// TestLRUBlockstore_Put_Eviction tests eviction when size limit exceeded
func TestLRUBlockstore_Put_Eviction(t *testing.T) {
	const limit uint64 = 100
	store := NewLRUBlockstore(limit)
	ctx := context.Background()

	// Add blocks with actual data (CID overhead adds ~3 bytes per block)
	// Each block is ~35 bytes total (32 data + ~3 CID overhead), so 3 blocks = 105 bytes > 100 limit
	data1 := "block number 1 with 30 bytes len!!1"
	data2 := "block number 2 with 30 bytes len!!2"
	data3 := "block number 3 with 30 bytes len!!3"

	block1 := newTestBlock(t, data1)
	err := store.Put(ctx, block1)
	require.NoError(t, err)

	block2 := newTestBlock(t, data2)
	err = store.Put(ctx, block2)
	require.NoError(t, err)

	// This should trigger eviction as we exceed limit with 3 blocks
	block3 := newTestBlock(t, data3)
	err = store.Put(ctx, block3)
	require.NoError(t, err)

	// Verify we only have 2 blocks after adding the 3rd (one was evicted)
	require.LessOrEqual(t, store.Size(), limit)

	// For blocks of ~35 bytes each, we can only fit 2 blocks under 100-byte limit
	require.Equal(t, 2, store.Len())

	// Verify block1 (first added) was evicted
	_, err = store.Get(ctx, block1.Cid())
	assert.Error(t, err)
	assert.Equal(t, ErrNotFound, err)

	// Verify block2 and block3 are still present
	_, err = store.Get(ctx, block2.Cid())
	assert.NoError(t, err)
	_, err = store.Get(ctx, block3.Cid())
	assert.NoError(t, err)
}

// TestLRUBlockstore_Put_Existing tests that existing blocks are moved to front
func TestLRUBlockstore_Put_Existing(t *testing.T) {
	store := NewLRUBlockstore(100)
	ctx := context.Background()

	block1 := newTestBlock(t, "first block")
	err := store.Put(ctx, block1)
	require.NoError(t, err)

	block2 := newTestBlock(t, "second block")
	err = store.Put(ctx, block2)
	require.NoError(t, err)
	expectedSize := store.Size()
	expectedLen := store.Len()

	// Re-add first block - should move it to front, not add duplicate
	err = store.Put(ctx, block1)
	require.NoError(t, err)

	require.Equal(t, expectedSize, store.Size())
	require.Equal(t, expectedLen, store.Len())
}

// TestLRUBlockstore_Get tests get operation
func TestLRUBlockstore_Get(t *testing.T) {
	store := NewLRUBlockstore(1000)
	ctx := context.Background()

	data := "test data"
	block := newTestBlock(t, data)
	err := store.Put(ctx, block)
	require.NoError(t, err)

	retrieved, err := store.Get(ctx, block.Cid())
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, []byte(data), retrieved.RawData())
}

// TestLRUBlockstore_Get_NotFound tests get on non-existent block
func TestLRUBlockstore_Get_NotFound(t *testing.T) {
	store := NewLRUBlockstore(1000)
	ctx := context.Background()

	c := testingutil.GenerateRawBlockCIDFromString(t, "nonexistent")
	_, err := store.Get(ctx, c)

	assert.Error(t, err)
	assert.Equal(t, ErrNotFound, err)
}

// TestLRUBlockstore_GetSize tests getting block size
func TestLRUBlockstore_GetSize(t *testing.T) {
	store := NewLRUBlockstore(1000)
	ctx := context.Background()

	data := "test data"
	block := newTestBlock(t, data)
	err := store.Put(ctx, block)
	require.NoError(t, err)

	size, err := store.GetSize(ctx, block.Cid())
	require.NoError(t, err)
	assert.Equal(t, len(data), size)
}

// TestLRUBlockstore_GetSize_NotFound tests size on non-existent block
func TestLRUBlockstore_GetSize_NotFound(t *testing.T) {
	store := NewLRUBlockstore(1000)
	ctx := context.Background()

	c := testingutil.GenerateRawBlockCIDFromString(t, "nonexistent")
	size, err := store.GetSize(ctx, c)

	assert.Error(t, err)
	assert.Equal(t, ErrNotFound, err)
	assert.Equal(t, -1, size)
}

// TestLRUBlockstore_Has tests has operation
func TestLRUBlockstore_Has(t *testing.T) {
	store := NewLRUBlockstore(1000)
	ctx := context.Background()

	data := "test data"
	block := newTestBlock(t, data)
	err := store.Put(ctx, block)
	require.NoError(t, err)

	exists, err := store.Has(ctx, block.Cid())
	require.NoError(t, err)
	assert.True(t, exists)

	notExists, err := store.Has(ctx, testingutil.GenerateRawBlockCIDFromString(t, "nonexistent"))
	require.NoError(t, err)
	assert.False(t, notExists)
}

// TestLRUBlockstore_PutMany tests put many operation
func TestLRUBlockstore_PutMany(t *testing.T) {
	store := NewLRUBlockstore(1000)
	ctx := context.Background()

	blocks := []blocks.Block{
		newTestBlock(t, "block1"),
		newTestBlock(t, "block2"),
		newTestBlock(t, "block3"),
	}

	err := store.PutMany(ctx, blocks)
	require.NoError(t, err)
	assert.Equal(t, len(blocks), store.Len())
}

// TestLRUBlockstore_AllKeysChan tests getting all keys
func TestLRUBlockstore_AllKeysChan(t *testing.T) {
	store := NewLRUBlockstore(1000)
	ctx := context.Background()

	blocks := []blocks.Block{
		newTestBlock(t, "block1"),
		newTestBlock(t, "block2"),
		newTestBlock(t, "block3"),
	}

	err := store.PutMany(ctx, blocks)
	require.NoError(t, err)

	ch, err := store.AllKeysChan(ctx)
	require.NoError(t, err)

	keys := make([]cid.Cid, 0)
	for c := range ch {
		keys = append(keys, c)
	}

	assert.Equal(t, 3, len(keys))
}

// TestLRUBlockstore_AllKeysChan_Empty tests keys on empty store
func TestLRUBlockstore_AllKeysChan_Empty(t *testing.T) {
	store := NewLRUBlockstore(1000)
	ctx := context.Background()

	ch, err := store.AllKeysChan(ctx)
	require.NoError(t, err)

	keys := make([]cid.Cid, 0)
	for c := range ch {
		keys = append(keys, c)
	}

	assert.Equal(t, 0, len(keys))
}

// TestLRUBlockstore_DeleteBlock tests delete operation
func TestLRUBlockstore_DeleteBlock(t *testing.T) {
	store := NewLRUBlockstore(1000)
	ctx := context.Background()

	data := "test data"
	block := newTestBlock(t, data)
	err := store.Put(ctx, block)
	require.NoError(t, err)

	err = store.DeleteBlock(ctx, block.Cid())
	require.NoError(t, err)

	_, err = store.Get(ctx, block.Cid())
	assert.Error(t, err)
	assert.Equal(t, ErrNotFound, err)
}

// TestLRUBlockstore_DeleteBlock_NotExisting tests delete on non-existent block
func TestLRUBlockstore_DeleteBlock_NotExisting(t *testing.T) {
	store := NewLRUBlockstore(1000)
	ctx := context.Background()

	c := testingutil.GenerateRawBlockCIDFromString(t, "nonexistent")
	err := store.DeleteBlock(ctx, c)

	// Should not error - it's a no-op
	assert.NoError(t, err)
}

// TestLRUBlockstore_Close tests close operation (should be no-op)
func TestLRUBlockstore_Close(t *testing.T) {
	store := NewLRUBlockstore(1000)

	err := store.Close()
	assert.NoError(t, err)
}

// TestLRUBlockstore_Size tests size tracking
func TestLRUBlockstore_Size(t *testing.T) {
	store := NewLRUBlockstore(1000)
	ctx := context.Background()

	assert.Equal(t, uint64(0), store.Size())

	block := newTestBlock(t, "test data")
	err := store.Put(ctx, block)
	require.NoError(t, err)

	assert.Equal(t, uint64(len("test data")), store.Size())
}

// TestLRUBlockstore_Len tests length tracking
func TestLRUBlockstore_Len(t *testing.T) {
	store := NewLRUBlockstore(1000)
	ctx := context.Background()

	assert.Equal(t, 0, store.Len())

	blocks := []blocks.Block{
		newTestBlock(t, "block1"),
		newTestBlock(t, "block2"),
		newTestBlock(t, "block3"),
	}

	err := store.PutMany(ctx, blocks)
	require.NoError(t, err)

	assert.Equal(t, 3, store.Len())
}

// TestLRUBlockstore_LRUEvictionOrder tests LRU eviction order
func TestLRUBlockstore_LRUEvictionOrder(t *testing.T) {
	// Use smaller limit to force eviction with 4 blocks of 10 bytes each
	// With 3 blocks = 30 bytes, limit 25 means we must evict when adding 4th
	store := NewLRUBlockstore(25)
	ctx := context.Background()

	block1 := newTestBlock(t, "1111111111")
	block2 := newTestBlock(t, "2222222222")
	block3 := newTestBlock(t, "3333333333")

	err := store.Put(ctx, block1)
	require.NoError(t, err)
	err = store.Put(ctx, block2)
	require.NoError(t, err)
	// This should evict block1 since we've reached limit
	err = store.Put(ctx, block3)
	require.NoError(t, err)

	// Only 2 blocks should remain
	require.Equal(t, 2, store.Len())
	require.LessOrEqual(t, store.Size(), uint64(25))

	// Verify block1 was evicted (least recently used)
	_, err = store.Get(ctx, block1.Cid())
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)

	// block3 and block2 should still be present (in that order from most to least recent)
	_, err = store.Get(ctx, block2.Cid())
	assert.NoError(t, err)
	_, err = store.Get(ctx, block3.Cid())
	assert.NoError(t, err)

	// Add block4 - should evict least recently used to make room
	block4 := newTestBlock(t, "4444444444")
	err = store.Put(ctx, block4)
	require.NoError(t, err)

	// Store size should stay within limit
	require.LessOrEqual(t, store.Size(), uint64(25))
	// Should still have only 2 blocks
	require.Equal(t, 2, store.Len())

	// Verify the oldest (block2) was evicted, block3 and block4 remain
	_, err = store.Get(ctx, block2.Cid())
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = store.Get(ctx, block3.Cid())
	assert.NoError(t, err)
	_, err = store.Get(ctx, block4.Cid())
	assert.NoError(t, err)
}

// TestLRUBlockstore_EdgeCases tests edge cases
func TestLRUBlockstore_EdgeCases(t *testing.T) {
	t.Run("zero limit", func(t *testing.T) {
		store := NewLRUBlockstore(0)
		ctx := context.Background()

		block := newTestBlock(t, "test")
		err := store.Put(ctx, block)

		// With zero limit, blocks might be added immediately
		// The behavior depends on the eviction strategy
		assert.NoError(t, err)
	})

	t.Run("single block eviction", func(t *testing.T) {
		store := NewLRUBlockstore(15)
		ctx := context.Background()

		// Add small block
		block1 := newTestBlock(t, "small")
		err := store.Put(ctx, block1)
		require.NoError(t, err)

		// Add block that exceeds limit
		block2 := newTestBlock(t, "this is a very long block that exceeds")

		// block2 should succeed (evicts block1 if needed)
		err = store.Put(ctx, block2)
		assert.NoError(t, err)

		// block1 should be evicted since block2 takes more space
		_, err = store.Get(ctx, block1.Cid())
		assert.Error(t, err)
		assert.Equal(t, ErrNotFound, err)
	})
}
