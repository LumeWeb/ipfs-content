package internal_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/lumeweb/ipfs-content/archive/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrepareReaderPreservePos tests the PrepareReaderPreservePos function
func TestPrepareReaderPreservePos(t *testing.T) {
	t.Run("preserves position and seeks to beginning", func(t *testing.T) {
		data := []byte("Hello, World!")
		buf := bytes.NewReader(data)

		// Move to position 5
		_, err := buf.Seek(5, io.SeekStart)
		require.NoError(t, err)

		// Prepare reader
		pos, err := internal.PrepareReaderPreservePos(buf)
		require.NoError(t, err)
		assert.Equal(t, int64(5), pos)

		// Verify reader is at beginning
		currentPos, err := buf.Seek(0, io.SeekCurrent)
		require.NoError(t, err)
		assert.Equal(t, int64(0), currentPos)
	})

	t.Run("handles errors when getting current position", func(t *testing.T) {
		// Create a reader that fails on Seek
		errSeeker := &errorSeeker{seekErr: errors.New("seek error")}
		_, err := internal.PrepareReaderPreservePos(errSeeker)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get reader position")
	})

	t.Run("handles errors when seeking to beginning", func(t *testing.T) {
		// Create a seeker that succeeds to get position but fails on seek to start
		errorSeeker := &brokenSeeker{
			pos: 10,
			startSeekErr: errors.New("seek to start error"),
		}
		_, err := internal.PrepareReaderPreservePos(errorSeeker)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to seek to beginning")
	})

	t.Run("handles negative position", func(t *testing.T) {
		data := []byte("Hello, World!")
		buf := bytes.NewReader(data)

		// Move to valid position
		_, err := buf.Seek(5, io.SeekStart)
		require.NoError(t, err)

		pos, err := internal.PrepareReaderPreservePos(buf)
		require.NoError(t, err)
		assert.Equal(t, int64(5), pos)
	})
}

// TestRestoreReaderPos tests the RestoreReaderPos function
func TestRestoreReaderPos(t *testing.T) {
	t.Run("restores to original position", func(t *testing.T) {
		data := []byte("Hello, World!")
		buf := bytes.NewReader(data)

		// Move to position 3
		_, err := buf.Seek(3, io.SeekStart)
		require.NoError(t, err)

		// Seek to beginning (simulating some processing)
		_, err = buf.Seek(0, io.SeekStart)
		require.NoError(t, err)

		// Restore to position 3
		err = internal.RestoreReaderPos(buf, 3)
		require.NoError(t, err)

		// Verify we're back at position 3
		currentPos, err := buf.Seek(0, io.SeekCurrent)
		require.NoError(t, err)
		assert.Equal(t, int64(3), currentPos)
	})

	t.Run("restores to end position", func(t *testing.T) {
		data := []byte("Hello, World!")
		buf := bytes.NewReader(data)

		length := int64(len(data))

		// Restore to end position
		err := internal.RestoreReaderPos(buf, length)
		require.NoError(t, err)

		// Verify we're at end
		currentPos, err := buf.Seek(0, io.SeekCurrent)
		require.NoError(t, err)
		assert.Equal(t, length, currentPos)
	})

	t.Run("handles seek errors", func(t *testing.T) {
		errorSeeker := &brokenSeeker{pos: 5, startSeekErr: errors.New("seek error")}
		err := internal.RestoreReaderPos(errorSeeker, 10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to restore original reader position")
	})

	t.Run("restores to zero position", func(t *testing.T) {
		data := []byte("Hello, World!")
		buf := bytes.NewReader(data)

		// Move to middle
		_, err := buf.Seek(5, io.SeekStart)
		require.NoError(t, err)

		// Restore to position 0
		err = internal.RestoreReaderPos(buf, 0)
		require.NoError(t, err)

		// Verify we're at beginning
		currentPos, err := buf.Seek(0, io.SeekCurrent)
		require.NoError(t, err)
		assert.Equal(t, int64(0), currentPos)
	})
}

// TestPrepareAndRestoreSequence tests the combined workflow
func TestPrepareAndRestoreSequence(t *testing.T) {
	t.Run("full workflow preserves position", func(t *testing.T) {
		data := []byte("Hello, World!")
		buf := bytes.NewReader(data)

		// Move to position 7
		_, err := buf.Seek(7, io.SeekStart)
		require.NoError(t, err)

		// Save position and prepare to read from beginning
		savedPos, err := internal.PrepareReaderPreservePos(buf)
		require.NoError(t, err)
		assert.Equal(t, int64(7), savedPos)

		// Read from beginning
		readData := make([]byte, 5)
		n, err := buf.Read(readData)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("Hello"), readData)

		// Restore to original position
		err = internal.RestoreReaderPos(buf, savedPos)
		require.NoError(t, err)

		// Verify we're back at saved position
		currentPos, err := buf.Seek(0, io.SeekCurrent)
		require.NoError(t, err)
		assert.Equal(t, int64(7), currentPos)

		// Read from saved position
		nextData := make([]byte, 6)
		n, err = buf.Read(nextData)
		require.NoError(t, err)
		assert.Equal(t, 6, n)
		assert.Equal(t, []byte("World!"), nextData)
	})
}

// errorSeeker is a mock seeker that always returns an error on Seek
type errorSeeker struct {
	seekErr error
}

func (e *errorSeeker) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func (e *errorSeeker) Seek(offset int64, whence int) (int64, error) {
	return 0, e.seekErr
}

// brokenSeeker is a mock seeker that can succeed or fail based on the operation
type brokenSeeker struct {
	pos           int64
	startSeekErr  error
}

func (b *brokenSeeker) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func (b *brokenSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekCurrent:
		return b.pos, nil
	case io.SeekStart:
		if b.startSeekErr != nil {
			return 0, b.startSeekErr
		}
		b.pos = offset
		return b.pos, nil
	default:
		return 0, errors.New("unsupported whence")
	}
}
