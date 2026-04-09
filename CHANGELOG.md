## 0.1.13 (2026-04-09)

### Features

- add CARv2 format support and comprehensive test coverage

### Fixes

- handle duplicate CIDs correctly in equalBlocks method

## 0.1.12 (2026-04-09)

### Features

- enhance test fixtures with CAR file generation support
- enable external test harness access to fixture utilities
- add programmatic fixture discovery API

### Fixes

- resolve shellcheck warnings in test fixture scripts
- add fixture infrastructure and improve package vendoring
- implement missing Big Buck Bunny CAR fixture generation
- standardize error handling and fix duplicate command execution
- improve error handling robustness in test fixture generation
- clean up test log output to avoid memory address dumping

## 0.1.11 (2026-04-09)

### Fixes

- use DefaultSplitter when chunkSize <= 0

## 0.1.10 (2026-04-08)

### Features

- add CAR reading with memory-bounded support

### Fixes

- update protobuf_generator to use boxo for test fixture generation
- Seek error handling, deterministic map iteration, context support

## 0.1.9 (2026-04-08)

### Features

- track UnixFS logical file size separately from block size

## 0.1.8 (2026-03-21)

### Fixes

- handle '.' as file in single-file filesystems

## 0.1.7 (2026-03-20)

### Fixes

- prevent root directory duplication in tree hierarchy

## 0.1.6 (2026-03-20)

### Fixes

- exclude dot paths from CAR summary and fix test filesystem

## 0.1.5 (2026-03-20)

### Features

- add FileSize to NodeInfo for UnixFS file size tracking

## 0.1.4 (2026-03-20)

### Features

- add dagnode and paths packages for IPFS DAG node analysis

### Fixes

- align chunk detection bounds with test fixtures

## 0.1.3 (2026-03-17)

### Fixes

- handle all UnixFS block types in collectAllBlocks
- propagate decode errors in collectAllBlocks

## 0.1.2 (2026-03-15)

### Fixes

- implement LevelBlockStore for DAG-aware block management
- address PR review feedback for LevelBlockStore

## 0.1.1 (2026-03-14)

### Features

- add PrepareCAR functions for CAR size pre-calculation

## 0.1.0 (2026-03-14)

### Breaking Changes

- Initial version

### Features

- initial implementation of ipfs-content library

### Fixes

- resolve critical and high priority issues from PR review
- correct import paths
