package blockstore

import (
	"context"
	"testing"

	"github.com/ipfs/go-block-format"
	"github.com/stretchr/testify/require"
)

func TestNewInMemoryBlockstore(t *testing.T) {
	t.Run("creates non-nil blockstore", func(t *testing.T) {
		bs := NewInMemoryBlockstore()
		require.NotNil(t, bs)
	})

	t.Run("creates multiple independent instances", func(t *testing.T) {
		bs1 := NewInMemoryBlockstore()
		bs2 := NewInMemoryBlockstore()
		require.NotNil(t, bs1)
		require.NotNil(t, bs2)
	})
}

func TestInMemoryBlockstore_AddBlocksFromFile(t *testing.T) {
	ctx := context.Background()

	t.Run("adds single block successfully", func(t *testing.T) {
		bs := NewInMemoryBlockstore()

		data := []byte("test data")
		block := blocks.NewBlock(data)

		blockList := []blocks.Block{block}
		err := bs.AddBlocksFromFile(blockList, ctx)
		require.NoError(t, err)
	})

	t.Run("adds multiple blocks successfully", func(t *testing.T) {
		bs := NewInMemoryBlockstore()

		data1 := []byte("test data 1")
		block1 := blocks.NewBlock(data1)

		data2 := []byte("test data 2")
		block2 := blocks.NewBlock(data2)

		blockList := []blocks.Block{block1, block2}
		err := bs.AddBlocksFromFile(blockList, ctx)
		require.NoError(t, err)
	})
}

func TestInMemoryBlockstore_BasicOperations(t *testing.T) {
	t.Run("put and get block", func(t *testing.T) {
		bs := NewInMemoryBlockstore()

		data := []byte("test data")
		block := blocks.NewBlock(data)

		c := block.Cid()

		err := bs.Put(context.Background(), block)
		require.NoError(t, err)

		retrieved, err := bs.Get(context.Background(), c)
		require.NoError(t, err)
		require.Equal(t, data, retrieved.RawData())
	})
}
