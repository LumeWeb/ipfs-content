# CAR Examples

This directory contains examples for using the car package, which provides
CAR (Content Addressable Archive) file streaming with two-pass generation for memory efficiency.

## Examples

- `main.go` - CAR streaming with size calculation and pre-calculation examples

## Running the Examples

```bash
go run main.go
```

## Features Demonstrated

1. Simple CAR streaming from a directory
2. CAR streaming with size pre-calculation (useful for TUS uploads)
3. Size calculation before writing CAR
4. Streaming to stdout and reading CAR files
