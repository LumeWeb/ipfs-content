package encoding

import (
	"context"

	"github.com/ipfs/boxo/ipld/merkledag"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	legacy "github.com/ipfs/go-ipld-legacy"
	dagpb "github.com/ipld/go-codec-dagpb"
	_ "github.com/ipld/go-ipld-prime/codec/cbor"
	_ "github.com/ipld/go-ipld-prime/codec/dagcbor"
	_ "github.com/ipld/go-ipld-prime/codec/dagjson"
	_ "github.com/ipld/go-ipld-prime/codec/json"
	_ "github.com/ipld/go-ipld-prime/codec/raw"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/node/basicnode"
)

// decoderRegistry holds the global decoder registry for IPLD formats
var decoderRegistry *legacy.Decoder

func init() {
	// Initialize a new decoder and register supported codecs
	d := legacy.NewDecoder()

	// Register protobuf codec (dag-pb) for IPLD nodes
	d.RegisterCodec(cid.DagProtobuf, dagpb.Type.PBNode, merkledag.ProtoNodeConverter)

	// Register raw codec for raw data blocks
	d.RegisterCodec(cid.Raw, basicnode.Prototype.Bytes, merkledag.RawNodeConverter)
	d.RegisterCodec(cid.DagCBOR, basicnode.Prototype.Any, DagCborNodeConverter)
	decoderRegistry = d
}

// CBORNode is a wrapper around LegacyNode that uniquely identifies CBOR-encoded nodes
type CBORNode struct {
	legacy.LegacyNode
}

// IsCBORNode returns true if the node is a CBORNode wrapper
func IsCBORNode(node legacy.UniversalNode) bool {
	_, ok := node.(*CBORNode)
	return ok
}

// DagCborNodeConverter converts a go-ipld-prime node + block combination to a CBORNode
// that satisfies both current and legacy ipld formats for DAG-CBOR.
func DagCborNodeConverter(b blocks.Block, node ipld.Node) (legacy.UniversalNode, error) {
	return &CBORNode{legacy.LegacyNode{b, node}}, nil
}

// DecodeBlock decodes an IPFS block into an IPLD node using the registered codecs.
// This handles dag-pb (for ProtoNodes), raw blocks, and dag-cbor (for CBORNode).
func DecodeBlock(ctx context.Context, block blocks.Block) (format.Node, error) {
	return decoderRegistry.DecodeNode(ctx, block)
}

// ToV1 converts a CID to version 1 format if it's version 0.
// Returns the CID unchanged if already version 1, or cid.Undefined for unsupported versions.
// This is useful for normalizing CIDs to the canonical v1 representation.
func ToV1(c cid.Cid) cid.Cid {
	switch c.Version() {
	case 0:
		newCid := cid.NewCidV1(c.Type(), c.Hash())
		return newCid
	case 1:
		// Already v1 - return as-is
		return c
	default:
		// Unsupported version
		return cid.Undef
	}
}

// NormalizeCid ensures a CID is in version 1 format.
// This is used to maintain consistent CID representations across the system.
// If the CID is already version 1, it is returned unchanged.
// If the CID is version 0, it is converted to version 1.
func NormalizeCid(c cid.Cid) cid.Cid {
	return ToV1(c)
}
