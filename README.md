# ipfs-content

A Go library for IPFS content processing, providing utilities for CAR file streaming, archive detection, block storage, and UnixFS node generation.

## Installation

```bash
go get github.com/lumeweb/ipfs-content
```

## Packages

### car

CAR (Content Addressable aRchive) file generation utilities.

- **StreamCAR**: Stream CAR data to an io.Writer from directory structures
- **StreamCARWithSize**: Stream CAR data with pre-calculated size
- **CalculateCARSize**: Calculate the total CAR file size before writing
- **BuildTreeSummary**: Build metadata summary from a directory structure for efficient CAR generation

Supported options via `WithBufferSize`, `WithBlockstore`, `WithUnixFSNodeGenerator`.

```go
import "github.com/lumeweb/ipfs-content/car"

// Stream a directory to CAR format
err := car.StreamCAR(ctx, fileSystem, rootPath, writer)
```

### blockstore

In-memory block storage utilities.

- **NewLRUBlockstore**: Create a size-bounded LRU cache for IPFS blocks
- **NewMemoryBlockstore**: Create an unbounded in-memory block store

```go
import "github.com/lumeweb/ipfs-content/blockstore"

store := blockstore.NewLRUBlockstore(100 * 1024 * 1024) // 100MB cache
```

### unixfs

UnixFS node generation for IPFS.

- **NewUnixFSNodeGenerator**: Create a generator for UnixFS DAG nodes
- **GenerateDAGNode**: Generate a UnixFS node for a file or directory
- **WithUnixFSNodeDAGService**: Custom DAG service option
- **WithUnixFSNodeBlockstore**: Custom blockstore option

```go
import "github.com/lumeweb/ipfs-content/unixfs"

gen := unixfs.NewUnixFSNodeGenerator()
node, err := gen.GenerateDAGNode(ctx, file)
```

### archive

Archive type detection utilities.

- **DetectArchiveType**: Detect archive type by file extension and magic bytes
- **SupportedArchiveTypes**: ZIP, TAR, RAR, 7Z, CAR

```go
import "github.com/lumeweb/ipfs-content/archive"

archiveType, err := archive.DetectArchiveType(bytes.NewReader(data))
```

## License

MIT
