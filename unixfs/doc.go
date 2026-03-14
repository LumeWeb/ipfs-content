// Package unixfs provides UnixFS node generation for IPFS.
//
// This package provides UnixFSNodeGenerator interface for creating IPFS-compatible
// directory structures from readers and directories. The default implementation uses
// boxo/merkledag libraries.
//
// The interface supports:
//   - Creating UnixFS nodes from readers
//   - Creating directory nodes with optional children
//   - Creating DAGs from readers with configurable chunking
//   - Accessing underlying DAG service and blockstore
//
// The package uses the options pattern for flexible dependency injection,
// allowing custom DAG services and blockstores to be configured.
//
// Basic Usage:
//
//	import "go.lumeweb.com/ipfs-content/unixfs"
//	import "go.lumeweb.com/ipfs-content/car"
//
//	// Create with custom components
//	generator := unixfs.NewUnixFSNodeGenerator(
//	    unixfs.WithUnixFSNodeDAGService(dagService),
//	    unixfs.WithUnixFSNodeBlockstore(blockstore),
//	)
//
//	// Create node from file
//	node, err := generator.CreateNode(ctx, file)
package unixfs
