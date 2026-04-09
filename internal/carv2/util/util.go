// Package util implements low-level CAR file format utilities for reading and writing CARv1 files.
//
// This package adapts CAR format utilities from github.com/ipld/go-car/v2
// to work with the internal/io package in this project, providing primitives for
// length-prefixed data encoding/decoding, block reading/writing, and header parsing.
package util

import (
	"errors"
	"io"
	"math"

	"github.com/multiformats/go-varint"
	cid "github.com/ipfs/go-cid"

	internalio "go.lumeweb.com/ipfs-content/internal/io"
)

var ErrSectionTooLarge = errors.New("invalid section data, length of read beyond allowable maximum")
var ErrHeaderTooLarge = errors.New("invalid header data, length of read beyond allowable maximum")

// BytesReader is an interface that combines io.Reader and io.ByteReader.
type BytesReader interface {
	io.Reader
	io.ByteReader
}

// ReadNode reads a single node from a CARv1 stream.
// It reads a length-prefixed block, extracts the CID, and returns the CID along with the node data.
// The zeroLenAsEOF parameter controls whether a zero-length section is treated as EOF.
// The maxReadBytes parameter limits the maximum number of bytes to read.
func ReadNode(r io.Reader, zeroLenAsEOF bool, maxReadBytes uint64) (cid.Cid, []byte, error) {
	data, err := LdRead(r, zeroLenAsEOF, maxReadBytes)
	if err != nil {
		return cid.Cid{}, nil, err
	}

	n, c, err := cid.CidFromBytes(data)
	if err != nil {
		return cid.Cid{}, nil, err
	}

	return c, data[n:], nil
}

// ReadNodeHeader returns the specified CID of the node and the length of data to be read.
// This function is useful for peeking at the next node's metadata before reading its full content.
func ReadNodeHeader(r io.Reader, zeroLenAsEOF bool, maxReadBytes uint64) (cid.Cid, uint64, error) {
	maxReadBytes = min(maxReadBytes, math.MaxInt64) // io.LimitReader doesn't support uint64

	size, err := LdReadSize(r, zeroLenAsEOF, maxReadBytes)
	if err != nil {
		return cid.Cid{}, 0, err
	}

	if size == 0 {
		_, _, err := cid.CidFromBytes([]byte{}) // generate zero-byte CID error
		if err == nil {
			panic("expected zero-byte CID error")
		}
		return cid.Undef, 0, err
	}

	limitReader := io.LimitReader(r, int64(size)) // safe due to the `min` above
	n, c, err := cid.CidFromReader(limitReader)
	if err != nil {
		return cid.Cid{}, 0, err
	}

	return c, size - uint64(n), nil
}

// WriteBlock writes a single block to CARv1 format.
// It writes the length prefix, CID bytes, and block data to the writer.
// This is a convenience function that combines LdWrite with CID and data.
func WriteBlock(w io.Writer, c cid.Cid, data []byte) error {
	return LdWrite(w, c.Bytes(), data)
}

// LdWrite writes length-prefixed data to a writer.
// It writes the sum of lengths of all data slices as a varint,
// followed by the data slices themselves.
func LdWrite(w io.Writer, d ...[]byte) error {
	var sum uint64
	for _, s := range d {
		sum += uint64(len(s))
	}

	buf := make([]byte, 8)
	n := varint.PutUvarint(buf, sum)
	_, err := w.Write(buf[:n])
	if err != nil {
		return err
	}

	for _, s := range d {
		_, err = w.Write(s)
		if err != nil {
			return err
		}
	}

	return nil
}

// LdSize calculates the size in bytes of length-prefixed data.
// It returns the sum of data lengths plus the size of the varint encoding that length.
func LdSize(d ...[]byte) uint64 {
	var sum uint64
	for _, s := range d {
		sum += uint64(len(s))
	}
	s := varint.UvarintSize(sum)
	return sum + uint64(s)
}

// LdReadSize reads the length prefix from a CARv1 stream.
// The zeroLenAsEOF parameter controls whether a zero-length section is treated as EOF.
// The maxReadBytes parameter limits the maximum number of bytes to read.
func LdReadSize(r io.Reader, zeroLenAsEOF bool, maxReadBytes uint64) (uint64, error) {
	l, err := varint.ReadUvarint(internalio.ToByteReader(r))
	if err != nil {
		// If the length of bytes read is non-zero when the error is EOF then signal an unclean EOF.
		if l > 0 && err == io.EOF {
			return 0, io.ErrUnexpectedEOF
		}
		return 0, err
	} else if l == 0 && zeroLenAsEOF {
		return 0, io.EOF
	}

	if l > maxReadBytes { // Don't OOM
		return 0, ErrSectionTooLarge
	}
	return l, nil
}

// LdRead reads a length-prefixed data slice from a reader.
// The zeroLenAsEOF parameter controls whether a zero-length section is treated as EOF.
// The maxReadBytes parameter limits the maximum number of bytes to read.
func LdRead(r io.Reader, zeroLenAsEOF bool, maxReadBytes uint64) ([]byte, error) {
	l, err := LdReadSize(r, zeroLenAsEOF, maxReadBytes)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, l)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	return buf, nil
}
