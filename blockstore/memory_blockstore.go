package blockstore

import (
	"context"

	boxoblockstore "github.com/ipfs/boxo/blockstore"
	blockformat "github.com/ipfs/go-block-format"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
)

// InMemoryBlockstore is a thread-safe blockstore that stores all blocks in memory.
// Unlike LRUBlockstore, it has no size limit and will continue adding blocks until
// out of memory. Use this only when you know the total size of data is limited.
type InMemoryBlockstore struct {
	boxoblockstore.Blockstore
}

// NewInMemoryBlockstore creates a new in-memory blockstore using boxo's map datastore.
func NewInMemoryBlockstore() *InMemoryBlockstore {
	dstore := dssync.MutexWrap(ds.NewMapDatastore())
	return &InMemoryBlockstore{
		Blockstore: boxoblockstore.NewBlockstore(dstore),
	}
}

// AddBlocksFromFile allows adding multiple blocks to the blockstore
func (b *InMemoryBlockstore) AddBlocksFromFile(ctx context.Context, blocks []blockformat.Block) error {
	for _, blk := range blocks {
		if err := b.Blockstore.Put(ctx, blk); err != nil {
			return err
		}
	}
	return nil
}
