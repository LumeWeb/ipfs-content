package fs

import (
	"context"
	"io/fs"

	"go.lumeweb.com/ipfs-content/car"
)

// GetDAGSizeFromFS calculates the actual DAG block size from any fs.FS.
// This returns the sum of all UnixFS block sizes including:
// - File data blocks
// - Directory blocks
// - UnixFS/CAR overhead
//
// Memory usage is bounded by the LRU cache in PrepareCAR regardless of file size.
//
// wrapInDir determines if the root should be wrapped in a directory (true) or
// if a single file should be the root (false).
func GetDAGSizeFromFS(ctx context.Context, filesystem fs.FS, wrapInDir bool) (uint64, error) {
	_, summary, err := car.PrepareCAR(ctx, filesystem, wrapInDir)
	if err != nil {
		return 0, err
	}
	return summary.TotalSize, nil
}

// GetLogicalFileSizeFromFS calculates the logical file size from a filesystem.
// This is the sum of all file sizes before UnixFS chunking, useful for quota
// validation showing the user's intended upload size.
func GetLogicalFileSizeFromFS(ctx context.Context, filesystem fs.FS, wrapInDir bool) (uint64, error) {
	_, summary, err := car.PrepareCAR(ctx, filesystem, wrapInDir)
	if err != nil {
		return 0, err
	}
	return summary.TotalLogicalFileSize(), nil
}
