// Package blockstore provides memory-based IPFS block storage implementations.
//
// This package provides two blockstore implementations:
//   - LRUBlockstore: Thread-safe LRU cache with size-based eviction using a doubly-linked list
//   - InMemoryBlockstore: Unbounded in-memory blockstore using boxo's map datastore
//
// LRUBlockstore is ideal for memory-constrained operations where you need to limit
// memory usage while processing IPFS blocks, such as CAR file generation. It evicts
// the least-recently-used blocks when the size limit is exceeded.
//
// InMemoryBlockstore provides unbounded storage for use cases where memory usage
// is known to be limited through other means.
//
// Basic Usage:
//
//	import "go.lumeweb.com/ipfs-content/blockstore"
//
//	// LRU blockstore with 100MB limit
//	store := blockstore.NewLRUBlockstore(100 * 1024 * 1024)
//
//	// In-memory blockstore (no size limit)
//	memStore := blockstore.NewInMemoryBlockstore()
//
// All operations use proper locking (RWMutex) for concurrent access.
package blockstore
