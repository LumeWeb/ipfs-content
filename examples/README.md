# Go Examples

This directory contains example code demonstrating how to use the ipfs-content library.

## Package Examples

Each subdirectory contains examples for a specific package:

- **archive** - Archive format detection and extraction
- **blockstore** - Memory-based IPFS block storage
- **car** - CAR (Content Addressable Archive) file streaming
- **format** - Unified file format type system
- **httpclient** - HTTP client factory
- **retry** - Configurable retry logic
- **unixfs** - UnixFS node generation for IPFS
- **validation** - Path and component validation

## Running Examples

To run an example:

```bash
cd examples/<package>
go run main.go
```

## Example Overview

### Archive Example
Demonstrates archive format detection, extractor creation, and browsing archive contents as a filesystem.

### Blockstore Example
Shows how to use LRUBlockstore and InMemoryBlockstore for storing IPFS blocks with configurable memory limits.

### CAR Example
Demonstrates CAR file streaming with size pre-calculation, useful for TUS uploads where size must be known upfront.

### Format Example
Shows how to use the Format enum to identify and work with different file formats across the library.

### HTTP Client Example
Demonstrates creating configured HTTP clients and service clients using the factory pattern.

### Retry Example
Shows configurable retry logic with backoff strategies for handling transient failures.

### UnixFS Example
Demonstrates creating UnixFS nodes from readers and directories using the UnixFSNodeGenerator interface.

### Validation Example
Shows path validation to prevent zip-slip attacks and component validation for checking dependencies.

## Package Documentation

For detailed API documentation, see:
- Package godoc comments in each package's `doc.go` file
- [README.md](../README.md) for comprehensive library documentation
