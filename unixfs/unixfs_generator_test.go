package unixfs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/exchange/offline"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testingutil "go.lumeweb.com/ipfs-content/internal/testing"
)

// setupNodeGeneratorTest creates in-memory IPFS components for testing
func setupNodeGeneratorTest(t *testing.T) (UnixFSNodeGenerator, context.Context, func()) {
	ctx := context.Background()

	// Create in-memory implementations
	dstore := dssync.MutexWrap(ds.NewMapDatastore())
	bstore := blockstore.NewBlockstore(dstore)
	dagService := merkledag.NewDAGService(blockservice.New(bstore, offline.Exchange(bstore)))

	// Create the generator with real components
	generator := NewUnixFSNodeGenerator(
		WithUnixFSNodeDAGService(dagService),
		WithUnixFSNodeBlockstore(bstore),
	)

	// Return cleanup function
	cleanup := func() {
		_ = dstore.Close()
	}

	return generator, ctx, cleanup
}

// TestNewUnixFSNodeGenerator tests the constructor
func TestNewUnixFSNodeGenerator(t *testing.T) {
	t.Parallel()
	generator, _, cleanup := setupNodeGeneratorTest(t)
	defer cleanup()

	require.NotNil(t, generator)

	// Verify returns DAG service and blockstore
	assert.NotNil(t, generator.GetDAGService())
	assert.NotNil(t, generator.GetBlockstore())
}

// TestIPFSUnixFSNodeGenerator_CreateDirectory tests directory creation
func TestIPFSUnixFSNodeGenerator_CreateDirectory(t *testing.T) {
	t.Parallel()
	generator, _, cleanup := setupNodeGeneratorTest(t)
	defer cleanup()

	dir, err := generator.CreateDirectory()

	assert.NoError(t, err)
	assert.NotNil(t, dir)

	// Verify it's a UnixFS directory by getting its node
	node, err := dir.GetNode()
	assert.NoError(t, err)
	assert.NotNil(t, node)
	assert.NotEqual(t, cid.Undef, node.Cid())
}

// TestIPFSUnixFSNodeGenerator_CreateDirectoryWithLinks tests directory with child links
func TestIPFSUnixFSNodeGenerator_CreateDirectoryWithLinks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		children    []DirectoryChild
		expectError bool
	}{
		{
			name:        "empty directory",
			children:    []DirectoryChild{},
			expectError: false,
		},
		{
			name: "single child",
			children: []DirectoryChild{
				{Name: "file1.txt", CID: testingutil.GenerateCIDFromString(t, "test", testingutil.DagCBORCIDOptions())},
			},
			expectError: false,
		},
		{
			name: "multiple children",
			children: []DirectoryChild{
				{Name: "file1.txt", CID: testingutil.GenerateCIDFromString(t, "test", testingutil.DagCBORCIDOptions())},
				{Name: "file2.txt", CID: testingutil.GenerateCIDFromString(t, "test", testingutil.DagCBORCIDOptions())},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator, ctx, cleanup := setupNodeGeneratorTest(t)
			defer cleanup()

			node, err := generator.CreateDirectoryWithLinks(ctx, tt.children)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, node)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, node)
				assert.NotEqual(t, cid.Undef, node.Cid())
			}
		})
	}
}

// TestIPFSUnixFSNodeGenerator_CreateDirectoryWithLinks_ContextCancellation tests context cancellation
func TestIPFSUnixFSNodeGenerator_CreateDirectoryWithLinks_ContextCancellation(t *testing.T) {
	generator, ctx, cleanup := setupNodeGeneratorTest(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(ctx)
	cancel() // Cancel before calling

	children := []DirectoryChild{
		{Name: "file1.txt", CID: testingutil.GenerateCIDFromString(t, "test", testingutil.DagCBORCIDOptions())},
	}

	_, err := generator.CreateDirectoryWithLinks(ctx, children)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

// TestIPFSUnixFSNodeGenerator_CreateNode tests basic node creation
func TestIPFSUnixFSNodeGenerator_CreateNode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		content      []byte
		expectError  bool
		expectedSize int64
	}{
		{
			name:         "small content",
			content:      []byte("hello world"),
			expectError:  false,
			expectedSize: 11,
		},
		{
			name:         "empty content",
			content:      []byte(""),
			expectError:  false,
			expectedSize: 0,
		},
		{
			name:         "medium content",
			content:      bytes.Repeat([]byte("test content "), 100),
			expectError:  false,
			expectedSize: 1300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator, ctx, cleanup := setupNodeGeneratorTest(t)
			defer cleanup()

			reader := io.NopCloser(bytes.NewReader(tt.content))

			rsc, err := newReadSeekCloser(reader)
			if err != nil {
				t.Fatalf("failed to create readSeekCloser: %v", err)
			}

			node, err := generator.CreateNode(ctx, rsc)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, node)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, node)
				assert.NotEqual(t, cid.Undef, node.Cid())

				// Verify node size
				nodeSize, err := node.Size()
				assert.NoError(t, err)
				assert.GreaterOrEqual(t, nodeSize, uint64(tt.expectedSize))
			}
		})
	}
}

// TestIPFSUnixFSNodeGenerator_CreateUnixFSNode tests node creation with custom parameters
func TestIPFSUnixFSNodeGenerator_CreateUnixFSNode(t *testing.T) {
	tests := []struct {
		name         string
		content      []byte
		maxLinks     int
		chunkSize    int64
		expectError  bool
		expectedSize int64
	}{
		{
			name:         "custom maxlinks and chunksize",
			content:      bytes.Repeat([]byte("test"), 1000),
			maxLinks:     100,
			chunkSize:    512,
			expectError:  false,
			expectedSize: 4000,
		},
		{
			name:         "small chunk size",
			content:      []byte("small content"),
			maxLinks:     10,
			chunkSize:    64,
			expectError:  false,
			expectedSize: 13,
		},
		{
			name:         "large maxlinks",
			content:      bytes.Repeat([]byte("x"), 10000),
			maxLinks:     1000,
			chunkSize:    1024,
			expectError:  false,
			expectedSize: 10000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator, ctx, cleanup := setupNodeGeneratorTest(t)
			defer cleanup()

			reader := io.NopCloser(bytes.NewReader(tt.content))

			rsc, err := newReadSeekCloser(reader)
			if err != nil {
				t.Fatalf("failed to create readSeekCloser: %v", err)
			}

			node, err := generator.CreateUnixFSNode(ctx, rsc, tt.maxLinks, tt.chunkSize)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, node)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, node)
				assert.NotEqual(t, cid.Undef, node.Cid())

				// Verify node size
				nodeSize, err := node.Size()
				assert.NoError(t, err)
				assert.GreaterOrEqual(t, nodeSize, uint64(tt.expectedSize))
			}
		})
	}
}

// TestIPFSUnixFSNodeGenerator_CreateDAGFromReader tests the core DAG creation logic
func TestIPFSUnixFSNodeGenerator_CreateDAGFromReader(t *testing.T) {
	tests := []struct {
		name         string
		content      []byte
		maxLinks     int
		chunkSize    int64
		rawLeaves    bool
		expectError  bool
		expectedSize int64
	}{
		{
			name:         "raw leaves false",
			content:      []byte("test content"),
			maxLinks:     10,
			chunkSize:    256,
			rawLeaves:    false,
			expectError:  false,
			expectedSize: 12,
		},
		{
			name:         "raw leaves true",
			content:      []byte("test content"),
			maxLinks:     10,
			chunkSize:    256,
			rawLeaves:    true,
			expectError:  false,
			expectedSize: 12,
		},
		{
			name:         "large content with raw leaves",
			content:      bytes.Repeat([]byte("large content chunk"), 1000),
			maxLinks:     100,
			chunkSize:    512,
			rawLeaves:    true,
			expectError:  false,
			expectedSize: 19000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator, ctx, cleanup := setupNodeGeneratorTest(t)
			defer cleanup()

			reader := bytes.NewReader(tt.content)

			node, err := generator.CreateDAGFromReader(ctx, reader, tt.maxLinks, tt.chunkSize, tt.rawLeaves)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, node)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, node)
				assert.NotEqual(t, cid.Undef, node.Cid())

				// Verify node size
				nodeSize, err := node.Size()
				assert.NoError(t, err)
				assert.GreaterOrEqual(t, nodeSize, uint64(tt.expectedSize))
			}
		})
	}
}

// TestIPFSUnixFSNodeGenerator_CreateDAGFromReader_NilReader tests nil reader
func TestIPFSUnixFSNodeGenerator_CreateDAGFromReader_NilReader(t *testing.T) {
	generator, ctx, cleanup := setupNodeGeneratorTest(t)
	defer cleanup()

	_, err := generator.CreateDAGFromReader(ctx, nil, 10, 256, false)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

// TestIPFSUnixFSNodeGenerator_ContextCancellation tests context cancellation
func TestIPFSUnixFSNodeGenerator_ContextCancellation(t *testing.T) {
	tests := []struct {
		name         string
		cancelBefore bool
		method       string
	}{
		{
			name:         "CreateNode cancelled before",
			cancelBefore: true,
			method:       "CreateNode",
		},
		{
			name:         "CreateUnixFSNode cancelled before",
			cancelBefore: true,
			method:       "CreateUnixFSNode",
		},
		{
			name:         "CreateDAGFromReader cancelled before",
			cancelBefore: true,
			method:       "CreateDAGFromReader",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator, _, cleanup := setupNodeGeneratorTest(t)
			defer cleanup()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelBefore {
				cancel()
			}

			content := []byte("test content")
			reader := io.NopCloser(bytes.NewReader(content))

	var err error
			switch tt.method {
			case "CreateNode":
				rsc, rerr := newReadSeekCloser(reader)
				if rerr != nil {
					err = rerr
				} else {
					_, err = generator.CreateNode(ctx, rsc)
				}
			case "CreateUnixFSNode":
				rsc, rerr := newReadSeekCloser(reader)
				if rerr != nil {
					err = rerr
				} else {
					_, err = generator.CreateUnixFSNode(ctx, rsc, 10, 256)
				}
			case "CreateDAGFromReader":
				_, err = generator.CreateDAGFromReader(ctx, bytes.NewReader(content), 10, 256, false)
			}

			if tt.cancelBefore {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, context.Canceled))
			}
		})
	}
}

// TestIPFSUnixFSNodeGenerator_VariousContentSizes tests different content sizes
func TestIPFSUnixFSNodeGenerator_VariousContentSizes(t *testing.T) {
	t.Parallel()
	sizes := []struct {
		name string
		size int
	}{
		{"empty", 0},
		{"single", 1},
		{"small", 256},
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"1MB", 1024 * 1024},
	}

	for _, sizeTest := range sizes {
		t.Run(sizeTest.name, func(t *testing.T) {
			generator, ctx, cleanup := setupNodeGeneratorTest(t)
			defer cleanup()

			content := make([]byte, sizeTest.size)
			for i := range content {
				content[i] = byte(i % 256)
			}

			reader := io.NopCloser(bytes.NewReader(content))

			rsc, rerr := newReadSeekCloser(reader)
			if rerr != nil {
				t.Fatalf("failed to create readSeekCloser: %v", rerr)
			}

			node, err := generator.CreateNode(ctx, rsc)

			assert.NoError(t, err)
			assert.NotNil(t, node)
			assert.NotEqual(t, cid.Undef, node.Cid())

			nodeSize, err := node.Size()
			assert.NoError(t, err)
			assert.GreaterOrEqual(t, nodeSize, uint64(sizeTest.size))
		})
	}
}

// TestIPFSUnixFSNodeGenerator_PerformanceEdgeCases tests edge cases
func TestIPFSUnixFSNodeGenerator_PerformanceEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		maxLinks   int
		chunkSize  int64
		expectError bool
	}{
		{
			name:       "zero maxlinks",
			maxLinks:   0,
			chunkSize:  256,
			expectError: false,
		},
		{
			name:       "very large maxlinks",
			maxLinks:   1000000,
			chunkSize:  256,
			expectError: false,
		},
		{
			name:       "very small chunk size",
			maxLinks:   10,
			chunkSize:  1,
			expectError: false,
		},
		{
			name:       "zero chunk size",
			maxLinks:   10,
			chunkSize:  0,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator, ctx, cleanup := setupNodeGeneratorTest(t)
			defer cleanup()

			content := []byte("test content")
			reader := io.NopCloser(bytes.NewReader(content))

			rsc, rerr := newReadSeekCloser(reader)
			if rerr != nil {
				t.Fatalf("failed to create readSeekCloser: %v", rerr)
			}

			node, err := generator.CreateUnixFSNode(ctx, rsc, tt.maxLinks, tt.chunkSize)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, node)
				assert.NotEqual(t, cid.Undef, node.Cid())
			}
		})
	}
}

// Helper functions


// Helper types

type readSeekCloser struct {
	io.Reader
	io.Seeker
}

func (r *readSeekCloser) Close() error {
	return nil
}

func newReadSeekCloser(r io.Reader) (io.ReadSeekCloser, error) {
	if rs, ok := r.(io.ReadSeekCloser); ok {
		return rs, nil
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	br := bytes.NewReader(data)
	return &readSeekCloser{Reader: br, Seeker: br}, nil
}

// failingErrorSeeker implements io.ReadSeekCloser but fails on operations
type failingErrorSeeker struct {
	pos     int64
	readErr error
	seekErr error
}

func (r *failingErrorSeeker) Read(p []byte) (n int, err error) {
	if r.readErr != nil {
		return 0, r.readErr
	}
	return 0, io.EOF
}

func (r *failingErrorSeeker) Seek(offset int64, whence int) (int64, error) {
	if r.seekErr != nil {
		return 0, r.seekErr
	}
	return 0, errors.New("seek not supported")
}

func (r *failingErrorSeeker) Close() error {
	return nil
}
