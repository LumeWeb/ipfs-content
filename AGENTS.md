# AGENTS.md
This file provides guidance to various AI agents when working with code in this repository.

## Project Overview

This is **ipfs-content**, a Go library for IPFS content processing. It provides utilities for:
- CAR (Content Addressable aRchive) file streaming with size pre-calculation
- Archive detection and extraction (ZIP, TAR, RAR, 7Z)
- Block storage implementations (LRU cache and in-memory)
- UnixFS DAG node generation
- HTTP client factory with retry logic

The library is primarily consumed by services that need to work with IPFS content, particularly for uploading and processing CAR files.

## Common Commands

### Building
```bash
go build -v ./...
```

### Testing
```bash
# Run all tests with race detection and coverage
go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

# Run specific package tests
go test -v ./car/...
go test -v ./archive/...

# Run specific test function
go test -v ./unixfs/... -run TestNewUnixFSNodeGenerator
```

### Coverage
```bash
# Generate coverage report
go test -race -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -func=coverage.out
go tool cover -html=coverage.out
```

### Mocks
```bash
# Generate or regenerate mocks for the archive package
mockery
```

### Dependencies
```bash
go mod download
go mod verify
go mod tidy
```

## High-Level Architecture

### Package Structure

The library is organized into several focused packages with clear responsibilities:

#### `car`
Core CAR (Content Addressable aRchive) file generation package. Uses a **two-pass approach**:
- **Pass 1**: Walk the filesystem and build a `TreeSummary` with metadata
- **Pass 2**: Write CARv1 using the summary, regenerating blocks on demand

This allows streaming with size pre-calculation (critical for TUS uploads where size must be known upfront). Key functions:
- `StreamCAR()` - Stream CAR data to io.Writer
- `StreamCARWithSize()` - Stream CAR with pre-calculated size
- `CalculateCARSize()` - Calculate total CAR file size before writing
- `PrepareCAR()` - Build CARBuilder and TreeSummary for size pre-calculation and deferred streaming
- `PrepareCARWithDefaultMemory()` - Prepare CAR with default 100MB memory limit
- `NewDAGServiceWithMemoryLimit()` - Creates LRU blockstore, blockservice, and DAG service
- `NewDAGServiceWithLevelAware()` - Creates LevelBlockStore with DAG service for two-phase generation

The package uses blockstores (from `blockstore` package) to limit memory usage:
- **LRUBlockstore** for memory-constrained operations
- **LevelBlockStore** for DAG-heavy operations requiring cohort-based level tracking

#### `blockstore`
Implements memory-based block storage for IPFS blocks:
- `LRUBlockstore` - Size-bounded LRU cache (thread-safe, evicts least-recently-used blocks when limit exceeded)
- `InMemoryBlockstore` - Unbounded in-memory blockstore using boxo's map datastore
- `LevelBlockStore` - DAG-aware blockstore with cohort-based level tracking and rotation

The `LRUBlockstore` uses a doubly-linked list to track usage order and evicts blocks when the total size limit is exceeded. This is critical for CAR generation where you need to limit memory usage.

`LevelBlockStore` tracks DAG depth levels using cohort rotation (default 10 levels) to prevent "block not found" errors during two-phase CAR generation. Uses bidirectional parent-child and ancestor relationship tracking instead of memory-based eviction.

#### `unixfs`
UnixFS node generation for IPFS. Defines `UnixFSNodeGenerator` interface with methods:
- `CreateNode()` / `CreateUnixFSNode()` - Create UnixFS nodes from readers
- `CreateDirectory()` / `CreateDirectoryWithLinks()` - Create directory nodes
- `CreateDAGFromReader()` - Create DAG from reader
- `GetDAGService()` / `GetBlockstore()` - Access underlying IPFS components

The default implementation `IPFSUnixFSNodeGenerator` uses boxo/merkledag libraries. Options pattern allows dependency injection:
- `WithUnixFSNodeDAGService(dagService)`
- `WithUnixFSNodeBlockstore(blockstore)`

#### `archive`
Archive detection and extraction with support for ZIP, TAR, TAR.GZ, TAR.BZ2, RAR, and 7Z formats. Key components:

**Format Detection**: Uses `ArchiveRegistry` pattern for format detection and extractor creation. Detects format using:
- Magic bytes detection for CAR and compressed TAR archives
- MIME type mapping for standard formats (ZIP, TAR, RAR, 7Z)
- The `github.com/mholt/archives` library for archive processing

**Extraction Flow**:
1. `DetectFormat(reader)` identifies archive type
2. `CreateExtractor(reader)` creates appropriate extractor
3. `Filesystem(ctx)` provides `fs.FS` interface to browse archive
4. Each file entry returns `ArchiveFileEntry` with streaming `ContentReader`

**Security**: All extractors validate paths using `validation.ValidateArchivePath()` to prevent zip-slip attacks (path traversal).

**Backend**: Uses `github.com/mholt/archives` as the actual extraction backend via `ArchivesDriver` abstraction.

**Mocking**: Uses mockery (configured in `.mockery.yaml`) to generate `mocks/ArchiveExtractor` for testing.

#### `format`
Unified file format type system covering both upload formats and archive formats. Defines `Format` enum with constants:
- `FormatUnknown`, `FormatCAR`, `FormatFile`, `FormatZIP`
- `FormatRAR`, `FormatTAR`, `FormatTAR_GZ`, `FormatTAR_BZ2`, `Format7Z`

Methods:
- `IsUploadFormat()` - Returns true if format supports direct upload (currently only CAR)
- `IsArchiveFormat()` - Returns true if format is an archive container that can be extracted

#### `validation`
Path validation for security. `ValidateArchivePath()` prevents path traversal attacks by:
- Checking for suspicious patterns (`\`, `//`, `./`)
- Cleaning the path and checking if it escapes archive root
- Rejecting absolute paths

Also provides component validation utilities with fluent API: `ValidateRequired()`, `NotNil()`, etc.

#### `retry`
Retry logic using `github.com/avast/retry-go/v4`. Provides:
- `Options(ctx)` - Default retry configuration (3 attempts, backoff delay with jitter, max delay 30s)
- `OptionsWithConfig(ctx, cfg)` - Customizable retry settings

#### `httpclient`
HTTP client factory for creating configured HTTP clients with retry and timeout support. Provides:
- `CreateDefaultClient(opts...)` - Create client with default options
- `WithDefaultClient[T](factory)` - Higher-order function to create service clients

Options: timeout, max retries, keep-alives, etc.

#### `dagnode`
IPFS DAG node analysis for content structure identification and metadata extraction. Provides:
- `AnalyzeNode(ctx, block)` - Analyze IPFS blocks and extract comprehensive metadata (type, links, sizes, UnixFS info)
- `IsPartialFile(info)` - Determine if a block represents a partial file chunk (240KB-256KB range)
- `NodeInfo` struct - Memory-efficient binary CID storage with type detection and chunk size tracking

Supports node type detection (raw, protobuf, CBOR), UnixFS metadata extraction, and partial file identification for efficient chunk-based processing.

#### `encoding`
Public package for IPLD block decoding and CID normalization. Provides:
- `DecodeBlock(ctx, block)` - Decode IPFS blocks into IPLD nodes using registered codecs
- `NormalizeCid(cid.Cid)` - Convert CID to version 1 format for consistency across the system
- `DagCborNodeConverter` - Handle CBOR-encoded blocks for DAG-CBOR decoding

Supports codecs: dag-pb (protobuf), raw data blocks, and dag-cbor for CBOR nodes.

#### `paths`
IPFS and IPNS path constants for standardized path handling. Provides:
- `IPFSPathPrefix` - "/ipfs/" prefix for IPFS content paths
- `IPNSPathPrefix` - "/ipns/" prefix for IPNS content paths

Ensures consistent path handling for IPFS and IPNS URIs across the library.

#### Internal Packages
- `internal/encoding` - Legacy internal encoding (CID normalization moved to public `encoding` package)
- `internal/io` - IO utilities (readers, converters, read-seek wrappers)
- `internal/carv1` - CARv1 format utilities
- `internal/testing` - Test fixture infrastructure and helpers

### Package Dependency Relationships

```
car → blockstore, unixfs, internal/carv1, encoding
    → uses LRU blockstore or LevelBlockStore for memory management
    → uses encoding.DecodeBlock for DAG traversal
unixfs → uses boxo/merkledag, boxo/blockstore
archive → uses mholt/archives (backend), validation (security)
format → standalone, used by multiple packages
validation → standalone, used by archive package
retry → standalone, used by httpclient and other packages
httpclient → standalone
dagnode → encoding (for DecodeBlock, NormalizeCid)
encoding → standalone, used by car, dagnode, and other packages
paths → standalone, used by IPFS/IPNS path handling
```

### Key Patterns and Idioms

**Option Pattern for Configuration**: Used in `unixfs`, `httpclient`, `retry` packages to provide flexible optional configuration:
```go
func WithUnixFSNodeDAGService(dagService format.DAGService) UnixFSNodeGeneratorOption
func WithTimeout(timeout time.Duration) *FactoryOptions
```

**Two-Pass Generation**: CAR file generation uses a two-pass approach to enable size pre-calculation without storing all blocks in memory. `PrepareCAR()` enables inspection before streaming for upload method selection.

**DAG-Aware Block Storage**: `LevelBlockStore` uses cohort rotation and bidirectional parent-child tracking instead of LRU eviction. This prevents "block not found" errors during CAR generation by maintaining structured DAG metadata.

**LRU Eviction Blockstore**: The custom `LRUBlockstore` implementation uses a doubly-linked list to track access order and evict blocks when size limit is exceeded.

**Registry Pattern**: `ArchiveRegistry` manages format detection and extractor creation, allowing dynamic registration of new formats.

**Stream-Based Archive Extraction**: `ArchiveExtractor.Filesystem()` returns `fs.FS` interface; `ArchiveFileEntry.ContentReader()` provides streaming access to file content.

**CID Normalization**: CIDs are consistently normalized to v1 format using `encoding.NormalizeCid()` throughout the codebase for consistency.

## Important Constraints

### Memory Management
- Configure memory limits when generating CAR files via `NewDAGServiceWithMemoryLimit()`
- For DAG-heavy operations, use `NewDAGServiceWithLevelAware()` with `LevelBlockStore` to prevent block eviction
- Default memory limit is 100MB (`DefaultMemoryLimit`)
- Use `LRUBlockstore` for bounded memory, `InMemoryBlockstore` only when data size is limited
- LevelBlockStore uses cohort rotation (default 10 levels) to track DAG depth without eviction

### Security
- All archive paths MUST be validated using `validation.ValidateArchivePath()` before processing
- Reject suspicious patterns (`\`, `//`, `./`, absolute paths)
- Use package-level validation functions rather than re-implementing path validation

### Format Handling
- Use `Format` type enum from `format` package for all format-related code
- Check `IsUploadFormat()` before assuming format supports direct upload
- `FormatFile` represents a single file, not an archive container
- `FormatCAR` is not an archive that needs traditional extraction

### CID Consistency
- Normalize CIDs to v1 using `encoding.NormalizeCid()` for consistency across the system
- The library defaults to CIDv1 encoding (DagProtobuf with Sha2_256)

### Testing Patterns
- Use `setupNodeGeneratorTest()` pattern from `unixfs/unixfs_generator_test.go` to create test fixtures
- Tests use `testing/fstest.MapFS` for filesystem testing
- Use `github.com/stretchr/testify` for assertions (assert, require)
- Mocks are generated by mockery and stored in `archive/mocks/`

### Go Version
- Requires Go 1.26.0 or later (specified in go.mod)
- Uses standard Go tooling; no custom build scripts

### Dependencies
- Core IPFS libraries: `github.com/ipfs/boxo`, `github.com/ipfs/go-cid`, etc.
- IPLD libraries: `github.com/ipld/go-ipld-prime`, `github.com/ipld/go-codec-dagpb`
- Archive handling: `github.com/mholt/archives`
- File type detection: `github.com/h2non/filetype`
- Utilities: `github.com/samber/lo`, `github.com/docker/go-units`

## Testing Notes

### Test Structure
- Tests are co-located with source files (`*_test.go`)
- Use `t.Parallel()` for concurrent tests where possible
- Test helpers are in `archive/archive_test_helpers.go` for archive-related testing

### Running Tests
- CI runs `go test -v -race -coverprofile=coverage.out -covermode=atomic ./...`
- Reports coverage percentage on pull requests
- Tests should be self-contained and not rely on external files unless using `fstest.MapFS`

### Mock Generation
- Mocks are generated using mockery with configuration in `.mockery.yaml`
- Only mocks the `ArchiveExtractor` interface from the `archive` package
- Run `mockery` to regenerate mocks after interface changes

## Build System

### Continuous Integration
- GitHub Actions workflow in `.github/workflows/ci.yml`
- Runs on `ubuntu-latest` with stable Go version
- Triggers on push and pull requests to main/master/develop branches

### CI Steps
1. Checkout code
2. Set up Go (stable version)
3. Download dependencies (`go mod download`)
4. Verify dependencies (`go mod verify`)
5. Build (`go build -v ./...`)
6. Test with race detector and coverage
7. Generate coverage reports
8. Post coverage comment on PRs

### Configuration Files
- `go.mod` / `go.sum` - Go module dependency management
- `.mockery.yaml` - Mockery configuration for generating mocks
- `.gitignore` - Standard Go gitignore patterns

## Linting and Formatting

The project follows standard Go conventions. No additional linting tools are configured in CI. Follow Go's standard:
- `go fmt` for formatting
- `go vet` for basic static analysis
- Use `golint` or `golangci-lint` locally if desired

## Documentation

### Adding Comments
- Add godoc comments for all exported functions, types, and constants
- Comments should explain the **why**, not the **what**
- See `GOLANG_COMMENTS.md` in project rules for detailed commenting guidelines

### Example Usage
Refer to `README.md` for package-level usage examples. Add example code comments in godoc for complex APIs.