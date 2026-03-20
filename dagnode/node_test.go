package dagnode_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/boxo/ipld/unixfs"
	multicodec "github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pb "github.com/ipfs/boxo/ipld/unixfs/pb"

	"go.lumeweb.com/ipfs-content/dagnode"
)

var (
	finalDataDir = ""
)

const testDataDir = "internal/testing/fixtures/data"

func init() {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		// Fallback to relative path if runtime.Caller fails
		finalDataDir = testDataDir
	} else {
		// Navigate to the project root from the test file location
		testFile := file
		// Navigate up from dagnode/node_test.go to project root
		projectRoot := filepath.Join(filepath.Dir(testFile), "..")
		finalDataDir = filepath.Join(projectRoot, testDataDir)
	}
}

type InfoFile struct {
	File         string `json:"file"`
	Dir          string `json:"dir"`
	CID          string `json:"cid"`
	Size         uint64 `json:"size"`
	MessageSize  uint64 `json:"message_size"`
	RawBlockSize uint64 `json:"raw_block_size"`
	Missing      uint   `json:"missing"`
	IsPartial    bool   `json:"is_partial"`
	Type         string `json:"type"`
}

func loadInfoFromFile(t *testing.T, filename string) InfoFile {
	t.Helper()
	// Change .info to .info.json
	jsonFile := strings.ReplaceAll(filename, ".info", ".info.json")
	data, err := os.ReadFile(filepath.Join(finalDataDir, jsonFile))
	require.NoError(t, err)

	var info InfoFile
	err = json.Unmarshal(data, &info)
	require.NoError(t, err)
	return info
}

func loadBlockFromFile(t *testing.T, filename string) blocks.Block {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(finalDataDir, filename))
	if err != nil {
		t.Fatalf("failed to read block file: %v", err)
	}

	// Determine CID based on filename
	infoFilename := strings.ReplaceAll(filename, ".block", ".info")
	infoFile := loadInfoFromFile(t, infoFilename)

	if infoFile.CID == "N/A" || infoFile.CID == "" {
		t.Fatalf("CID is missing in info file %s", infoFilename)
	}

	_cid, err := cid.Decode(infoFile.CID)
	if err != nil {
		t.Fatalf("failed to decode CID from info file %s: %v", infoFilename, err)
	}

	block, err := blocks.NewBlockWithCid(data, _cid)
	if err != nil {
		t.Fatalf("failed to create block: %v", err)
	}
	return block
}

// createUnixFSFileBlock creates a properly encoded block from UnixFS file data
// with CID v1 encoding and DagProtobuf codec.
func createUnixFSFileBlock(t *testing.T, data []byte) blocks.Block {
	t.Helper()

	pbNode := merkledag.NodeWithData(data)
	err := pbNode.SetCidBuilder(cid.V1Builder{Codec: cid.DagProtobuf, MhType: uint64(multicodec.Sha2_256)})
	require.NoError(t, err)

	encoded, err := pbNode.Marshal()
	require.NoError(t, err, "Failed to marshal ProtoNode")

	cidBuilder := cid.V1Builder{Codec: cid.DagProtobuf, MhType: uint64(multicodec.Sha2_256)}
	c, err := cidBuilder.Sum(encoded)
	require.NoError(t, err)

	block, err := blocks.NewBlockWithCid(encoded, c)
	require.NoError(t, err)

	return block
}

// createRawBlock creates a properly encoded block from raw data
// with CID v1 encoding and Raw codec.
func createRawBlock(t *testing.T, data []byte) blocks.Block {
	t.Helper()

	cidBuilder := cid.V1Builder{Codec: cid.Raw, MhType: uint64(multicodec.Sha2_256)}
	c, err := cidBuilder.Sum(data)
	require.NoError(t, err)

	block, err := blocks.NewBlockWithCid(data, c)
	require.NoError(t, err)

	return block
}

func TestAnalyzeNode_RawData(t *testing.T) {
	tests := []struct {
		name     string
		filename string
	}{
		{"data240000", "data_240000_0.block"},
		{"data256000", "data_256000_0.block"},
		{"data262144", "data_262144_0.block"},
		{"data524288", "data_524288_0.block"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := loadBlockFromFile(t, tt.filename)
			info, err := dagnode.AnalyzeNode(context.Background(), block)
			require.NoError(t, err)

			assert.Equal(t, dagnode.NodeTypeRaw, info.Type)

			infoFile := loadInfoFromFile(t, strings.ReplaceAll(tt.filename, ".block", ".info"))
			assert.Equal(t, infoFile.Size, info.BlockSize)
		})
	}
}

func TestDetectPartialFile_RawData(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(finalDataDir, "data_*_0.block"))
	require.NoError(t, err)

	for _, file := range files {
		filename := filepath.Base(file)
		t.Run(filename, func(t *testing.T) {
			block := loadBlockFromFile(t, filename)
			isPartial, err := dagnode.DetectPartialFile(context.Background(), block)
			assert.NoError(t, err)

			// Load info from file
			infoFile := loadInfoFromFile(t, strings.ReplaceAll(filename, ".block", ".info"))

			assert.Equal(t, infoFile.IsPartial, isPartial)
		})
	}
}

func TestAnalyzeNode_ProtobufData(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(finalDataDir, "protobuf_*_0.block"))
	require.NoError(t, err)

	for _, file := range files {
		filename := filepath.Base(file)
		t.Run(filename, func(t *testing.T) {
			block := loadBlockFromFile(t, filename)
			info, err := dagnode.AnalyzeNode(context.Background(), block)
			require.NoError(t, err)
			assert.Equal(t, dagnode.NodeTypeProtobuf, info.Type)

			// Load info from file
			infoFile := loadInfoFromFile(t, strings.ReplaceAll(filename, ".block", ".info"))

			// Validate CID matches
			if infoFile.CID != "" && infoFile.CID != "N/A" {
				_cid, err := info.GetCID()
				require.NoError(t, err)
				assert.Equal(t, infoFile.CID, _cid.String(), "CID mismatch")
			}

			// Validate size - protobuf size should match the generated size from info file
			assert.Equal(t, infoFile.RawBlockSize, info.BlockSize, "Protobuf size mismatch")

			// Additional protobuf-specific validations
			assert.Equal(t, infoFile.MessageSize, info.DataSize, "Data size should match protobuf message size")
			assert.Equal(t, infoFile.RawBlockSize, uint64(len(block.RawData())), "Raw block size should match")
			assert.False(t, info.IsUnixFS, "Protobuf data should not be UnixFS")
			assert.Equal(t, pb.Data_Raw, info.UnixFSType, "Protobuf data should have Raw type")
		})
	}
}

func TestDetectPartialFile_ProtobufData(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(finalDataDir, "protobuf_*_0.block"))
	require.NoError(t, err)

	for _, file := range files {
		filename := filepath.Base(file)
		t.Run(filename, func(t *testing.T) {
			block := loadBlockFromFile(t, filename)
			isPartial, err := dagnode.DetectPartialFile(context.Background(), block)
			assert.NoError(t, err)

			// Load info from file
			infoFile := loadInfoFromFile(t, strings.ReplaceAll(filename, ".block", ".info"))

			assert.Equal(t, infoFile.IsPartial, isPartial)
		})
	}
}

func TestAnalyzeNode_UnixFSData(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(finalDataDir, "unixfs_*.block"))
	require.NoError(t, err)

	for _, file := range files {
		filename := filepath.Base(file)
		t.Run(filename, func(t *testing.T) {
			block := loadBlockFromFile(t, filename)
			info, err := dagnode.AnalyzeNode(context.Background(), block)
			require.NoError(t, err)

			// Load info from file
			infoFile := loadInfoFromFile(t, strings.ReplaceAll(filename, ".block", ".info"))

			// Check the type of the block
			if infoFile.Type == "raw_data" {
				// Skip UnixFS-specific assertions for raw data blocks
				assert.Equal(t, dagnode.NodeTypeRaw, info.Type)
				assert.Equal(t, infoFile.MessageSize, info.DataSize)
				assert.Equal(t, infoFile.RawBlockSize, info.BlockSize)
				return // Skip the rest of the UnixFS assertions
			}

			assert.Equal(t, dagnode.NodeTypeProtobuf, info.Type)
			assert.True(t, info.IsUnixFS)

			// Extract UnixFS type from filename
			var expectedUnixFSType pb.Data_DataType
			switch {
			case strings.Contains(filename, "file"):
				// _file.block files are the UnixFS protobuf root nodes
				if strings.HasSuffix(filename, "_file.block") {
					expectedUnixFSType = pb.Data_File
				} else {
					// Other file blocks are raw data chunks
					expectedUnixFSType = pb.Data_Raw
				}
			case strings.Contains(filename, "directory"):
				expectedUnixFSType = pb.Data_Directory
			case strings.Contains(filename, "symlink"):
				expectedUnixFSType = pb.Data_Symlink
			default:
				t.Fatalf("unknown UnixFS type in filename: %s", filename)
			}

			assert.Equal(t, expectedUnixFSType, info.UnixFSType)

			// Additional assertions based on UnixFS type
			switch expectedUnixFSType {
			case pb.Data_File:
				assert.Equal(t, infoFile.Size, info.BlockSize, "File data size should match info file")
				assert.GreaterOrEqual(t, info.LinkCount(), 0, "File should have zero or more links")

				if info.IsFileRoot {
					assert.Greater(t, info.BlockSize, uint64(0), "File root block should have non-zero size")
					assert.Greater(t, len(info.ChunkSizes), 0, "File root should have one or more block sizes")
				}

			case pb.Data_Directory:
				if infoFile.CID != "" && infoFile.CID != "N/A" {
					_cid, err := info.GetCID()
					require.NoError(t, err)
					assert.Equal(t, infoFile.CID, _cid.String(), "Directory CID should match info file")
				}
				assert.Greater(t, info.LinkCount(), 0, "Directory should have one or more links")
				assert.Equal(t, infoFile.RawBlockSize, info.BlockSize, "Directory data size should match info file")
				assert.Equal(t, infoFile.Size, info.BlockSize, "Directory data size should match info file")
				assert.Equal(t, infoFile.MessageSize, info.DataSize, "Directory message size should match data size")
				assert.Equal(t, infoFile.IsPartial, false, "Directory should never be marked as partial")

			}
		})
	}
}

func TestDetectPartialFile_UnixFSData(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(finalDataDir, "unixfs_*.block"))
	require.NoError(t, err)

	for _, file := range files {
		filename := filepath.Base(file)
		t.Run(filename, func(t *testing.T) {
			block := loadBlockFromFile(t, filename)
			isPartial, err := dagnode.DetectPartialFile(context.Background(), block)
			assert.NoError(t, err)

			// Load info from file
			infoFile := loadInfoFromFile(t, strings.ReplaceAll(filename, ".block", ".info"))

			assert.Equal(t, infoFile.IsPartial, isPartial)
		})
	}
}

func TestFileSize_UnixFSFile(t *testing.T) {
	// Test that FileSize is correctly extracted from UnixFS file nodes
	fileSize := uint64(500000)

	// Create UnixFS file data and block
	data := unixfs.FilePBData(nil, fileSize)
	block := createUnixFSFileBlock(t, data)

	// Analyze the node
	info, err := dagnode.AnalyzeNode(context.Background(), block)
	require.NoError(t, err, "Failed to analyze node")

	// Verify UnixFS properties
	assert.True(t, info.IsUnixFS, "Node should be UnixFS")
	assert.Equal(t, pb.Data_File, info.UnixFSType, "UnixFS type should be File")

	// Verify FileSize is set correctly
	assert.Equal(t, fileSize, info.FileSize, "FileSize should match the original file size")
}

func TestFileSize_NonUnixFS(t *testing.T) {
	// Test that FileSize is 0 for non-UnixFS nodes
	testData := []byte("test data")
	block := createRawBlock(t, testData)

	info, err := dagnode.AnalyzeNode(context.Background(), block)
	require.NoError(t, err, "Failed to analyze node")

	// Verify FileSize is 0 for raw data
	assert.False(t, info.IsUnixFS, "Raw data node should not be UnixFS")
	assert.Equal(t, uint64(0), info.FileSize, "FileSize should be 0 for non-UnixFS nodes")
}

func TestFileSize_MultiChunkFile(t *testing.T) {
	// Test FileSize for a file that would be split into multiple chunks
	fileSize := uint64(1048576) // 1MB

	// Create file with chunk sizes (256KB each)
	chunkSize := uint64(262144)
	numChunks := fileSize / chunkSize

	// Create UnixFS file data with block sizes using FSNode
	fsNode := unixfs.NewFSNode(pb.Data_File)
	for i := uint64(0); i < numChunks; i++ {
		fsNode.AddBlockSize(chunkSize)
	}

	// Serialize to bytes
	data, err := fsNode.GetBytes()
	require.NoError(t, err, "Failed to serialize FSNode")

	// Create block
	block := createUnixFSFileBlock(t, data)

	// Analyze the node
	info, err := dagnode.AnalyzeNode(context.Background(), block)
	require.NoError(t, err, "Failed to analyze node")

	// Verify FileSize is set correctly
	assert.Equal(t, fileSize, info.FileSize, "FileSize should match the original file size")
	assert.Equal(t, numChunks, uint64(len(info.ChunkSizes)), "Should have correct number of chunk sizes")
}
