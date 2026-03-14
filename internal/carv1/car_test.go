package carv1

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/multiformats/go-varint"

	"github.com/stretchr/testify/require"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
)

func TestEOFHandling(t *testing.T) {
	// fixture is a clean single-block, single-root CAR
	fixture, err := hex.DecodeString("3aa265726f6f747381d82a58250001711220151fe9e73c6267a7060c6f6c4cca943c236f4b196723489608edb42a8b8fa80b6776657273696f6e012c01711220151fe9e73c6267a7060c6f6c4cca943c236f4b196723489608edb42a8b8fa80ba165646f646779f5")
	if err != nil {
		t.Fatal(err)
	}

	load := func(t *testing.T, byts []byte) *CarReader {
		cr, err := NewCarReader(bytes.NewReader(byts))
		if err != nil {
			t.Fatal(err)
		}

		blk, err := cr.Next()
		if err != nil {
			t.Fatal(err)
		}
		if blk.Cid().String() != "bafyreiavd7u6opdcm6tqmddpnrgmvfb4enxuwglhenejmchnwqvixd5ibm" {
			t.Fatal("unexpected CID")
		}

		return cr
	}

	t.Run("CleanEOF", func(t *testing.T) {
		cr := load(t, fixture)

		blk, err := cr.Next()
		if err != io.EOF {
			t.Fatal("Didn't get expected EOF")
		}
		if blk != nil {
			t.Fatal("EOF returned expected block")
		}
	})

	t.Run("BadVarint", func(t *testing.T) {
		fixtureBadVarint := append(fixture, 160)
		cr := load(t, fixtureBadVarint)

		blk, err := cr.Next()
		if err != io.ErrUnexpectedEOF {
			t.Fatal("Didn't get unexpected EOF")
		}
		if blk != nil {
			t.Fatal("EOF returned unexpected block")
		}
	})

	t.Run("TruncatedBlock", func(t *testing.T) {
		fixtureTruncatedBlock := append(fixture, 100, 0, 0)
		cr := load(t, fixtureTruncatedBlock)

		blk, err := cr.Next()
		if err != io.ErrUnexpectedEOF {
			t.Fatal("Didn't get unexpected EOF")
		}
		if blk != nil {
			t.Fatal("EOF returned unexpected block")
		}
	})
}

func TestBadHeaders(t *testing.T) {
	testCases := []struct {
		name   string
		hex    string
		errStr string // either the whole error string
		errPfx string // or just the prefix
	}{
		{
			"{version:2}",
			"0aa16776657273696f6e02",
			"invalid car version: 2",
			"",
		},
		{
			// an unfortunate error because we don't use a pointer
			"{roots:[baeaaaa3bmjrq]}",
			"13a165726f6f747381d82a480001000003616263",
			"invalid car version: 0",
			"",
		},
		{
			"{version:\"1\",roots:[baeaaaa3bmjrq]}",
			"1da265726f6f747381d82a4800010000036162636776657273696f6e6131",
			"", "invalid header: ",
		},
		{
			"{version:1}",
			"0aa16776657273696f6e01",
			"empty car, no roots",
			"",
		},
		{
			"{version:1,roots:{cid:baeaaaa3bmjrq}}",
			"20a265726f6f7473a163636964d82a4800010000036162636776657273696f6e01",
			"",
			"invalid header: ",
		},
		{
			"{version:1,roots:[baeaaaa3bmjrq],blip:true}",
			"22a364626c6970f565726f6f747381d82a4800010000036162636776657273696f6e01",
			"",
			"invalid header: ",
		},
		{
			"[1,[]]",
			"03820180",
			"",
			"invalid header: ",
		},
		{
			// this is an unfortunate error, it'd be nice to catch it better but it's
			// very unlikely we'd ever see this in practice
			"null",
			"01f6",
			"",
			"invalid car version: 0",
		},
	}

	makeCar := func(t *testing.T, byts string) error {
		fixture, err := hex.DecodeString(byts)
		if err != nil {
			t.Fatal(err)
		}
		_, err = NewCarReader(bytes.NewReader(fixture))
		return err
	}

	t.Run("Sanity check {version:1,roots:[baeaaaa3bmjrq]}", func(t *testing.T) {
		err := makeCar(t, "1ca265726f6f747381d82a4800010000036162636776657273696f6e01")
		if err != nil {
			t.Fatal(err)
		}
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := makeCar(t, tc.hex)
			if err == nil {
				t.Fatal("expected error from bad header, didn't get one")
			}
			if tc.errStr != "" {
				if err.Error() != tc.errStr {
					t.Fatalf("bad error: %v", err)
				}
			} else {
				if !strings.HasPrefix(err.Error(), tc.errPfx) {
					t.Fatalf("bad error: %v", err)
				}
			}
		})
	}
}

func TestReadHeaderAt(t *testing.T) {
	t.Run("reads from ReaderAt", func(t *testing.T) {
		roots := []cid.Cid{blocks.NewBlock([]byte("test")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		err := WriteHeader(header, buf)
		require.NoError(t, err)

		rAt := bytes.NewReader(buf.Bytes())
		h, err := ReadHeaderAt(rAt, 4096)
		require.NoError(t, err)
		require.NotNil(t, h)
		require.Equal(t, uint64(1), h.Version)
	})

	t.Run("handles bytes.Reader as ReaderAt", func(t *testing.T) {
		roots := []cid.Cid{blocks.NewBlock([]byte("test")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		err := WriteHeader(header, buf)
		require.NoError(t, err)

		rAt := bytes.NewReader(buf.Bytes())

		h, err := ReadHeaderAt(rAt, 4096)
		require.NoError(t, err)
		require.NotNil(t, h)
		require.Equal(t, uint64(1), h.Version)
	})

	t.Run("returns header too large error", func(t *testing.T) {
		roots := []cid.Cid{blocks.NewBlock([]byte("test")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		err := WriteHeader(header, buf)
		require.NoError(t, err)

		rAt := bytes.NewReader(buf.Bytes())
		_, err = ReadHeaderAt(rAt, 1)
		require.Error(t, err)
	})

	t.Run("handles pure ReaderAt", func(t *testing.T) {
		roots := []cid.Cid{blocks.NewBlock([]byte("test")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		err := WriteHeader(header, buf)
		require.NoError(t, err)

		rAt := &readerOnlyAt{data: buf.Bytes()}
		h, err := ReadHeaderAt(rAt, 4096)
		require.NoError(t, err)
		require.NotNil(t, h)
		require.Equal(t, uint64(1), h.Version)
	})
}

func TestWriteHeader(t *testing.T) {
	t.Run("writes header successfully", func(t *testing.T) {
		roots := []cid.Cid{blocks.NewBlock([]byte("test")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		err := WriteHeader(header, buf)
		require.NoError(t, err)
		require.NotZero(t, buf.Len())
	})

	t.Run("writes header with multiple roots", func(t *testing.T) {
		roots := []cid.Cid{
			blocks.NewBlock([]byte("test1")).Cid(),
			blocks.NewBlock([]byte("test2")).Cid(),
		}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		err := WriteHeader(header, buf)
		require.NoError(t, err)
		require.NotZero(t, buf.Len())
	})

	t.Run("handles write error", func(t *testing.T) {
		roots := []cid.Cid{blocks.NewBlock([]byte("test")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		err := WriteHeader(header, failingWriter{})
		require.Error(t, err)
		require.Equal(t, "write failed", err.Error())
	})
}

func TestHeaderSize(t *testing.T) {
	t.Run("calculates size for single root header", func(t *testing.T) {
		roots := []cid.Cid{blocks.NewBlock([]byte("test")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		size, err := HeaderSize(header)
		require.NoError(t, err)
		require.NotZero(t, size)
	})

	t.Run("calculates size for multiple roots header", func(t *testing.T) {
		roots := []cid.Cid{
			blocks.NewBlock([]byte("test1")).Cid(),
			blocks.NewBlock([]byte("test2")).Cid(),
			blocks.NewBlock([]byte("test3")).Cid(),
		}
		header := &CarHeader{Roots: roots, Version: 1}

		size, err := HeaderSize(header)
		require.NoError(t, err)
		require.NotZero(t, size)
	})

	t.Run("calculates size for empty roots header", func(t *testing.T) {
		header := &CarHeader{Roots: []cid.Cid{}, Version: 1}

		size, err := HeaderSize(header)
		require.NoError(t, err)
		require.NotZero(t, size)
	})
}

func TestWriteBlock(t *testing.T) {
	t.Run("writes block successfully", func(t *testing.T) {
		c := blocks.NewBlock([]byte("test")).Cid()
		data := []byte("test data")

		buf := new(bytes.Buffer)
		err := WriteBlock(buf, c, data)
		require.NoError(t, err)
		require.NotZero(t, buf.Len())
	})

	t.Run("writes empty block", func(t *testing.T) {
		c := blocks.NewBlock([]byte("")).Cid()
		data := []byte{}

		buf := new(bytes.Buffer)
		err := WriteBlock(buf, c, data)
		require.NoError(t, err)
	})

	t.Run("writes large block", func(t *testing.T) {
		c := blocks.NewBlock([]byte("test")).Cid()
		data := make([]byte, 1024*1024) // 1MB

		buf := new(bytes.Buffer)
		err := WriteBlock(buf, c, data)
		require.NoError(t, err)
		require.GreaterOrEqual(t, buf.Len(), len(data))
	})

	t.Run("handles length write error", func(t *testing.T) {
		c := blocks.NewBlock([]byte("test")).Cid()
		data := []byte("test data")

		err := WriteBlock(failingWriter{}, c, data)
		require.Error(t, err)
		require.Contains(t, err.Error(), "write length")
	})

	t.Run("handles CID write error", func(t *testing.T) {
		fw := &failOnNthWriter{Nth: 2} // Fail on second write (after length)
		c := blocks.NewBlock([]byte("test")).Cid()
		data := []byte("test data")

		err := WriteBlock(fw, c, data)
		require.Error(t, err)
		require.Contains(t, err.Error(), "write CID")
	})

	t.Run("handles data write error", func(t *testing.T) {
		fw := &failOnNthWriter{Nth: 3} // Fail on third write (after length and CID)
		c := blocks.NewBlock([]byte("test")).Cid()
		data := []byte("test data")

		err := WriteBlock(fw, c, data)
		require.Error(t, err)
		require.Contains(t, err.Error(), "write block data")
	})
}

// failOnNthWriter fails on the Nth write call
type failOnNthWriter struct {
	calls int
	Nth   int
}

func (f *failOnNthWriter) Write(p []byte) (n int, err error) {
	f.calls++
	if f.calls == f.Nth {
		return 0, errors.New("write failed")
	}
	return len(p), nil
}

// readerOnlyAt implements only io.ReaderAt (not io.Reader)
type readerOnlyAt struct {
	data     []byte
	position int
}

func (r *readerOnlyAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n = copy(p, r.data[off:])
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func TestNewCarReaderWithZeroLengthSectionAsEOF(t *testing.T) {
	t.Run("creates reader successfully", func(t *testing.T) {
		roots := []cid.Cid{blocks.NewBlock([]byte("test")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		err := WriteHeader(header, buf)
		require.NoError(t, err)

		cr, err := NewCarReaderWithZeroLengthSectionAsEOF(buf)
		require.NoError(t, err)
		require.NotNil(t, cr)
		require.True(t, cr.zeroLenAsEOF)
	})

	t.Run("handles invalid version", func(t *testing.T) {
		roots := []cid.Cid{blocks.NewBlock([]byte("test")).Cid()}
		header := &CarHeader{Roots: roots, Version: 2}

		buf := new(bytes.Buffer)
		err := WriteHeader(header, buf)
		require.NoError(t, err)

		_, err = NewCarReaderWithZeroLengthSectionAsEOF(buf)
		require.Error(t, err)
	})

	t.Run("handles empty roots", func(t *testing.T) {
		header := &CarHeader{Roots: []cid.Cid{}, Version: 1}

		buf := new(bytes.Buffer)
		err := WriteHeader(header, buf)
		require.NoError(t, err)

		_, err = NewCarReaderWithZeroLengthSectionAsEOF(buf)
		require.Error(t, err)
	})
}

func TestLoadCar_FlushOnEOF(t *testing.T) {
	t.Run("flushes batch on EOF with less than 1000 blocks", func(t *testing.T) {
		mockBatchStore := &mockBatchStoreStore{
			blocks: make(map[string]blocks.Block),
		}

		// Create a CAR with a few blocks (less than 1000)
		roots := []cid.Cid{blocks.NewBlock([]byte("root")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		require.NoError(t, WriteHeader(header, buf))

		// Add 3 blocks
		for i := 0; i < 3; i++ {
			block := blocks.NewBlock([]byte{byte(i)})
			require.NoError(t, WriteBlock(buf, block.Cid(), block.RawData()))
		}

		h, err := LoadCar(mockBatchStore, buf)
		require.NoError(t, err)
		require.Equal(t, uint64(1), h.Version)
		require.Len(t, mockBatchStore.blocks, 3) // Should flush on EOF
	})
}

func TestLoadCar(t *testing.T) {
	t.Run("loads car with batch store", func(t *testing.T) {
		mockBatchStore := &mockBatchStoreStore{
			blocks: make(map[string]blocks.Block),
		}

		// Create a simple CAR with one block
		roots := []cid.Cid{blocks.NewBlock([]byte("root")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		require.NoError(t, WriteHeader(header, buf))

		block := blocks.NewBlock([]byte("test"))
		require.NoError(t, WriteBlock(buf, block.Cid(), block.RawData()))

		h, err := LoadCar(mockBatchStore, buf)
		require.NoError(t, err)
		require.Equal(t, uint64(1), h.Version)
	})

	t.Run("loads car with regular store", func(t *testing.T) {
		mockRegularStore := &mockRegularStoreStore{
			blocks: make(map[string]blocks.Block),
		}

		// Create a simple CAR with one block
		roots := []cid.Cid{blocks.NewBlock([]byte("root")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		require.NoError(t, WriteHeader(header, buf))

		block := blocks.NewBlock([]byte("test"))
		require.NoError(t, WriteBlock(buf, block.Cid(), block.RawData()))

		h, err := LoadCar(mockRegularStore, buf)
		require.NoError(t, err)
		require.Equal(t, uint64(1), h.Version)
	})

	t.Run("handles invalid header", func(t *testing.T) {
		mockStore := &mockBatchStoreStore{
			blocks: make(map[string]blocks.Block),
		}

		buf := new(bytes.Buffer)
		buf.Write([]byte("invalid header"))

		_, err := LoadCar(mockStore, buf)
		require.Error(t, err)
	})

	t.Run("handles PutMany error in fast path", func(t *testing.T) {
		mockStore := &mockBatchStoreStore{
			blocks:     make(map[string]blocks.Block),
			putManyErr: errors.New("put many failed"),
		}

		// Create a simple CAR with one block
		roots := []cid.Cid{blocks.NewBlock([]byte("root")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		require.NoError(t, WriteHeader(header, buf))

		block := blocks.NewBlock([]byte("test"))
		require.NoError(t, WriteBlock(buf, block.Cid(), block.RawData()))

		_, err := LoadCar(mockStore, buf)
		require.Error(t, err)
		require.Equal(t, "put many failed", err.Error())
	})

	t.Run("handles Put error in slow path", func(t *testing.T) {
		mockStore := &mockRegularStoreStore{
			blocks: make(map[string]blocks.Block),
			putErr: errors.New("put failed"),
		}

		// Create a simple CAR with one block
		roots := []cid.Cid{blocks.NewBlock([]byte("root")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		require.NoError(t, WriteHeader(header, buf))

		block := blocks.NewBlock([]byte("test"))
		require.NoError(t, WriteBlock(buf, block.Cid(), block.RawData()))

		_, err := LoadCar(mockStore, buf)
		require.Error(t, err)
		require.Equal(t, "put failed", err.Error())
	})

	t.Run("loads car with multiple blocks using fast path", func(t *testing.T) {
		mockStore := &mockBatchStoreStore{
			blocks: make(map[string]blocks.Block),
		}

		// Create a CAR with multiple blocks to test buffering
		roots := []cid.Cid{blocks.NewBlock([]byte("root")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		require.NoError(t, WriteHeader(header, buf))

		// Add multiple blocks
		for i := 0; i < 10; i++ {
			block := blocks.NewBlock([]byte{byte(i)})
			require.NoError(t, WriteBlock(buf, block.Cid(), block.RawData()))
		}

		h, err := LoadCar(mockStore, buf)
		require.NoError(t, err)
		require.Equal(t, uint64(1), h.Version)
		require.Len(t, mockStore.blocks, 10)
	})
}

func TestLdWrite(t *testing.T) {
	t.Run("writes data successfully", func(t *testing.T) {
		data := [][]byte{[]byte("part1"), []byte("part2")}
		buf := new(bytes.Buffer)

		err := LdWrite(buf, data...)
		require.NoError(t, err)
		require.NotZero(t, buf.Len())
	})

	t.Run("handles prefix write error", func(t *testing.T) {
		data := [][]byte{[]byte("part1"), []byte("part2")}
		fw := &failOnNthWriter{Nth: 1}

		err := LdWrite(fw, data...)
		require.Error(t, err)
		require.Equal(t, "write failed", err.Error())
	})

	t.Run("handles data write error", func(t *testing.T) {
		data := [][]byte{[]byte("part1"), []byte("part2")}
		fw := &failOnNthWriter{Nth: 2}

		err := LdWrite(fw, data...)
		require.Error(t, err)
		require.Equal(t, "write failed", err.Error())
	})
}

func TestReadNode(t *testing.T) {
	t.Run("reads node successfully", func(t *testing.T) {
		blk := blocks.NewBlock([]byte("test data"))
		c := blk.Cid()
		data := blk.RawData()

		buf := new(bytes.Buffer)
		require.NoError(t, WriteBlock(buf, c, data))

		nodeCid, nodeData, err := ReadNode(buf, false, 4096)
		require.NoError(t, err)
		require.Equal(t, c, nodeCid)
		require.Equal(t, data, nodeData)
	})

	t.Run("handles invalid CID data", func(t *testing.T) {
		// Create a node with valid length encoding but invalid CID payload
		buf := new(bytes.Buffer)
		// Write a length prefix
		lengthBytes := make([]byte, 8)
		_ = varint.UvarintSize(10)
		varint.PutUvarint(lengthBytes[:1], 10)
		buf.Write(lengthBytes[:1])
		// Write 10 bytes that are not a valid CID
		buf.Write([]byte("notacid!!!"))

		_, _, err := ReadNode(buf, false, 4096)
		require.Error(t, err)
	})
}

func TestReadNodeHeader(t *testing.T) {
	t.Run("reads node header successfully", func(t *testing.T) {
		blk := blocks.NewBlock([]byte("test data"))
		c := blk.Cid()
		data := blk.RawData()

		buf := new(bytes.Buffer)
		require.NoError(t, WriteBlock(buf, c, data))

		rc, remainingSize, err := ReadNodeHeader(buf, false, 4096)
		require.NoError(t, err)
		require.Equal(t, c, rc)
		require.Equal(t, uint64(len(data)), remainingSize)
	})

	t.Run("handles zero length as EOF", func(t *testing.T) {
		buf := new(bytes.Buffer)
		buf.Write([]byte{0})

		_, _, err := ReadNodeHeader(buf, true, 4096)
		require.Equal(t, io.EOF, err)
	})

	t.Run("handles section too large", func(t *testing.T) {
		buf := new(bytes.Buffer)
		buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})

		_, _, err := ReadNodeHeader(buf, false, 4096)
		require.Error(t, err)
	})

	t.Run("handles zero length with zeroAsEOF false", func(t *testing.T) {
		// Write invalid CID data (0 bytes length)
		buf := new(bytes.Buffer)
		buf.Write([]byte{0})

		_, _, err := ReadNodeHeader(buf, false, 4096)
		// Should error because 0-byte CID is invalid
		require.Error(t, err)
	})

	t.Run("handles unexpected EOF with non-zero bytes read", func(t *testing.T) {
		// Start a varint but don't finish it - we'll get bytes read but EOF
		buf := new(bytes.Buffer)
		// Write 0x80 which indicates the next byte continues the varint
		buf.Write([]byte{0x80})

		_, _, err := ReadNodeHeader(buf, false, 4096)
		require.Equal(t, io.ErrUnexpectedEOF, err)
	})
}

// Mock stores for testing

type mockBatchStoreStore struct {
	blocks     map[string]blocks.Block
	putErr     error
	putManyErr error
}

func (m *mockBatchStoreStore) PutMany(_ context.Context, blocks []blocks.Block) error {
	if m.putManyErr != nil {
		return m.putManyErr
	}
	for _, blk := range blocks {
		m.blocks[blk.Cid().String()] = blk
	}
	return nil
}

func (m *mockBatchStoreStore) Put(_ context.Context, blk blocks.Block) error {
	if m.putErr != nil {
		return m.putErr
	}
	m.blocks[blk.Cid().String()] = blk
	return nil
}

type mockRegularStoreStore struct {
	blocks map[string]blocks.Block
	putErr error
}

func (m *mockRegularStoreStore) Put(_ context.Context, blk blocks.Block) error {
	if m.putErr != nil {
		return m.putErr
	}
	m.blocks[blk.Cid().String()] = blk
	return nil
}

// failingWriter is a mock writer that always fails
type failingWriter struct{}

func (failingWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("write failed")
}

func TestIntegrityMismatch(t *testing.T) {
	t.Run("Next detects CID/Data mismatch", func(t *testing.T) {
		// Create a CAR with a header and a block where the CID doesn't match the data
		roots := []cid.Cid{blocks.NewBlock([]byte("root")).Cid()}
		header := &CarHeader{Roots: roots, Version: 1}

		buf := new(bytes.Buffer)
		require.NoError(t, WriteHeader(header, buf))

		// Create a block with CID for "test" but data for "different"
		originalBlock := blocks.NewBlock([]byte("test"))
		c := originalBlock.Cid()
		differentData := []byte("different")

		require.NoError(t, WriteBlock(buf, c, differentData))

		cr, err := NewCarReader(buf)
		require.NoError(t, err)

		// Next should detect the CID/Data mismatch
		_, err = cr.Next()
		require.Error(t, err)
		require.Contains(t, err.Error(), "mismatch in content integrity")
	})
}

func TestContainsRoot(t *testing.T) {
	cid1 := blocks.NewBlock([]byte("fish")).Cid()
	cid2 := blocks.NewBlock([]byte("lobster")).Cid()
	cid3 := blocks.NewBlock([]byte("shark")).Cid()

	t.Run("nil roots returns false", func(t *testing.T) {
		header := &CarHeader{Roots: nil, Version: 1}
		require.False(t, header.containsRoot(cid1))
	})

	t.Run("empty roots returns false", func(t *testing.T) {
		header := &CarHeader{Roots: []cid.Cid{}, Version: 1}
		require.False(t, header.containsRoot(cid1))
	})

	t.Run("single root matches", func(t *testing.T) {
		header := &CarHeader{Roots: []cid.Cid{cid1}, Version: 1}
		require.True(t, header.containsRoot(cid1))
	})

	t.Run("single root doesn't match different CID", func(t *testing.T) {
		header := &CarHeader{Roots: []cid.Cid{cid1}, Version: 1}
		require.False(t, header.containsRoot(cid2))
	})

	t.Run("multiple roots matches first", func(t *testing.T) {
		header := &CarHeader{Roots: []cid.Cid{cid1, cid2, cid3}, Version: 1}
		require.True(t, header.containsRoot(cid1))
	})

	t.Run("multiple roots matches middle", func(t *testing.T) {
		header := &CarHeader{Roots: []cid.Cid{cid1, cid2, cid3}, Version: 1}
		require.True(t, header.containsRoot(cid2))
	})

	t.Run("multiple roots matches last", func(t *testing.T) {
		header := &CarHeader{Roots: []cid.Cid{cid1, cid2, cid3}, Version: 1}
		require.True(t, header.containsRoot(cid3))
	})

	t.Run("multiple roots doesn't match different CID", func(t *testing.T) {
		cid4 := blocks.NewBlock([]byte("whale")).Cid()
		header := &CarHeader{Roots: []cid.Cid{cid1, cid2, cid3}, Version: 1}
		require.False(t, header.containsRoot(cid4))
	})
}

func TestCarHeaderMatches(t *testing.T) {
	oneCid := blocks.NewBlock([]byte("fish")).Cid()
	anotherCid := blocks.NewBlock([]byte("lobster")).Cid()
	tests := []struct {
		name  string
		one   CarHeader
		other CarHeader
		want  bool
	}{
		{
			"SameVersionNilRootsIsMatching",
			CarHeader{nil, 1},
			CarHeader{nil, 1},
			true,
		},
		{
			"SameVersionEmptyRootsIsMatching",
			CarHeader{[]cid.Cid{}, 1},
			CarHeader{[]cid.Cid{}, 1},
			true,
		},
		{
			"SameVersionSingleRootMatches",
			CarHeader{[]cid.Cid{oneCid}, 1},
			CarHeader{[]cid.Cid{oneCid}, 1},
			true,
		},
		{
			"SameVersionNonEmptySameRootsIsMatching",
			CarHeader{[]cid.Cid{oneCid}, 1},
			CarHeader{[]cid.Cid{oneCid}, 1},
			true,
		},
		{
			"SameVersionNonEmptySameRootsInDifferentOrderIsMatching",
			CarHeader{[]cid.Cid{oneCid, anotherCid}, 1},
			CarHeader{[]cid.Cid{anotherCid, oneCid}, 1},
			true,
		},
		{
			"SameVersionSingleRootFastPathMatches",
			CarHeader{[]cid.Cid{oneCid}, 1},
			CarHeader{[]cid.Cid{oneCid}, 1},
			true,
		},
		{
			"SameVersionSingleRootFastPathNoMatch",
			CarHeader{[]cid.Cid{oneCid}, 1},
			CarHeader{[]cid.Cid{anotherCid}, 1},
			false,
		},
		{
			"SameVersionDifferentRootsIsNotMatching",
			CarHeader{[]cid.Cid{oneCid}, 1},
			CarHeader{[]cid.Cid{anotherCid}, 1},
			false,
		},
		{
			"DifferentVersionDifferentRootsIsNotMatching",
			CarHeader{[]cid.Cid{oneCid}, 0},
			CarHeader{[]cid.Cid{anotherCid}, 1},
			false,
		},
		{
			"MismatchingVersionIsNotMatching",
			CarHeader{nil, 0},
			CarHeader{nil, 1},
			false,
		},
		{
			"ZeroValueHeadersAreMatching",
			CarHeader{},
			CarHeader{},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.one.Matches(tt.other)
			require.Equal(t, tt.want, got, "Matches() = %v, want %v", got, tt.want)
		})
	}
}
