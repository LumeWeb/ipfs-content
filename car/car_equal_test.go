package car

import (
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"

	testingutil "go.lumeweb.com/ipfs-content/internal/testing"
)

func TestTreeSummary_Equal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		left     *TreeSummary
		right    *TreeSummary
		expected bool
	}{
		{
			name:     "both nil",
			left:     nil,
			right:    nil,
			expected: true,
		},
		{
			name:     "one nil",
			left:     nil,
			right:    &TreeSummary{},
			expected: false,
		},
		{
			name:     "different RootCID",
			left:     &TreeSummary{RootCID: testingutil.GenerateDistinctCID(t, 1)},
			right:    &TreeSummary{RootCID: testingutil.GenerateDistinctCID(t, 2)},
			expected: false,
		},
		{
			name:     "different TotalSize",
			left:     &TreeSummary{TotalSize: 100},
			right:    &TreeSummary{TotalSize: 200},
			expected: false,
		},
		{
			name:     "different CARSize",
			left:     &TreeSummary{CARSize: 100},
			right:    &TreeSummary{CARSize: 200},
			expected: false,
		},
		{
			name: "different block count",
			left: &TreeSummary{
				BlockOrder: []cid.Cid{testingutil.GenerateDistinctCID(t, 1)},
				BlockSizes: []uint64{100},
			},
			right: &TreeSummary{
				BlockOrder: []cid.Cid{testingutil.GenerateDistinctCID(t, 1), testingutil.GenerateDistinctCID(t, 2)},
				BlockSizes: []uint64{100, 200},
			},
			expected: false,
		},
		{
			name: "different block CIDs",
			left: &TreeSummary{
				BlockOrder: []cid.Cid{testingutil.GenerateDistinctCID(t, 1)},
				BlockSizes: []uint64{100},
			},
			right: &TreeSummary{
				BlockOrder: []cid.Cid{testingutil.GenerateDistinctCID(t, 2)},
				BlockSizes: []uint64{100},
			},
			expected: false,
		},
		{
			name: "different block sizes",
			left: &TreeSummary{
				BlockOrder: []cid.Cid{testingutil.GenerateDistinctCID(t, 1)},
				BlockSizes: []uint64{100},
			},
			right: &TreeSummary{
				BlockOrder: []cid.Cid{testingutil.GenerateDistinctCID(t, 1)},
				BlockSizes: []uint64{200},
			},
			expected: false,
		},
		{
			name: "same blocks, different order (should be equal per CAR spec)",
			left: &TreeSummary{
				BlockOrder: []cid.Cid{testingutil.GenerateDistinctCID(t, 1), testingutil.GenerateDistinctCID(t, 2)},
				BlockSizes: []uint64{100, 200},
			},
			right: &TreeSummary{
				BlockOrder: []cid.Cid{testingutil.GenerateDistinctCID(t, 2), testingutil.GenerateDistinctCID(t, 1)},
				BlockSizes: []uint64{200, 100},
			},
			expected: true,
		},
		{
			name: "different TreeEntries count",
			left: &TreeSummary{
				TreeEntries: map[string]*TreeEntry{"file1.txt": {}},
			},
			right: &TreeSummary{
				TreeEntries: map[string]*TreeEntry{
					"file1.txt": {},
					"file2.txt": {},
				},
			},
			expected: false,
		},
		{
			name: "different TreeEntries keys",
			left: &TreeSummary{
				TreeEntries: map[string]*TreeEntry{"file1.txt": {Name: "file1.txt"}},
			},
			right: &TreeSummary{
				TreeEntries: map[string]*TreeEntry{"file2.txt": {Name: "file2.txt"}},
			},
			expected: false,
		},
		{
			name: "different TreeEntries values",
			left: &TreeSummary{
				TreeEntries: map[string]*TreeEntry{
					"file1.txt": {Name: "file1.txt", IsDir: false},
				},
			},
			right: &TreeSummary{
				TreeEntries: map[string]*TreeEntry{
					"file1.txt": {Name: "file1.txt", IsDir: true},
				},
			},
			expected: false,
		},
		{
			name: "complete match",
			left: &TreeSummary{
				RootCID:   testingutil.GenerateDistinctCID(t, 1),
				TotalSize: 1000,
				CARSize:   1200,
				BlockOrder: []cid.Cid{
					testingutil.GenerateDistinctCID(t, 3),
					testingutil.GenerateDistinctCID(t, 4),
				},
				BlockSizes: []uint64{100, 200},
				TreeEntries: map[string]*TreeEntry{
					"file.txt": {
						Name:            "file.txt",
						Path:            "",
						IsDir:           false,
						CID:             testingutil.GenerateDistinctCID(t, 5),
						LogicalFileSize: 300,
					},
				},
			},
			right: &TreeSummary{
				RootCID:   testingutil.GenerateDistinctCID(t, 1),
				TotalSize: 1000,
				CARSize:   1200,
				BlockOrder: []cid.Cid{
					testingutil.GenerateDistinctCID(t, 4),
					testingutil.GenerateDistinctCID(t, 3),
				},
				BlockSizes: []uint64{200, 100},
				TreeEntries: map[string]*TreeEntry{
					"file.txt": {
						Name:            "file.txt",
						Path:            "",
						IsDir:           false,
						CID:             testingutil.GenerateDistinctCID(t, 5),
						LogicalFileSize: 300,
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.left.Equal(tt.right)
			assert.Equal(t, tt.expected, result, "Equal() result mismatch")
		})
	}
}

func TestTreeEntry_Equal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		left     *TreeEntry
		right    *TreeEntry
		expected bool
	}{
		{
			name:     "both nil",
			left:     nil,
			right:    nil,
			expected: true,
		},
		{
			name:     "one nil",
			left:     nil,
			right:    &TreeEntry{},
			expected: false,
		},
		{
			name:     "different Path",
			left:     &TreeEntry{Path: "path1"},
			right:    &TreeEntry{Path: "path2"},
			expected: false,
		},
		{
			name:     "different Name",
			left:     &TreeEntry{Name: "name1"},
			right:    &TreeEntry{Name: "name2"},
			expected: false,
		},
		{
			name:     "different IsDir",
			left:     &TreeEntry{IsDir: true},
			right:    &TreeEntry{IsDir: false},
			expected: false,
		},
		{
			name:     "different CID",
			left:     &TreeEntry{CID: testingutil.GenerateDistinctCID(t, 1)},
			right:    &TreeEntry{CID: testingutil.GenerateDistinctCID(t, 2)},
			expected: false,
		},
		{
			name:     "different ChunkSize",
			left:     &TreeEntry{ChunkSize: 100},
			right:    &TreeEntry{ChunkSize: 200},
			expected: false,
		},
		{
			name:     "different LogicalFileSize",
			left:     &TreeEntry{LogicalFileSize: 100},
			right:    &TreeEntry{LogicalFileSize: 200},
			expected: false,
		},
		{
			name:     "different Children count",
			left:     &TreeEntry{Children: []string{"child1"}},
			right:    &TreeEntry{Children: []string{"child1", "child2"}},
			expected: false,
		},
		{
			name: "different Children values (same count)",
			left: &TreeEntry{Children: []string{"child1"}},
			right: &TreeEntry{Children: []string{"child2"}},
			expected: false,
		},
		{
			name: "different Children order",
			left: &TreeEntry{Children: []string{"child1", "child2"}},
			right: &TreeEntry{Children: []string{"child2", "child1"}},
			expected: false,
		},
		{
			name: "complete match with children",
			left: &TreeEntry{
				Name:            "file.txt",
				Path:            "dir1",
				IsDir:           false,
				CID:             testingutil.GenerateDistinctCID(t, 1),
				ChunkSize:       1024,
				LogicalFileSize: 2048,
				Children:        []string{"child1", "child2"},
			},
			right: &TreeEntry{
				Name:            "file.txt",
				Path:            "dir1",
				IsDir:           false,
				CID:             testingutil.GenerateDistinctCID(t, 1),
				ChunkSize:       1024,
				LogicalFileSize: 2048,
				Children:        []string{"child1", "child2"},
			},
			expected: true,
		},
		{
			name: "complete match without children",
			left: &TreeEntry{
				Name:            "file.txt",
				Path:            "",
				IsDir:           false,
				CID:             testingutil.GenerateDistinctCID(t, 1),
				ChunkSize:       1024,
				LogicalFileSize: 2048,
				Children:        nil,
			},
			right: &TreeEntry{
				Name:            "file.txt",
				Path:            "",
				IsDir:           false,
				CID:             testingutil.GenerateDistinctCID(t, 1),
				ChunkSize:       1024,
				LogicalFileSize: 2048,
				Children:        []string{},
			},
			expected: false, // nil vs empty slice are different
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.left.Equal(tt.right)
			assert.Equal(t, tt.expected, result, "Equal() result mismatch")
		})
	}
}

