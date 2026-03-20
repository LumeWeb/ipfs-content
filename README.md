# ipfs-content

A Go library for IPFS content processing. Streamlined CAR generation, archive extraction, and DAG utilities for building IPFS-powered applications.

## Overview

**ipfs-content** handles the heavy lifting of working with IPFS blocks and archives:

- **CAR files** — Efficient two-pass streaming with size pre-calculation for uploads
- **Archives** — Extract ZIP, TAR, RAR, 7Z formats with security validation
- **Block storage** — Memory-bounded LRU cache or DAG-aware tracking
- **UnixFS** — Build IPFS-compatible directory structures
- **DAG analysis** — Inspect blocks and detect chunk boundaries

Perfect for upload services, content gateways, and IPFS integrations.

## Quick Start

```bash
go get go.lumeweb.com/ipfs-content
```

Generate a CAR file from a directory:

```go
import "go.lumeweb.com/ipfs-content/car"

// Stream CAR to writer with 100MB memory limit
rootCID, carSize, err := car.StreamCARWithSize(
    ctx,
    os.DirFS("./content"),
    writer,
    100*1024*1024,
    true, // wrap in directory
)
```

Detect and extract archives:

```go
import "go.lumeweb.com/ipfs-content/archive"

// Detect format and extract to filesystem
extractor, err := archive.CreateExtractor(file)
defer extractor.Close()

fsys, err := extractor.Filesystem(ctx) // Browse like an fs.FS
```

## Key Capabilities

### CAR Generation

Stream CAR files efficiently with built-in memory management:

```go
// Direct streaming
rootCID, err := car.StreamCAR(ctx, fsys, writer, maxMemory, wrapInDir)

// Pre-calculate size before writing (useful for upload method selection)
builder, summary, err := car.PrepareCAR(ctx, fsys, maxMemory, wrapInDir)
carSize := car.CalculateCARSize(summary)
rootCID, err := car.WriteCAR(ctx, builder, writer)
```

**Two-pass approach:** Walk filesystem → Build metadata → Write CAR. Enables size pre-calculation without storing all blocks.

### Block Storage

Choose the right storage strategy for your workload:

```go
// LRU cache with size limit (memory-constrained)
store := blockstore.NewLRUBlockstore(100 * 1024 * 1024)

// DAG-aware storage (prevents "block not found" errors during regeneration)
levelStore := blockstore.NewLevelBlockStore()

// Unbounded (small datasets)
memStore := blockstore.NewInMemoryBlockstore()
```

### Archive Extraction

Support for ZIP, TAR, TAR.GZ, TAR.BZ2, RAR, and 7Z:

```go
// Detect format from file
format, err := archive.DetectFormat(reader)

// Browse contents as filesystem
fsys, err := extractor.Filesystem(ctx)
entries, err := fs.ReadDir(fsys, ".")
```

All paths are validated to prevent zip-slip attacks.

### DAG Analysis

Inspect IPFS blocks and chunk boundaries:

```go
// Analyze block metadata
info, err := dagnode.AnalyzeNode(ctx, block)
// info.Type, info.LinkCount(), info.ChunkSizes...

// Detect partial chunks (useful for streaming)
isPartial := dagnode.IsPartialFile(info) // 240KB-256KB range
```

## Design Patterns

- **Option pattern** — Flexible configuration (unixfs, httpclient, retry)
- **Registry pattern** — Extensible format detection and extraction
- **Stream-based I/O** — Work with archives and CARs without loading into memory
- **Security-first** — Path validation for archive extraction

## Complete Example

Convert an archive to a CAR file:

```go
package main

import (
    "context"
    "log"
    "os"

    "go.lumeweb.com/ipfs-content/archive"
    "go.lumeweb.com/ipfs-content/car"
)

func main() {
    ctx := context.Background()

    // Open archive
    file, err := os.Open("archive.zip")
    if err != nil {
        log.Fatal(err)
    }
    defer file.Close()

    // Detect format
    format, err := archive.DetectFormat(file)
    _ = format // ZIP, TAR, etc.

    // Extract as filesystem
    extractor, err := archive.CreateExtractor(file)
    if err != nil {
        log.Fatal(err)
    }
    defer extractor.Close()

    fsys, err := extractor.Filesystem(ctx)
    if err != nil {
        log.Fatal(err)
    }

    // Stream to CAR
    rootCID, err := car.StreamCAR(ctx, fsys, os.Stdout, 100*1024*1024, true)
    log.Printf("Root CID: %s", rootCID)
}
```

## Packages

- `car` — CAR streaming and size pre-calculation
- `blockstore` — LRU cache and DAG-aware storage
- `unixfs` — IPFS directory structures
- `archive` — Multi-format archive extraction
- `format` — Unified type system
- `dagnode` — DAG analytics and metadata
- `encoding` — IPLD decoding and CID normalization
- `paths` — IPFS/IPNS path constants
- `validation` — Security and component validation
- `retry` — Retry utilities
- `httpclient` — HTTP client factory

## Dependencies

- [boxo](https://github.com/ipfs/boxo) — IPFS implementation
- [go-ipld-prime](https://github.com/ipld/go-ipld-prime) — IPLD codecs
- [mholt/archives](https://github.com/mholt/archives) — Archive handling

## License

MIT
