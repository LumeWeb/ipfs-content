# ipfs-content

A Go library for IPFS content processing, providing utilities for CAR file streaming, archive detection and extraction, block storage management, and UnixFS node generation.

## Overview

This library provides a comprehensive set of tools for working with IPFS content in Go applications. It handles the complexities of:

- **CAR (Content Addressable Archive) file generation** with two-pass streaming for memory efficiency
- **Archive format detection and extraction** supporting ZIP, TAR, TAR.GZ, TAR.BZ2, RAR, and 7Z formats
- **Memory-bounded block storage** with LRU eviction
- **UnixFS node generation** for creating IPFS-compatible directory structures

## Installation

```bash
go get go.lumeweb.com/ipfs-content
```

## Packages

### car

CAR (Content Addressable aRchive) file streaming with two-pass generation for memory efficiency.

**Key Functions:**

- `StreamCAR(ctx, filesystem, writer, maxMemory, wrapInDir)` — Stream CAR data to an io.Writer from directory structures
- `StreamCARWithSize(ctx, filesystem, writer, maxMemory, wrapInDir)` — Stream CAR with pre-calculated size (useful for TUS uploads)
- `CalculateCARSize(summary)` — Calculate total CAR file size before writing
- `NewDAGServiceWithMemoryLimit(memoryLimit)` — Creates LRU blockstore with DAG service for memory-constrained operations
- `NewCARBuilder(bs, dagService, generator)` — Create a CAR builder for fine-grained control

**Two-Pass Architecture:**

The CAR package uses a two-pass generation strategy:
1. **Pass 1:** Walk filesystem and build TreeSummary with metadata (block CIDs, sizes, tree structure)
2. **Pass 2:** Write CARv1 using the summary, regenerating blocks on demand

This approach allows pre-calculating the CAR size without storing all blocks in memory.

```go
import "go.lumeweb.com/ipfs-content/car"

// Simple CAR streaming
rootCID, err := car.StreamCAR(ctx, os.DirFS("./content"), writer, 100*1024*1024, true)

// With size calculation for TUS uploads
rootCID, carSize, err := car.StreamCARWithSize(ctx, os.DirFS("./content"), writer, 100*1024*1024, true)
fmt.Printf("Root CID: %s, CAR Size: %d\n", rootCID, carSize)
```

---

### blockstore

Memory-based IPFS block storage with configurable memory limits.

**Types:**

- `LRUBlockstore` — Thread-safe LRU cache with size-based eviction (uses doubly-linked list)
- `InMemoryBlockstore` — Unbounded in-memory blockstore using boxo's map datastore

**Key Functions:**

- `NewLRUBlockstore(sizeLimit uint64)` — Create a size-bounded LRU cache
- `NewInMemoryBlockstore()` — Create an unbounded in-memory blockstore

```go
import "go.lumeweb.com/ipfs-content/blockstore"

// LRU blockstore with 100MB limit
store := blockstore.NewLRUBlockstore(100 * 1024 * 1024)

// In-memory blockstore (no size limit)
memStore := blockstore.NewInMemoryBlockstore()
```

**Thread Safety:** All operations use proper locking (RWMutex) for concurrent access.

---

### unixfs

UnixFS node generation for IPFS, creating DAG structures from readers and directories.

**Interface: `UnixFSNodeGenerator`**

```go
type UnixFSNodeGenerator interface {
    CreateNode(ctx context.Context, reader io.ReadSeekCloser) (format.Node, error)
    CreateUnixFSNode(ctx context.Context, r io.ReadSeekCloser, maxlinks int, chunkSize int64) (format.Node, error)
    CreateDAGFromReader(ctx context.Context, reader io.Reader, maxlinks int, chunkSize int64, rawLeaves bool) (format.Node, error)
    CreateDirectory() (unixfsio.Directory, error)
    CreateDirectoryWithLinks(ctx context.Context, children []DirectoryChild) (format.Node, error)
    GetDAGService() format.DAGService
    GetBlockstore() blockstore.Blockstore
}
```

**Key Functions:**

- `NewUnixFSNodeGenerator(options ...UnixFSNodeGeneratorOption)` — Create generator with options
- `WithUnixFSNodeDAGService(dagService)` — Configure custom DAG service
- `WithUnixFSNodeBlockstore(blockstore)` — Configure custom blockstore

**Options Pattern:**

```go
import "go.lumeweb.com/ipfs-content/unixfs"
import "go.lumeweb.com/ipfs-content/car"

// Create with custom components
generator := unixfs.NewUnixFSNodeGenerator(
    unixfs.WithUnixFSNodeDAGService(dagService),
    unixfs.WithUnixFSNodeBlockstore(blockstore),
)

// Create node from file
node, err := generator.CreateNode(ctx, file)
```

---

### archive

Archive detection and extraction supporting multiple formats.

**Supported Formats:** ZIP, TAR, TAR.GZ, TAR.BZ2, RAR, 7Z

**Key Functions:**

- `DetectFormat(reader io.Reader)` — Detect archive format by extension and magic bytes
- `CreateExtractor(reader archives.ReaderAtSeeker)` — Create appropriate extractor for the format
- `DefaultRegistry()` — Access the global archive registry

**Type: `ArchiveFileEntry`**

```go
type ArchiveFileEntry struct {
    Name() string           // Base name of file
    Size() int64            // File size in bytes
    Mode() os.FileMode      // File permissions
    ModTime() time.Time     // Modification time
    IsDir() bool            // Whether entry is a directory
    ContentReader() io.ReadCloser  // Direct access to file content
    Attributes() map[string]string // Format-specific attributes
}
```

**Type: `ArchiveExtractor`**

```go
type ArchiveExtractor interface {
    Format() Format
    Filesystem(ctx context.Context) (fs.FS, error)
    Close() error
}
```

```go
import "go.lumeweb.com/ipfs-content/archive"

// Detect and extract archive
format, err := archive.DetectFormat(reader)
if err != nil {
    log.Fatal(err)
}

extractor, err := archive.CreateExtractor(reader)
if err != nil {
    log.Fatal(err)
}
defer extractor.Close()

// Browse archive as filesystem
fsys, err := extractor.Filesystem(ctx)
entries, err := fs.ReadDir(fsys)
```

**Registry Pattern:** The archive package uses a registry pattern for format detection and extractor creation, allowing registration of custom detectors and extractors.

---

### format

Unified file format type system used across the library.

**Format Enum:**

```go
const (
    FormatUnknown  // Unknown or unrecognized format
    FormatCAR       // IPFS Content Addressable Archive
    FormatFile      // Regular single file
    FormatZIP       // ZIP archive
    FormatRAR       // RAR archive
    FormatTAR       // TAR archive
    FormatTAR_GZ    // Gzip-compressed TAR
    FormatTAR_BZ2   // Bzip2-compressed TAR
    Format7Z        // 7z archive
)
```

**Methods:**

- `IsUploadFormat() bool` — Returns true if format is supported for direct upload (only CAR)
- `IsArchiveFormat() bool` — Returns true if format is an extractable archive
- `String() string` — Human-readable format name
- `ParseFormat(s string) Format` — Parse string to Format

```go
import "go.lumeweb.com/ipfs-content/format"

if format.IsArchiveFormat() {
    // Extract archive contents
}

if format.IsUploadFormat() {
    // Ready for IPFS upload
}
```

---

### validation

Path validation and component validation utilities.

**Path Validation:**

```go
import "go.lumeweb.com/ipfs-content/validation"

// Validate archive paths to prevent zip-slip attacks
err := validation.ValidateArchivePath("path/within/archive")
if err != nil {
    // Reject malicious path
}
```

**Component Validation:**

```go
validator := validation.NewComponentValidator()

// Check required components
err := validator.ValidateRequired(
    validation.Component{Name: "DAGService", Value: dagService},
    validation.Component{Name: "Blockstore", Value: blockstore},
)

// Or use helper methods
err := validator.NotNil("DAGService", dagService)
err := validator.AllNotNil("DAGService", dagService, "Blockstore", blockstore)
```

---

### retry

Retry logic with configurable backoff strategies.

**Key Functions:**

- `Options(ctx context.Context)` — Default retry configuration (3 attempts, backoff with jitter, max 30s)
- `OptionsWithConfig(ctx, cfg OptionsConfig)` — Custom retry settings

```go
import "go.lumeweb.com/ipfs-content/retry"
import "github.com/avast/retry-go/v4"

// Default retry (3 attempts, exponential backoff with 5s max jitter, 30s max delay)
err := retry.Do(
    func() error { return someOperation() },
    retry.Options(ctx)...,
)

// Custom retry configuration
cfg := retry.OptionsConfig{
    Attempts:  5,
    MaxDelay: time.Minute,
    MaxJitter: 10 * time.Second,
}
err := retry.Do(
    func() error { return someOperation() },
    retry.OptionsWithConfig(ctx, cfg)...,
)
```

---

### httpclient

HTTP client factory with sensible defaults and options.

**Key Functions:**

- `CreateDefaultClient(opts ...func(*FactoryOptions))` — Create HTTP client with options
- `WithDefaultClient[T](factory ClientFunc[T])` — Higher-order function for service clients
- `WithCustomClient[T](factory ClientFunc[T])` — Factory with custom HTTP client

**Options:**

```go
import "go.lumeweb.com/ipfs-content/httpclient"

// Create client with custom options using functional options pattern
client := httpclient.CreateDefaultClient(
    func(opts *httpclient.FactoryOptions) {
        opts.WithTimeout(60 * time.Second)
        opts.WithKeepAlives(true)
        opts.WithMaxRetries(5)
    },
)

// Service client factory pattern
type MyService interface {
    DoSomething(ctx context.Context) error
}

createService := httpclient.WithDefaultClient(func(baseURL string, client *http.Client) (MyService, error) {
    return NewMyServiceClient(baseURL, client), nil
})

svc, err := createService("https://api.example.com")
```

---

## Key Patterns

### Option Pattern

Configuration uses the functional options pattern for flexibility:

```go
type Option func(*Options)

func WithTimeout(d time.Duration) Option {
    return func(o *Options) { o.Timeout = d }
}
```

### Two-Pass CAR Generation

Memory-efficient CAR streaming without storing all blocks:

```
Pass 1: Walk filesystem → Build TreeSummary (metadata only)
Pass 2: Write CAR → Regenerate blocks from summary
```

### Registry Pattern

Archive format detection uses a centralized registry:

```go
registry := archive.NewArchiveRegistry()
registry.RegisterDetector(myDetector)
registry.RegisterExtractor(format, myCreator)
```

### Security-First Validation

All archive paths are validated before extraction to prevent path traversal attacks.

---

## Complete Example

```go
package main

import (
    "context"
    "io"
    "log"
    "os"

    "go.lumeweb.com/ipfs-content/archive"
    "go.lumeweb.com/ipfs-content/car"
    "go.lumeweb.com/ipfs-content/format"
)

func main() {
    ctx := context.Background()

    // Open archive file
    file, err := os.Open("archive.zip")
    if err != nil {
        log.Fatal(err)
    }
    defer file.Close()

    // Detect archive format
    archiveFormat, err := archive.DetectFormat(file)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Detected format: %s", archiveFormat)

    // Create extractor
    extractor, err := archive.CreateExtractor(file)
    if err != nil {
        log.Fatal(err)
    }
    defer extractor.Close()

    // Browse archive contents
    fsys, err := extractor.Filesystem(ctx)
    if err != nil {
        log.Fatal(err)
    }

    // Stream archive contents to CAR
    writer := os.Stdout
    rootCID, err := car.StreamCAR(ctx, fsys, ".", writer, 100*1024*1024, true)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Root CID: %s", rootCID)
}
```

## Dependencies

- [boxo](https://github.com/ipfs/boxo) — IPFS implementation libraries
- [go-cid](https://github.com/ipfs/go-cid) — CID implementation
- [go-ipld-format](https://github.com/ipfs/go-ipld-format) — IPLD format interface
- [mholt/archives](https://github.com/mholt/archives) — Archive extraction backend
- [avast/retry-go](https://github.com/avast/retry-go) — Retry logic
- [docker/go-units](https://github.com/docker/go-units) — Human-readable size formatting

## License

MIT