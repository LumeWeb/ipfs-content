// Package carv2 provides low-level CAR file format utilities for reading and writing CARv1 files.
//
// This package adapts CAR format handling from github.com/ipld/go-car/v2
// to work with the internal/io package in this project, maintaining compatibility with
// the IPLD CARv1 specification.
package carv2

import (
	"context"
	"fmt"
	"io"

	carv2util "go.lumeweb.com/ipfs-content/internal/carv2/util"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	internalio "go.lumeweb.com/ipfs-content/internal/io"
)

const DefaultMaxAllowedHeaderSize uint64 = 32 << 20 // 32MiB
const DefaultMaxAllowedSectionSize uint64 = 8 << 20 // 8MiB

func init() {
	cbor.RegisterCborType(CarHeader{})
}

// Store is an interface for storing CARv1 blocks.
// It provides the Put method to add blocks to the store.
type Store interface {
	Put(context.Context, blocks.Block) error
}

// ReadStore is an interface for retrieving CARv1 blocks.
// It provides the Get method to retrieve blocks by CID from the store.
type ReadStore interface {
	Get(context.Context, cid.Cid) (blocks.Block, error)
}

// CarHeader represents the header of a CARv1 file.
// It contains the version number and a list of root CIDs.
type CarHeader struct {
	Roots   []cid.Cid
	Version uint64
}

// ReadHeaderAt reads a CARv1 header from an io.ReaderAt.
// It converts the ReaderAt to a Reader if necessary and reads the header.
// The maxReadBytes parameter limits the maximum number of bytes to read.
func ReadHeaderAt(at io.ReaderAt, maxReadBytes uint64) (*CarHeader, error) {
	var rr io.Reader
	switch r := at.(type) {
	case io.Reader:
		rr = r
	default:
		rr = internalio.ToReadSeeker(at)
	}
	return ReadHeader(rr, maxReadBytes)
}

// ReadHeader reads a CARv1 header from an io.Reader.
// It reads the length-prefixed CBOR-encoded header and decodes it.
// The maxReadBytes parameter limits the maximum number of bytes to read.
func ReadHeader(r io.Reader, maxReadBytes uint64) (*CarHeader, error) {
	hb, err := carv2util.LdRead(r, false, maxReadBytes)
	if err != nil {
		if err == carv2util.ErrSectionTooLarge {
			err = carv2util.ErrHeaderTooLarge
		}
		return nil, err
	}

	var ch CarHeader
	if err := cbor.DecodeInto(hb, &ch); err != nil {
		return nil, fmt.Errorf("invalid header: %v", err)
	}

	return &ch, nil
}

// WriteHeader writes a CARv1 header to an io.Writer.
// It encodes the header as CBOR and writes it in length-prefixed format.
func WriteHeader(h *CarHeader, w io.Writer) error {
	hb, err := cbor.Encode(h)
	if err != nil {
		return err
	}

	return carv2util.LdWrite(w, hb)
}

// HeaderSize calculates the size in bytes of a CARv1 header when encoded.
// It returns the size including the length prefix.
func HeaderSize(h *CarHeader) (uint64, error) {
	hb, err := cbor.Encode(h)
	if err != nil {
		return 0, err
	}

	return carv2util.LdSize(hb), nil
}

// CarReader provides functionality for reading CARv1 files.
// It wraps an io.Reader and provides methods to iterate through blocks.
type CarReader struct {
	r                     io.Reader
	Header                *CarHeader
	zeroLenAsEOF          bool
	maxAllowedSectionSize uint64
}

// NewCarReaderWithZeroLengthSectionAsEOF creates a new CarReader that treats zero-length sections as EOF.
func NewCarReaderWithZeroLengthSectionAsEOF(r io.Reader) (*CarReader, error) {
	return NewCarReaderWithoutDefaults(r, true, DefaultMaxAllowedHeaderSize, DefaultMaxAllowedSectionSize)
}

// NewCarReader creates a new CarReader from an io.Reader.
func NewCarReader(r io.Reader) (*CarReader, error) {
	return NewCarReaderWithoutDefaults(r, false, DefaultMaxAllowedHeaderSize, DefaultMaxAllowedSectionSize)
}

// NewCarReaderWithoutDefaults creates a new CarReader with custom parameters.
func NewCarReaderWithoutDefaults(r io.Reader, zeroLenAsEOF bool, maxAllowedHeaderSize uint64, maxAllowedSectionSize uint64) (*CarReader, error) {
	ch, err := ReadHeader(r, maxAllowedHeaderSize)
	if err != nil {
		return nil, err
	}

	if ch.Version != 1 {
		return nil, fmt.Errorf("invalid car version: %d", ch.Version)
	}

	if len(ch.Roots) == 0 {
		return nil, fmt.Errorf("empty car, no roots")
	}

	return &CarReader{
		r:                     r,
		Header:                ch,
		zeroLenAsEOF:          zeroLenAsEOF,
		maxAllowedSectionSize: maxAllowedSectionSize,
	}, nil
}

// Next reads the next block from the CAR file.
func (cr *CarReader) Next() (blocks.Block, error) {
	c, data, err := carv2util.ReadNode(cr.r, cr.zeroLenAsEOF, cr.maxAllowedSectionSize)
	if err != nil {
		return nil, err
	}

	hashed, err := c.Prefix().Sum(data)
	if err != nil {
		return nil, err
	}

	if !hashed.Equals(c) {
		return nil, fmt.Errorf("mismatch in content integrity, name: %s, data: %s", c, hashed)
	}

	return blocks.NewBlockWithCid(data, c)
}

type batchStore interface {
	PutMany(context.Context, []blocks.Block) error
}

// LoadCar reads a CARv1 file from an io.Reader and stores all blocks in the provided Store.
// It returns the CAR header after successfully loading all blocks.
// If the Store implements batchStore (PutMany), blocks are loaded in batches for efficiency.
func LoadCar(s Store, r io.Reader) (*CarHeader, error) {
	ctx := context.TODO()
	cr, err := NewCarReader(r)
	if err != nil {
		return nil, err
	}

	if bs, ok := s.(batchStore); ok {
		return loadCarFast(ctx, bs, cr)
	}

	return loadCarSlow(ctx, s, cr)
}

func loadCarFast(ctx context.Context, s batchStore, cr *CarReader) (*CarHeader, error) {
	var buf []blocks.Block
	for {
		blk, err := cr.Next()
		if err != nil {
			if err == io.EOF {
				if len(buf) > 0 {
					if err := s.PutMany(ctx, buf); err != nil {
						return nil, err
					}
				}
				return cr.Header, nil
			}
			return nil, err
		}

		buf = append(buf, blk)

		if len(buf) > 1000 {
			if err := s.PutMany(ctx, buf); err != nil {
				return nil, err
			}
			buf = buf[:0]
		}
	}
}

func loadCarSlow(ctx context.Context, s Store, cr *CarReader) (*CarHeader, error) {
	for {
		blk, err := cr.Next()
		if err != nil {
			if err == io.EOF {
				return cr.Header, nil
			}
			return nil, err
		}

		if err := s.Put(ctx, blk); err != nil {
			return nil, err
		}
	}
}

// Matches checks whether two headers match.
// Two headers are considered matching if:
//  1. They have the same version number, and
//  2. They contain the same root CIDs in any order.
//
// Note, this function explicitly ignores the order of roots.
// If order of roots matter use reflect.DeepEqual instead.
func (h CarHeader) Matches(other CarHeader) bool {
	if h.Version != other.Version {
		return false
	}
	thisLen := len(h.Roots)
	if thisLen != len(other.Roots) {
		return false
	}
	// Headers with a single root are popular.
	// Implement a fast execution path for popular cases.
	if thisLen == 1 {
		return h.Roots[0].Equals(other.Roots[0])
	}

	// Check other contains all roots.
	// TODO: should this be optimised for cases where the number of roots are large since it has O(N^2) complexity?
	for _, r := range h.Roots {
		if !other.containsRoot(r) {
			return false
		}
	}
	return true
}

func (h *CarHeader) containsRoot(root cid.Cid) bool {
	for _, r := range h.Roots {
		if r.Equals(root) {
			return true
		}
	}
	return false
}
