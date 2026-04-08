// Package blockstore provides memory-based IPFS block storage implementations.
//
// This package provides the following blockstore implementations:
//   - LRUBlockstore: Thread-safe LRU cache with size-based eviction using a doubly-linked list
//   - InMemoryBlockstore: Unbounded in-memory blockstore using boxo's map datastore
//   - IndexedBlockstore: LRU blockstore with offset tracking for reloading evicted blocks from CAR files
//
// LRUBlockstore is ideal for memory-constrained operations where you need to limit
// memory usage while processing IPFS blocks, such as CAR file generation. It evicts
// the least-recently-used blocks when the size limit is exceeded.
//
// InMemoryBlockstore provides unbounded storage for use cases where memory usage
// is known to be limited through other means.
//
// IndexedBlockstore extends LRUBlockstore with the ability to reload evicted blocks
// from a seekable source (like a CAR file) using pre-built offset indices. This enables
// memory-bounded CAR reading where blocks can be reloaded on-demand when evicted from cache.
// It tracks multiple offsets per CID to handle duplicate blocks from identical content.
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
//	// Indexed blockstore for CAR reading with memory limits
//	carReader := bytes.NewReader(carBytes)
//	ibs := blockstore.NewIndexedBlockstore(10 * 1024 * 1024, carReader)
//	err := ibs.IndexAll(ctx)
//
// All operations use proper locking (RWMutex) for concurrent access.
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
