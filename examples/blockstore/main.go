// Package main demonstrates memory-based block storage.
//
// This example shows how to use LRUBlockstore and InMemoryBlockstore
// for storing IPFS blocks with configurable memory limits.
package main

import (
	"context"
	"fmt"
	"log"

	"go.lumeweb.com/ipfs-content/blockstore"
	blockformat "github.com/ipfs/go-block-format"
)

func main() {
	ctx := context.Background()

	// LRU blockstore with 100MB limit
	fmt.Println("LRUBlockstore Example:")
	lruStore := blockstore.NewLRUBlockstore(100 * 1024 * 1024)

	// Put blocks
	blockData := []byte("test block data1")
	block := blockformat.NewBlock(blockData)
	err := lruStore.Put(ctx, block)
	if err != nil {
		log.Fatalf("Failed to put block: %v", err)
	}
	fmt.Printf("Stored block\n")

	// Get block
	retBlock, err := lruStore.Get(ctx, block.Cid())
	if err != nil {
		log.Fatalf("Failed to get block: %v", err)
	}
	fmt.Printf("Retrieved block data: %s\n", string(retBlock.RawData()))

	// In-memory blockstore (no size limit)
	fmt.Println("\nInMemoryBlockstore Example:")
	memStore := blockstore.NewInMemoryBlockstore()

	// Put blocks
	blockData2 := []byte("test block data2")
	block2 := blockformat.NewBlock(blockData2)
	err = memStore.Put(ctx, block2)
	if err != nil {
		log.Fatalf("Failed to put block: %v", err)
	}
	fmt.Printf("Stored another block\n")

	// Get block
	retBlock2, err := memStore.Get(ctx, block2.Cid())
	if err != nil {
		log.Fatalf("Failed to get block: %v", err)
	}
	fmt.Printf("Retrieved block data: %s\n", string(retBlock2.RawData()))
}
