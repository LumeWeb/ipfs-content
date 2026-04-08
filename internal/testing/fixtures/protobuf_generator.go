//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
)

func main() {
	size := flag.Int("size", 1024, "Size of protobuf data in bytes")
	partial := flag.Int("partial", 0, "Generate partial/corrupted protobuf data")
	flag.Parse()

	// Generate random data of specified size
	data := make([]byte, *size)
	rand.Read(data)

	// Create protobuf node using boxo's merkledag
	pbNode := merkledag.NodeWithData(data)

	// Set CID builder for dag-pb codec with SHA2-256
	err := pbNode.SetCidBuilder(cid.V1Builder{Codec: cid.DagProtobuf, MhType: uint64(multicodec.Sha2_256)})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set CID builder: %v\n", err)
		os.Exit(1)
	}

	// Marshal the protobuf node to get raw bytes
	protoData, err := pbNode.Marshal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal protobuf node: %v\n", err)
		os.Exit(1)
	}

	// Corrupt data if partial flag is set
	if *partial == 1 && len(protoData) > 10 {
		// Corrupt some bytes in the middle
		for i := len(protoData)/2 - 5; i < len(protoData)/2+5; i++ {
			protoData[i] = 0xFF
		}
	}

	// Put the block into IPFS (allow blocks >1MB for testing)
	cmd := exec.Command("ipfs", "dag", "put", "--input-codec=dag-pb", "--store-codec=dag-pb", "--allow-big-block")
	cmd.Stdin = bytes.NewReader(protoData)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ipfs dag put failed: %v\nOutput: %s\n", err, out)
		os.Exit(1)
	}

	// Print just the CID (removing any trailing newline)
	cidStr := string(bytes.TrimSpace(out))
	fmt.Print(cidStr)

	type ProtobufInfo struct {
		Size         int    `json:"size"`
		MessageSize  int    `json:"message_size"`
		Missing      int    `json:"missing"`
		CID          string `json:"cid"`
		IsPartial    bool   `json:"is_partial"`
		RawBlockSize int    `json:"raw_block_size"`
	}

	// Create JSON info file
	outputDir := os.Getenv("OUTPUT_DIR")
	if outputDir == "" {
		outputDir = "."
	}
	// Use 1/0 instead of true/false for consistent naming
	missingFlag := 0
	if *partial == 1 {
		missingFlag = 1
	}

	info := ProtobufInfo{
		Size:         *size,
		MessageSize:  len(data), // Size of the protobuf message data
		Missing:      missingFlag,
		CID:          cidStr,
		IsPartial:    *partial == 1,
		RawBlockSize: len(protoData), // Size of the serialized protobuf block
	}
	infoFile := filepath.Join(outputDir, fmt.Sprintf("protobuf_%d_%d.info.json", *size, missingFlag))
	file, err := os.Create(infoFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create info file: %v\n", err)
		os.Exit(1)
	}
	defer func(file *os.File) {
		err = file.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(file)

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(info); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write info file: %v\n", err)
		os.Exit(1)
	}
}
