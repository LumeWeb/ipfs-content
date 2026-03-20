package dagnode

import (
	"context"
	"fmt"

	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/boxo/ipld/unixfs"
	pb "github.com/ipfs/boxo/ipld/unixfs/pb"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	legacy "github.com/ipfs/go-ipld-legacy"

	"go.lumeweb.com/ipfs-content/encoding"
)

const (
	// 240 KiB - minimum size to consider as potential chunk
	sizeThreshold = 240 * 1024
	// 256 KiB - standard IPFS chunk size
	typicalChunkSize = 256 * 1024
)

type NodeInfoType string

const (
	NodeTypeRaw      NodeInfoType = "raw"
	NodeTypeProtobuf NodeInfoType = "dag-pb"
	NodeTypeCBOR     NodeInfoType = "cbor"
	NodeTypeUnknown  NodeInfoType = "unknown"
)

type NodeInfo struct {
	// Identification and type
	// Node name (e.g., filename within a directory). Empty for non-directory nodes.
	Name       string

	// Binary CID representation for memory efficiency. Use GetCID() to reconstruct full CID.
	CIDBytes   []byte

	// Node encoding type: raw, dag-pb (protobuf), dag-cbor, or unknown.
	Type       NodeInfoType

	// UnixFS node type when IsUnixFS=true: File, Directory, Symlink, etc.
	UnixFSType pb.Data_DataType

	// Child links (parallel arrays: LinkCIDs[i], LinkNames[i], LinkSizes[i] describe link at index i)
	// Binary CIDs of child nodes. More compact than full Link structs.
	LinkCIDs  [][]byte

	// Names corresponding to each LinkCID (e.g., filenames in a directory).
	LinkNames []string

	// Link-reported sizes of child nodes. May be actual content size or 0 if unknown.
	LinkSizes []uint64

	// Flags and classification
	// Whether this node contains UnixFS protobuf data.
	IsUnixFS   bool

	// Whether this node is the root of a UnixFS file (i.e., has child links for a chunked file).
	IsFileRoot bool

	// Storage and encoding metrics
	// BlockSize is the raw byte size of the full encoded IPFS block as stored on disk or transmitted.
	// For dag-pb nodes, this includes protobuf framing overhead in addition to data payload.
	BlockSize uint64

	// DataSize is the size of the data payload within the node structure.
	// For RawNode: raw data payload size within the block
	// For ProtoNode (UnixFS): size of the serialized UnixFS protobuf metadata (NOT file content)
	// For CBORNode: raw block data size
	DataSize uint64

	// UnixFS chunking metadata
	// ChunkSizes is the list of child block sizes when a UnixFS file is split across multiple blocks.
	// Only non-empty for UnixFS file roots (IsFileRoot=true).
	// Example: A 1MB file split into 256KB chunks would have [262144, 262144, 262144, 262144].
	ChunkSizes []uint64

	// FileSize is the logical byte size of the UnixFS file before chunking, as reported by the file's metadata.
	// Only set for UnixFS file nodes (IsUnixFS=true and UnixFSType=Data_File).
	// Example: A file that's split across multiple 256KB blocks still has FileSize=1048576 (the original size).
	FileSize uint64
}

func AnalyzeNode(ctx context.Context, block blocks.Block) (*NodeInfo, error) {
	node, err := encoding.DecodeBlock(ctx, block)
	if err != nil {
		return nil, err
	}

	links := node.Links()
	linkCount := len(links)

	// Pre-allocate slices to avoid multiple allocations
	info := &NodeInfo{
		CIDBytes:  encoding.NormalizeCid(block.Cid()).Bytes(),
		LinkCIDs:  make([][]byte, 0, linkCount),
		LinkNames: make([]string, 0, linkCount),
		LinkSizes: make([]uint64, 0, linkCount),
		BlockSize: uint64(len(block.RawData())),
	}

	// Extract link data efficiently
	for _, link := range links {
		if link != nil {
			info.LinkCIDs = append(info.LinkCIDs, encoding.NormalizeCid(link.Cid).Bytes())
			info.LinkNames = append(info.LinkNames, link.Name)
			info.LinkSizes = append(info.LinkSizes, link.Size)
		}
	}

	switch n := node.(type) {
	case *merkledag.RawNode:
		info.Type = NodeTypeRaw
		info.DataSize = uint64(len(n.RawData()))
	case *merkledag.ProtoNode:
		info.Type = NodeTypeProtobuf
		data := n.Data()
		info.DataSize = uint64(len(data))

		if fsNode, err := unixfs.FSNodeFromBytes(data); err == nil {
			info.IsUnixFS = true
			info.UnixFSType = fsNode.Type()

			if fsNode.Type() == pb.Data_File {
				info.FileSize = fsNode.FileSize()
				info.IsFileRoot = len(info.LinkCIDs) > 0
				blockSizes := fsNode.BlockSizes()
				if len(blockSizes) > 0 {
					info.ChunkSizes = make([]uint64, len(blockSizes))
					copy(info.ChunkSizes, blockSizes)
				}
			}
		}
	case *encoding.CBORNode:
		info.Type = NodeTypeCBOR
		info.DataSize = uint64(len(n.Block.RawData()))
	case *legacy.LegacyNode:
		info.Type = NodeTypeUnknown
		info.DataSize = uint64(len(n.Block.RawData()))
	default:
		info.Type = NodeTypeUnknown
	}

	return info, nil
}

// isLikelyChunk determines if a size is characteristic of an IPFS file chunk
func isLikelyChunk(size uint64) bool {
	// Check for sizes in chunking range (240KB <= size <= 256KB)
	return size >= sizeThreshold && size <= typicalChunkSize
}

func IsPartialFile(info *NodeInfo) bool {
	if info.IsUnixFS && info.UnixFSType == pb.Data_File && !info.IsFileRoot {
		// UnixFS files use standard chunk size range (240KB <= size < 256KB)
		return isLikelyChunk(info.DataSize)
	}

	// Non-UnixFS raw data: 240KB <= size <= 256KB is considered partial
	if info.Type == NodeTypeRaw {
		return info.DataSize >= sizeThreshold && info.DataSize <= typicalChunkSize
	}

	// Non-UnixFS protobuf data: NEVER considered partial
	return false
}

// GetCID returns the CID from its binary representation
func (info *NodeInfo) GetCID() (cid.Cid, error) {
	return cid.Cast(info.CIDBytes)
}

// GetLinkAt returns the link at the specified index as a format.Link
func (info *NodeInfo) GetLinkAt(index int) (*format.Link, error) {
	if index < 0 || index >= len(info.LinkCIDs) {
		return nil, fmt.Errorf("link index out of bounds")
	}

	linkCID, err := cid.Cast(info.LinkCIDs[index])
	if err != nil {
		return nil, fmt.Errorf("invalid CID at index %d: %w", index, err)
	}

	return &format.Link{
		Name: info.LinkNames[index],
		Size: info.LinkSizes[index],
		Cid:  linkCID,
	}, nil
}

// LinkCount returns the number of links in this node
func (info *NodeInfo) LinkCount() int {
	return len(info.LinkCIDs)
}

func DetectPartialFile(ctx context.Context, block blocks.Block) (bool, error) {
	info, err := AnalyzeNode(ctx, block)
	if err != nil {
		return false, err
	}

	return IsPartialFile(info), nil
}
