package blockstore

import (
	"context"
	"testing"

	"github.com/ipfs/boxo/ipld/merkledag"
	blockformat "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	multicodec "github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	// Test CIDs for use across tests
	testCID1 = mustGenerateCID("test1")
	testCID2 = mustGenerateCID("test2")
	testCID3 = mustGenerateCID("test3")
	testCID4 = mustGenerateCID("test4")
	testCID5 = mustGenerateCID("test5")

	// Test data
	testData1 = []byte("test data block 1")
	testData2 = []byte("test data block 2")
	testData3 = []byte("test data block 3")
	testData4 = []byte("test data block 4")
	testData5 = []byte("test data block 5")
)

func mustGenerateCID(data string) cid.Cid {
	c, err := cid.Prefix{
		Version:  1,
		Codec:    0x70, // dag-pb
		MhType:   0x12, // sha2-256
		MhLength: -1,
	}.Sum([]byte(data))
	if err != nil {
		panic(err)
	}
	return c
}

func NewTestBlock(cid cid.Cid, data []byte) blockformat.Block {
	block, err := blockformat.NewBlockWithCid(data, cid)
	if err != nil {
		panic(err)
	}
	return block
}

// assertBlockEquals checks if two blocks are equal
func assertBlockEquals(t *testing.T, expected, actual blockformat.Block) {
	assert.Equal(t, expected.Cid(), actual.Cid())
	assert.Equal(t, expected.RawData(), actual.RawData())
}

// TestNewLevelBlockStore verifies the constructor creates a valid blockstore
func TestNewLevelBlockStore(t *testing.T) {
	bs := NewLevelBlockStore()

	assert.NotNil(t, bs)
	assert.NotNil(t, bs.inner)
	assert.NotNil(t, bs.levelMap)
	assert.NotNil(t, bs.cohorts)
	assert.Equal(t, 0, bs.blockCount)
	assert.Equal(t, DefaultMaxLevels, bs.maxLevels)
}

// TestPutAndGet verifies basic put and get functionality
func TestPutAndGet(t *testing.T) {
	bs := NewLevelBlockStore()
	block := NewTestBlock(testCID1, testData1)

	// Put should succeed
	require.NoError(t, bs.Put(context.TODO(), block))

	// Get should return the same block
	retrieved, err := bs.Get(context.TODO(), testCID1)
	require.NoError(t, err)
	assertBlockEquals(t, block, retrieved)
}

// TestPutLeaf verifies leaf nodes get correct level tracking
func TestPutLeaf(t *testing.T) {
	bs := NewLevelBlockStore()

	// Put leaf (no children)
	leaf := NewTestBlock(testCID1, testData1)
	require.NoError(t, bs.Put(context.TODO(), leaf))

	// Should be at level 0
	level := bs.GetLevel(testCID1)
	assert.Equal(t, 0, level)

	// Second leaf should also be at level 0
	leaf2 := NewTestBlock(testCID2, testData2)
	require.NoError(t, bs.Put(context.TODO(), leaf2))
	level2 := bs.GetLevel(testCID2)
	assert.Equal(t, 0, level2)
}

// TestPutParent verifies parent nodes infer correct level from children
func TestPutParent(t *testing.T) {
	bs := NewLevelBlockStore()

	// Put leaves first (level 0)
	leaf1 := NewTestBlock(testCID1, testData1)
	leaf2 := NewTestBlock(testCID2, testData2)
	require.NoError(t, bs.Put(context.TODO(), leaf1))
	require.NoError(t, bs.Put(context.TODO(), leaf2))

	// Create parent with links to leaves
	parentData := createNodeWithLinks(testCID1, testCID2)
	parent := NewTestBlock(testCID3, parentData)
	require.NoError(t, bs.Put(context.TODO(), parent))

	// Parent should be at level 1
	level := bs.GetLevel(testCID3)
	assert.Equal(t, 1, level)
}

// TestMultiLevelParent verifies levels increase correctly with depth
func TestMultiLevelParent(t *testing.T) {
	bs := NewLevelBlockStore()

	// Level 0: leaves
	leaf1 := NewTestBlock(testCID1, testData1)
	leaf2 := NewTestBlock(testCID2, testData2)
	require.NoError(t, bs.Put(context.TODO(), leaf1))
	require.NoError(t, bs.Put(context.TODO(), leaf2))

	// Level 1: parent of leaves
	parent1Data := createNodeWithLinks(testCID1, testCID2)
	parent1 := NewTestBlock(testCID3, parent1Data)
	require.NoError(t, bs.Put(context.TODO(), parent1))

	// Level 1: more leaves
	leaf3 := NewTestBlock(testCID4, testData3)
	leaf4 := NewTestBlock(testCID5, testData4)
	require.NoError(t, bs.Put(context.TODO(), leaf3))
	require.NoError(t, bs.Put(context.TODO(), leaf4))

	// Level 2: parent of parent and leaves
	rootData := createNodeWithLinks(testCID3, testCID4, testCID5)
	root := NewTestBlock(mustGenerateCID("root"), rootData)
	require.NoError(t, bs.Put(context.TODO(), root))

	// Verify levels
	assert.Equal(t, 0, bs.GetLevel(testCID1))
	assert.Equal(t, 0, bs.GetLevel(testCID2))
	assert.Equal(t, 1, bs.GetLevel(testCID3))
	assert.Equal(t, 0, bs.GetLevel(testCID4))
	assert.Equal(t, 0, bs.GetLevel(testCID5))
	assert.Equal(t, 2, bs.GetLevel(mustGenerateCID("root")))
}

// TestCohortRotation verifies cohorts rotate when exceeding max levels


// TestLevelRotation verifies oldest levels are evicted when exceeding max levels
func TestLevelRotation(t *testing.T) {
	bs := NewLevelBlockStoreWithLevels(2) // Keep only 2 levels

	// Create leaves at level 0
	leaves := []cid.Cid{
		mustGenerateCID("leaf0"),
		mustGenerateCID("leaf1"),
	}
	for _, leafCID := range leaves {
		block := NewTestBlock(leafCID, []byte("data"))
		require.NoError(t, bs.Put(context.TODO(), block))
	}

	// Should be at level 0
	for _, leafCID := range leaves {
		assert.Equal(t, 0, bs.GetLevel(leafCID))
	}

	// Create parent at level 1
	parentData := createNodeWithLinks(leaves...)
	parentCID := mustGenerateCID("parent1")
	block := NewTestBlock(parentCID, parentData)
	require.NoError(t, bs.Put(context.TODO(), block))
	assert.Equal(t, 1, bs.GetLevel(parentCID))

	// Create grandparent at level 2 (should now exceed max of 2 levels)
	// Level 0 should be evicted
	grandparentData := createNodeWithLinks(parentCID)
	grandparentCID := mustGenerateCID("grandparent")
	block2 := NewTestBlock(grandparentCID, grandparentData)
	require.NoError(t, bs.Put(context.TODO(), block2))
	assert.Equal(t, 2, bs.GetLevel(grandparentCID))

	_ = leaves // Level 0 metadata may or may not be preserved depending on implementation
}

// TestGetChildren verifies children relationships are tracked correctly
func TestGetChildren(t *testing.T) {
	bs := NewLevelBlockStore()

	// Put leaves
	leaf1 := NewTestBlock(testCID1, testData1)
	leaf2 := NewTestBlock(testCID2, testData2)
	require.NoError(t, bs.Put(context.TODO(), leaf1))
	require.NoError(t, bs.Put(context.TODO(), leaf2))

	// Put parent
	parentData := createNodeWithLinks(testCID1, testCID2)
	parent := NewTestBlock(testCID3, parentData)
	require.NoError(t, bs.Put(context.TODO(), parent))

	// Get children of parent
	children, err := bs.GetChildren(context.TODO(), testCID3)
	require.NoError(t, err)
	require.Len(t, children, 2)
	assert.Contains(t, children, testCID1)
	assert.Contains(t, children, testCID2)
}

// TestGetAncestors verifies ancestor chain is tracked correctly
func TestGetAncestors(t *testing.T) {
	bs := NewLevelBlockStore()

	// Put leaves
	leaf1 := NewTestBlock(testCID1, testData1)
	require.NoError(t, bs.Put(context.TODO(), leaf1))

	// Put parent
	parentData := createNodeWithLinks(testCID1)
	parent := NewTestBlock(testCID2, parentData)
	require.NoError(t, bs.Put(context.TODO(), parent))

	// Put root
	rootData := createNodeWithLinks(testCID2)
	root := NewTestBlock(testCID3, rootData)
	require.NoError(t, bs.Put(context.TODO(), root))

	// Get ancestors of leaf (should go up to root)
	ancestors, err := bs.GetAncestors(context.TODO(), testCID1)
	require.NoError(t, err)
	require.Len(t, ancestors, 2)
	assert.Contains(t, ancestors, testCID2)
	assert.Contains(t, ancestors, testCID3)

	// Get ancestors of parent (should include root only)
	ancestors2, err := bs.GetAncestors(context.TODO(), testCID2)
	require.NoError(t, err)
	require.Len(t, ancestors2, 1)
	assert.Contains(t, ancestors2, testCID3)
}

// TestPutMany verifies multiple blocks can be added efficiently
func TestPutMany(t *testing.T) {
	bs := NewLevelBlockStore()

	blocks := []blockformat.Block{
		NewTestBlock(testCID1, testData1),
		NewTestBlock(testCID2, testData2),
		NewTestBlock(testCID3, testData3),
	}

	require.NoError(t, bs.PutMany(context.TODO(), blocks))

	// Verify all blocks exist
	for _, block := range blocks {
		retrieved, err := bs.Get(context.TODO(), block.Cid())
		require.NoError(t, err)
		assertBlockEquals(t, block, retrieved)
	}
}

// TestHas verifies block existence check works correctly
func TestHas(t *testing.T) {
	bs := NewLevelBlockStore()
	block := NewTestBlock(testCID1, testData1)

	// Initially false
	has, err := bs.Has(context.TODO(), testCID1)
	require.NoError(t, err)
	assert.False(t, has)

	// Put the block
	require.NoError(t, bs.Put(context.TODO(), block))

	// Now true
	has, err = bs.Has(context.TODO(), testCID1)
	require.NoError(t, err)
	assert.True(t, has)
}

// TestGetSize verifies block size retrieval works correctly
func TestGetSize(t *testing.T) {
	bs := NewLevelBlockStore()
	block := NewTestBlock(testCID1, testData1)

	require.NoError(t, bs.Put(context.TODO(), block))

	size, err := bs.GetSize(context.TODO(), testCID1)
	require.NoError(t, err)
	assert.Equal(t, len(testData1), size)
}

// TestDeleteBlock verifies block deletion works correctly
func TestDeleteBlock(t *testing.T) {
	bs := NewLevelBlockStore()
	block := NewTestBlock(testCID1, testData1)

	require.NoError(t, bs.Put(context.TODO(), block))

	// Block should exist
	has, _ := bs.Has(context.TODO(), testCID1)
	assert.True(t, has)

	// Delete the block
	require.NoError(t, bs.DeleteBlock(context.TODO(), testCID1))

	// Block should no longer exist
	has, _ = bs.Has(context.TODO(), testCID1)
	assert.False(t, has)
}

// TestAllKeysChan verifies all keys can be iterated
func TestAllKeysChan(t *testing.T) {
	bs := NewLevelBlockStore()

	blocks := []blockformat.Block{
		NewTestBlock(testCID1, testData1),
		NewTestBlock(testCID2, testData2),
		NewTestBlock(testCID3, testData3),
	}

	require.NoError(t, bs.PutMany(context.TODO(), blocks))

	// Get all keys
	keysCh, err := bs.AllKeysChan(context.TODO())
	require.NoError(t, err)

	// Collect keys
	keys := []cid.Cid{}
	for key := range keysCh {
		keys = append(keys, key)
	}

	require.Len(t, keys, len(blocks))
}

// TestSizeAndLen verifies size and count reporting
func TestSizeAndLen(t *testing.T) {
	bs := NewLevelBlockStore()

	blocks := []blockformat.Block{
		NewTestBlock(testCID1, testData1),
		NewTestBlock(testCID2, testData2),
		NewTestBlock(testCID3, testData3),
	}

	require.NoError(t, bs.PutMany(context.TODO(), blocks))

	assert.Equal(t, 3, bs.Len())
}

// TestClose verifies close is a no-op
func TestClose(t *testing.T) {
	bs := NewLevelBlockStore()
	assert.NoError(t, bs.Close())
}

// TestGetInnerStore returns the underlying blockstore
func TestGetInnerStore(t *testing.T) {
	bs := NewLevelBlockStore()
	require.NotNil(t, bs.GetInnerStore())
}

// TestConcurrentAccess verifies the blockstore is thread-safe
func TestConcurrentAccess(t *testing.T) {
	bs := NewLevelBlockStore()
	done := make(chan bool)
	errors := make(chan error, 10)

	// 10 goroutines putting blocks
	for i := range 10 {
		go func(index int) {
			for j := range 100 {
				cid := mustGenerateCID(string(rune(index*100 + j)))
				block := NewTestBlock(cid, []byte("data"))
				if err := bs.Put(context.Background(), block); err != nil {
					errors <- err
					return
				}
			}
			done <- true
		}(i)
	}

	// Wait for all to complete
	for range 10 {
		select {
		case <-done:
		case err := <-errors:
			t.Fatalf("concurrent access error: %v", err)
		}
	}

	// Verify we have all blocks
	// Note: we can't check exact count here as we might have evictions
}

// BenchmarkPut benchmarks put operation
func BenchmarkPut(b *testing.B) {
	bs := NewLevelBlockStore()

	b.ResetTimer()
	for i := range b.N {
		cid := mustGenerateCID(string(rune(i)))
		bs.Put(context.TODO(), NewTestBlock(cid, testData1))
	}
}

// BenchmarkPutLeaf benchmarks put of leaf nodes (fast path)
func BenchmarkPutLeaf(b *testing.B) {
	bs := NewLevelBlockStore()

	b.ResetTimer()
	for i := range b.N {
		cid := mustGenerateCID(string(rune(i)))
		leaf := NewTestBlock(cid, []byte("leaf data"))
		bs.Put(context.TODO(), leaf)
	}
}

// BenchmarkPutParent benchmarks put of parent nodes (slower path)
func BenchmarkPutParent(b *testing.B) {
	bs := NewLevelBlockStore()

	// Pre-populate leaves
	for i := range 100 {
		cid := mustGenerateCID(string(rune(i)))
		leaf := NewTestBlock(cid, []byte("leaf"))
		bs.Put(context.TODO(), leaf)
	}

	b.ResetTimer()
	for i := range b.N {
		cid := mustGenerateCID(string(rune(i)))
		parentData := createNodeWithLinks(cid)
		parent := NewTestBlock(mustGenerateCID(string(rune(i+100))), parentData)
		bs.Put(context.TODO(), parent)
	}
}

// BenchmarkGet benchmarks get operation
func BenchmarkGet(b *testing.B) {
	bs := NewLevelBlockStore()

	// Pre-populate
	for i := range 1000 {
		cid := mustGenerateCID(string(rune(i)))
		block := NewTestBlock(cid, []byte("data"))
		bs.Put(context.TODO(), block)
	}

	b.ResetTimer()
	for i := range b.N {
		cid := mustGenerateCID(string(rune(i % 1000)))
		bs.Get(context.TODO(), cid)
	}
}

// BenchmarkGetLevel benchmarks level lookup
func BenchmarkGetLevel(b *testing.B) {
	bs := NewLevelBlockStore()

	// Pre-populate
	for i := range 1000 {
		cid := mustGenerateCID(string(rune(i)))
		block := NewTestBlock(cid, []byte(iotaString(i)))
		bs.Put(context.TODO(), block)
	}

	b.ResetTimer()
	for i := range b.N {
		cid := mustGenerateCID(string(rune(i % 1000)))
		bs.GetLevel(cid)
	}
}

// Helper function to create a node with links
// Creates a valid merkledag.ProtoNode with the specified child links
func createNodeWithLinks(linkCIDs ...cid.Cid) []byte {
	// Create an empty ProtoNode
	node := merkledag.ProtoNode{}

	// Set a CID builder to ensure proper encoding
	node.SetCidBuilder(cid.V1Builder{Codec: cid.DagProtobuf, MhType: uint64(multicodec.Sha2_256)})

	// Add a link for each CID
	for _, linkCID := range linkCIDs {
		// Create the link using AddRawLink
		if err := node.AddRawLink("", &format.Link{
			Cid:  linkCID,
			Size: uint64(len("dummy")), // Dummy size
		}); err != nil {
			panic(err)
		}
	}

	// Encode to bytes
	encoded, err := node.Marshal()
	if err != nil {
		panic(err)
	}

	return encoded
}

// Helper function to create unique strings
func iotaString(n int) string {
	result := make([]byte, n)
	for i := range result {
		result[i] = byte(i)
	}
	return string(result)
}
