package car

import (
	"bytes"
	"context"
	"testing"
	"testing/fstest"

	"github.com/ipfs/go-cid"

	"go.lumeweb.com/ipfs-content/blockstore"
)

// TestIndexedBlockstore_Reload tests block reloading on cache eviction.
func TestIndexedBlockstore_Reload(t *testing.T) {
	ctx := context.Background()

	// Create a test filesystem with enough content to cause eviction
	testData := make([]byte, 200*1024) // 200KB per file
	for i := range testData {
		testData[i] = byte(i)
	}

	testFS := fstest.MapFS{
		"file1.txt": {Data: testData},
		"file2.txt": {Data: testData},
		"file3.txt": {Data: testData},
		"file4.txt": {Data: testData},
		"file5.txt": {Data: testData},
	}

	// Create CAR file
	var carBuffer bytes.Buffer
	rootCID, err := StreamCAR(ctx, testFS, &carBuffer, true)
	if err != nil {
		t.Fatalf("Failed to create CAR: %v", err)
	}

	// Read CAR with small memory limit (10KB) to force eviction
	summary, err := ReadCAR(context.Background(), bytes.NewReader(carBuffer.Bytes()), 10*1024)
	if err != nil {
		t.Fatalf("Failed to read CAR with small cache: %v", err)
	}

	// Verify we still got all the data
	if len(summary.TreeEntries) == 0 {
		t.Fatal("No tree entries found")
	}

	// Verify root CID matches (even though cache evicted)
	if summary.RootCID != rootCID {
		t.Errorf("Root CID mismatch: got %s, want %s", summary.RootCID, rootCID)
	}
}

// TestIndexedBlockstore_GetFromCAR tests IndexedBlockstore with a real CAR file.
func TestIndexedBlockstore_GetFromCAR(t *testing.T) {
	ctx := context.Background()

	// Create test CAR file
	testFS := fstest.MapFS{
		"test.txt": {Data: []byte("Test content for indexed blockstore")},
	}

	var carBuffer bytes.Buffer
	rootCID, err := StreamCAR(ctx, testFS, &carBuffer, true)
	if err != nil {
		t.Fatalf("Failed to create CAR: %v", err)
	}

	// Create indexed blockstore
	carReader := bytes.NewReader(carBuffer.Bytes())
	ibs := blockstore.NewIndexedBlockstore(100*1024, carReader)

	// Index all blocks
	if err := ibs.IndexAll(ctx); err != nil {
		t.Fatalf("Failed to index blocks: %v", err)
	}

	// Test Get for root CID
	block, err := ibs.Get(ctx, rootCID)
	if err != nil {
		t.Fatalf("Failed to get root block: %v", err)
	}

	if block.Cid() != rootCID {
		t.Errorf("Block CID mismatch: got %s, want %s", block.Cid(), rootCID)
	}

	// Test Has
	has, err := ibs.Has(ctx, rootCID)
	if err != nil {
		t.Fatalf("Failed to check Has: %v", err)
	}

	if !has {
		t.Error("Has returned false for existing block")
	}

	// Test GetSize
	size, err := ibs.GetSize(ctx, rootCID)
	if err != nil {
		t.Fatalf("Failed to GetSize: %v", err)
	}

	if size <= 0 {
		t.Errorf("Invalid size: %d", size)
	}

	// Test AllKeysChan
	keysChan, err := ibs.AllKeysChan(ctx)
	if err != nil {
		t.Fatalf("Failed to get AllKeysChan: %v", err)
	}

	keyCount := 0
	for key := range keysChan {
		if key == cid.Undef {
			t.Error("Got undefined CID from AllKeysChan")
		}
		keyCount++
	}

	if keyCount == 0 {
		t.Error("No keys from AllKeysChan")
	}
}
