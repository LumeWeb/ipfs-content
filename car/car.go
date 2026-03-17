package car

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/ipfs/boxo/blockservice"
	boxoblockstore "github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/exchange/offline"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/multiformats/go-varint"

	"go.lumeweb.com/ipfs-content/blockstore"
	"go.lumeweb.com/ipfs-content/internal/carv1"
	"go.lumeweb.com/ipfs-content/encoding"
	"go.lumeweb.com/ipfs-content/unixfs"
)

// DefaultMemoryLimit is the default memory limit for LRU blockstore operations (100MB).
// NewDAGServiceWithMemoryLimit creates a new LRU blockstore, blockservice, and DAG service trio
// with the specified memory limit. This is a convenience function for setting up the IPFS
// DAG infrastructure with memory-constrained block storage.
//
// The returned blockstore implements LRU eviction when the memory limit is exceeded,
// making it suitable for CAR file generation where you want to limit memory usage.
func NewDAGServiceWithMemoryLimit(memoryLimit uint64) (boxoblockstore.Blockstore, format.DAGService) {
	bs := blockstore.NewLRUBlockstore(memoryLimit)
	bsvc := blockservice.New(bs, offline.Exchange(bs))
	dagService := merkledag.NewDAGService(bsvc)
	return bs, dagService
}

// NewDAGServiceWithLevelAware creates a new LevelBlockStore, blockservice, and DAG service
// trio with the specified memory limit. This provides level-aware storage with cohort-based
// eviction that tracks parent-child relationships for efficient regeneration.
//
// The returned blockstore uses LevelBlockStore which preserves metadata (level relationships)
// across cohort rotations and a Reset() method to clear data while retaining metadata.
// This is suitable for two-phase operations where phase 1 builds a summary and phase 2
// regenerates blocks with level awareness.
func NewDAGServiceWithLevelAware() (boxoblockstore.Blockstore, format.DAGService) {
	bs := blockstore.NewLevelBlockStore()
	bsvc := blockservice.New(bs, offline.Exchange(bs))
	dagService := merkledag.NewDAGService(bsvc)
	return bs, dagService
}

// newCARBuilder creates a CARBuilder with the configured DAG service and node generator.
// This is an internal helper that encapsulates the common initialization pattern
// used across StreamCAR, StreamCARWithSize, and PrepareCAR.
func newCARBuilder() *CARBuilder {
	bs, dagService := NewDAGServiceWithLevelAware()
	generator := unixfs.NewUnixFSNodeGenerator(
		unixfs.WithUnixFSNodeDAGService(dagService),
		unixfs.WithUnixFSNodeBlockstore(bs),
	)
	return NewCARBuilder(bs, dagService, generator)
}

// StreamCAR is a convenience function that builds a directory tree from the
// given filesystem and writes it as a CARv1 to the provided writer.
// If wrapInDir is true, the content will be wrapped in a root directory (default behavior).
// If wrapInDir is false and there's only one file, the file itself will be the root.
func StreamCAR(ctx context.Context, filesystem fs.FS, w io.Writer, wrapInDir bool) (cid.Cid, error) {
	if err := ctx.Err(); err != nil {
		return cid.Cid{}, err
	}
	builder := newCARBuilder()
	rootCID, err := builder.BuildAndWrite(ctx, filesystem, w, wrapInDir)
	if err != nil {
		return cid.Cid{}, fmt.Errorf("build: %w", err)
	}

	return rootCID, nil
}

// StreamCARWithSize builds a directory tree and returns the root CID and the total CAR file size.
// This is useful for TUS uploads where the size must be known upfront.
// If wrapInDir is true, the content will be wrapped in a root directory (default behavior).
// If wrapInDir is false and there's only one file, the file itself will be the root.
func StreamCARWithSize(ctx context.Context, filesystem fs.FS, w io.Writer, wrapInDir bool) (cid.Cid, int64, error) {
	if err := ctx.Err(); err != nil {
		return cid.Cid{}, 0, err
	}
	builder := newCARBuilder()
	summary, err := builder.BuildSummary(ctx, filesystem, wrapInDir)
	if err != nil {
		return cid.Cid{}, 0, fmt.Errorf("build tree summary: %w", err)
	}

	// Normalize root CID to v1 format for consistency
	summary.RootCID = encoding.NormalizeCid(summary.RootCID)

	// Calculate CAR size with all overhead
	carSize, err := CalculateCARSize(summary)
	if err != nil {
		return cid.Cid{}, 0, err
	}

	// Write CAR to the provided writer
	if err = builder.WriteCAR(ctx, w); err != nil {
		return cid.Cid{}, 0, fmt.Errorf("write CAR: %w", err)
	}

	return summary.RootCID, carSize, nil
}

// CalculateCARSize computes the total CAR file size including header and all blocks.
// The CAR format is: [header] [block1] [block2] ... [blockN]
// Where each block is: [length] [CID bytes] [data bytes]
// The length is a varint encoding of (CID length + data length)
func CalculateCARSize(summary *TreeSummary) (int64, error) {
	// Calculate header size using the same method as WriteHeader
	v1Header := &carv1.CarHeader{
		Version: 1,
		Roots:   []cid.Cid{encoding.NormalizeCid(summary.RootCID)},
	}
	headerSize, err := carv1.HeaderSize(v1Header)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate CAR header size: %w", err)
	}

	// Calculate size for each block
	blocksSize := uint64(0)
	for i, blockCID := range summary.BlockOrder {
		normalizedCID := encoding.NormalizeCid(blockCID)

		cidLen := uint64(normalizedCID.ByteLen())
		dataLen := summary.BlockSizes[i]
		payloadSize := cidLen + dataLen
		// Each block: length varint + CID bytes + data bytes
		blocksSize += uint64(varint.UvarintSize(payloadSize)) + cidLen + dataLen
	}

	summary.CARSize = headerSize + blocksSize
	return int64(summary.CARSize), nil
}

// PrepareCAR returns a CARBuilder and TreeSummary ready for streaming.
// Use this when you need the CAR size or root CID before writing (e.g., to decide upload method).
// The caller is responsible for calling builder.WriteCAR() to stream the output.
//
// Example:
//   builder, summary, err := car.PrepareCAR(ctx, filesystem, wrapInDir)
//   carSize := car.CalculateCARSize(summary)
//   // ... decide upload method based on carSize ...
//   err = builder.WriteCAR(ctx, writer)
func PrepareCAR(ctx context.Context, filesystem fs.FS, wrapInDir bool) (*CARBuilder, *TreeSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	builder := newCARBuilder()
	summary, err := builder.BuildSummary(ctx, filesystem, wrapInDir)
	if err != nil {
		return nil, nil, err
	}
	return builder, summary, nil
}


