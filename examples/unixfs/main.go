// Package main demonstrates UnixFS node generation.
//
// This example shows how to create UnixFS nodes from readers and directories
// using the UnixFSNodeGenerator interface.
package main

import (
	"bytes"
	"context"
	"fmt"

	"go.lumeweb.com/ipfs-content/unixfs"
)

// readSeekCloser wraps bytes.Reader to implement io.ReadSeekCloser
type readSeekCloser struct {
	*bytes.Reader
}

func (r *readSeekCloser) Close() error {
	return nil
}

func main() {
	ctx := context.Background()

	// Example 1: Create node generator with default options
	fmt.Println("Example 1: Default Node Generator")
	generator := unixfs.NewUnixFSNodeGenerator()
	fmt.Printf("Created generator with blockstore: %v\n\n", generator.GetBlockstore() != nil)

	// Example 2: Create a UnixFS node from a reader
	fmt.Println("Example 2: Create UnixFS Node")

	testData := []byte("Hello, IPFS!")
	reader := &readSeekCloser{bytes.NewReader(testData)}

	// Create a UnixFS node from the reader
	node, err := generator.CreateNode(ctx, reader)
	if err != nil {
		fmt.Printf("Failed to create UnixFS node: %v\n", err)
	} else {
		fmt.Printf("Created UnixFS node with CID: %s\n", node.Cid())
		fmt.Printf("Block data length: %d bytes\n\n", len(node.RawData()))
	}

	// Example 3: Create a directory node
	fmt.Println("Example 3: Create Directory Node")

	// Create an empty directory
	_, err = generator.CreateDirectory()
	if err != nil {
		fmt.Printf("Failed to create directory: %v\n", err)
	} else {
		fmt.Printf("Created directory node\n")
	}

	fmt.Println("Done!")
}
