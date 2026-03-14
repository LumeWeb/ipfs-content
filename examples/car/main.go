// Package main demonstrates CAR (Content Addressable Archive) file streaming.
//
// This example shows how to stream CAR files from a directory with size pre-calculation,
// useful for TUS uploads where the total size must be known upfront.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"go.lumeweb.com/ipfs-content/car"
)

func main() {
	ctx := context.Background()

	// Create a temporary directory with test content
	tmpDir, err := os.MkdirTemp("", "car-example-*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test file
	testFile := fmt.Sprintf("%s/test.txt", tmpDir)
	err = os.WriteFile(testFile, []byte("Hello, IPFS!"), 0644)
	if err != nil {
		log.Fatalf("Failed to create test file: %v", err)
	}

	// Example 1: Simple CAR streaming
	fmt.Println("Example 1: Simple CAR Streaming")
	carFile1, err := os.CreateTemp("", "car1-*.car")
	if err != nil {
		log.Fatalf("Failed to create CAR file: %v", err)
	}
	defer carFile1.Close()
	defer os.Remove(carFile1.Name())

	// Stream CAR with 100MB memory limit
	rootCID, err := car.StreamCAR(ctx, os.DirFS(tmpDir), carFile1, 100*1024*1024, true)
	if err != nil {
		log.Fatalf("Failed to stream CAR: %v", err)
	}
	fmt.Printf("Created CAR file with root CID: %s\n", rootCID)

	// Example 2: CAR streaming with size calculation
	fmt.Println("\nExample 2: CAR Streaming with Size Calculation")
	carFile2, err := os.CreateTemp("", "car2-*.car")
	if err != nil {
		log.Fatalf("Failed to create CAR file: %v", err)
	}
	defer carFile2.Close()
	defer os.Remove(carFile2.Name())

	// Stream CAR with size pre-calculation (useful for TUS uploads)
	rootCID, carSize, err := car.StreamCARWithSize(ctx, os.DirFS(tmpDir), carFile2, 100*1024*1024, true)
	if err != nil {
		log.Fatalf("Failed to stream CAR with size: %v", err)
	}
	fmt.Printf("Root CID: %s, CAR Size: %d bytes (%.2f MB)\n", rootCID, carSize, float64(carSize)/(1024*1024))

	// Example 3: CAR size calculation before streaming
	fmt.Println("\nExample 3: Size Calculation Before Streaming")

	// First pass: build builder and summary
	builder, summary, err := car.PrepareCAR(ctx, os.DirFS(tmpDir), 100*1024*1024, true)
	if err != nil {
		log.Fatalf("Failed to build summary: %v", err)
	}

	// Calculate size without writing
	size, err := car.CalculateCARSize(summary)
	if err != nil {
		log.Fatalf("Failed to calculate size: %v", err)
	}
	fmt.Printf("Total CAR size would be: %d bytes (%.2f MB)\n", size, float64(size)/(1024*1024))

	// Optionally write the CAR file if desired
	fmt.Println("\nWriting the prepared CAR file...")
	carFile4, err := os.CreateTemp("", "car4-*.car")
	if err != nil {
		log.Fatalf("Failed to create CAR file: %v", err)
	}
	defer carFile4.Close()
	defer os.Remove(carFile4.Name())

	err = builder.WriteCAR(ctx, carFile4)
	if err != nil {
		log.Fatalf("Failed to write CAR: %v", err)
	}
	fmt.Println("CAR file created successfully from prepared builder")

	// Example 4: Streaming to stdout (useful for piped output)
	fmt.Println("Example 4: Writing CAR to temp file")

	carFile3, err := os.CreateTemp("", "car3-*.car")
	if err != nil {
		log.Fatalf("Failed to create CAR file: %v", err)
	}
	defer carFile3.Close()
	defer os.Remove(carFile3.Name())

	rootCID, err = car.StreamCAR(ctx, os.DirFS(tmpDir), carFile3, 100*1024*1024, true)
	if err != nil {
		log.Fatalf("Failed to stream CAR: %v", err)
	}
	fmt.Printf("Root CID: %s\n", rootCID)

	// Display some information about the file
	info, _ := carFile3.Stat()
	fmt.Printf("CAR file size: %d bytes\n", info.Size())
}
