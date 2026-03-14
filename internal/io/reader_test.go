package io

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrepareReaderPreservePos tests the PrepareReaderPreservePos function
func TestPrepareReaderPreservePos(t *testing.T) {
	t.Run("preserves position and seeks to beginning", func(t *testing.T) {
		data := []byte("Hello, World!")
		buf := bytes.NewReader(data)

		// Record initial size
		initialSize := int64(buf.Size())
		require.Equal(t, int64(13), initialSize)

		// Read some data to move the position
		_, _ = buf.Read(make([]byte, 5))
		remaining := int64(buf.Len())
		require.Equal(t, int64(8), remaining)

		// Prepare reader - should save position and seek to beginning
		// Note: position is calculated from the size and remaining
		expectedPos := initialSize - remaining
		pos, err := PrepareReaderPreservePos(buf)
		require.NoError(t, err)
		assert.Equal(t, expectedPos, pos, "Should return original position")

		// Verify we're at beginning
		currentPos, _ := buf.Seek(0, io.SeekCurrent)
		assert.Equal(t, int64(0), currentPos, "Should be at beginning")
	})

	t.Run("handles position at beginning", func(t *testing.T) {
		data := []byte("test")
		buf := bytes.NewReader(data)

		pos, err := PrepareReaderPreservePos(buf)
		require.NoError(t, err)
		assert.Equal(t, int64(0), pos)

		currentPos, _ := buf.Seek(0, io.SeekCurrent)
		assert.Equal(t, int64(0), currentPos)
	})

	t.Run("handles position at end", func(t *testing.T) {
		data := []byte("test")
		buf := bytes.NewReader(data)

		// Read all to move to end
		_, _ = buf.Read(make([]byte, len(data)))

		pos, err := PrepareReaderPreservePos(buf)
		require.NoError(t, err)
		assert.Equal(t, int64(4), pos)

		currentPos, _ := buf.Seek(0, io.SeekCurrent)
		assert.Equal(t, int64(0), currentPos)
	})
}

// TestRestoreReaderPos tests the RestoreReaderPos function
func TestRestoreReaderPos(t *testing.T) {
	t.Run("restores original position", func(t *testing.T) {
		data := []byte("Hello, World!")
		buf := bytes.NewReader(data)

		// Move to position 5
		_, _ = buf.Seek(5, io.SeekStart)
		require.Equal(t, int64(5), getPos(t, buf))

		// Prepare and then restore
		origPos, _ := PrepareReaderPreservePos(buf)
		err := RestoreReaderPos(buf, origPos)
		require.NoError(t, err)

		currentPos, _ := buf.Seek(0, io.SeekCurrent)
		assert.Equal(t, origPos, currentPos, "Should restore to original position")
	})

	t.Run("restores to beginning", func(t *testing.T) {
		data := []byte("test")
		buf := bytes.NewReader(data)

		err := RestoreReaderPos(buf, 0)
		require.NoError(t, err)

		currentPos, _ := buf.Seek(0, io.SeekCurrent)
		assert.Equal(t, int64(0), currentPos)
	})

	t.Run("restores to end", func(t *testing.T) {
		data := []byte("test")
		buf := bytes.NewReader(data)

		err := RestoreReaderPos(buf, 4)
		require.NoError(t, err)

		currentPos, _ := buf.Seek(0, io.SeekCurrent)
		assert.Equal(t, int64(4), currentPos)
	})
}

// TestPrepareAndRestoreReaderPos tests the combined workflow
func TestPrepareAndRestoreReaderPos(t *testing.T) {
	t.Run("prepare then restore full cycle", func(t *testing.T) {
		data := []byte("0123456789")
		buf := bytes.NewReader(data)

		// Move to a non-zero position
		_, _ = buf.Seek(6, io.SeekStart)
		origPos := getPos(t, buf)
		assert.Equal(t, int64(6), origPos)

		// Prepare - should go to beginning
		savedPos, err := PrepareReaderPreservePos(buf)
		require.NoError(t, err)
		assert.Equal(t, origPos, savedPos)
		assert.Equal(t, int64(0), getPos(t, buf))

		// Read some data
		b := make([]byte, 3)
		n, _ := buf.Read(b)
		assert.Equal(t, 3, n)
		assert.Equal(t, []byte("012"), b)

		// Restore - should go back to original position
		err = RestoreReaderPos(buf, savedPos)
		require.NoError(t, err)
		assert.Equal(t, origPos, getPos(t, buf))
	})
}

// TestPrepareReaderPreservePos_ErrorHandling tests error cases for PrepareReaderPreservePos
func TestPrepareReaderPreservePos_ErrorHandling(t *testing.T) {
	t.Run("handles error when getting current position", func(t *testing.T) {
		errSeeker := &errorSeeker{seekErr: assert.AnError}
		_, err := PrepareReaderPreservePos(errSeeker)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get reader position")
	})

	t.Run("handles error when seeking to beginning", func(t *testing.T) {
		brokenSeeker := &brokenSeeker{pos: 10, startSeekErr: assert.AnError}
		_, err := PrepareReaderPreservePos(brokenSeeker)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to seek to beginning")
	})
}

// TestRestoreReaderPos_ErrorHandling tests error cases for RestoreReaderPos
func TestRestoreReaderPos_ErrorHandling(t *testing.T) {
	t.Run("handles seek errors", func(t *testing.T) {
		errorSeeker := &brokenSeeker{pos: 5, startSeekErr: assert.AnError}
		err := RestoreReaderPos(errorSeeker, 10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to restore original reader position")
	})
}

// errorSeeker is a mock seeker that always returns an error on Seek
type errorSeeker struct {
	seekErr error
}

func (e *errorSeeker) Read(p []byte) (n int, err error) {
	return 0, assert.AnError
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
	return 0, assert.AnError
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
		return 0, assert.AnError
	}
}

// TestNewReadSeekCloser tests the NewReadSeekCloser function
func TestNewReadSeekCloser(t *testing.T) {
	t.Run("wraps Reader with Seek support", func(t *testing.T) {
		data := []byte("test content")
		buf := bytes.NewReader(data)

		rsc := NewReadSeekCloser(buf)

		require.NotNil(t, rsc)
		assert.NotNil(t, rsc.s, "Should have seeker")
	})
}

// TestReadSeekCloser_Read tests the Read method
func TestReadSeekCloser_Read(t *testing.T) {
	t.Run("reads data", func(t *testing.T) {
		data := []byte("hello world")
		buf := bytes.NewReader(data)
		rsc := NewReadSeekCloser(buf)

		p := make([]byte, 5)
		n, err := rsc.Read(p)

		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("hello"), p)
	})

	t.Run("handles EOF", func(t *testing.T) {
		data := []byte("test")
		buf := bytes.NewReader(data)
		rsc := NewReadSeekCloser(buf)

		// Read all data
		_, _ = rsc.Read(make([]byte, 4))

		// Read again should get EOF
		p := make([]byte, 4)
		n, err := rsc.Read(p)

		assert.Equal(t, 0, n)
		assert.Equal(t, io.EOF, err)
	})
}

// TestReadSeekCloser_Seek tests the Seek method
func TestReadSeekCloser_Seek(t *testing.T) {
	t.Run("seeks when supported", func(t *testing.T) {
		data := []byte("0123456789")
		buf := bytes.NewReader(data)
		rsc := NewReadSeekCloser(buf)

		pos, err := rsc.Seek(5, io.SeekStart)
		require.NoError(t, err)
		assert.Equal(t, int64(5), pos)

		b := make([]byte, 1)
		n, _ := rsc.Read(b)
		assert.Equal(t, 1, n)
		assert.Equal(t, byte('5'), b[0])
	})

	t.Run("returns error when seek not supported", func(t *testing.T) {
		data := []byte("test")
		buf := &pureReader{bytes.NewReader(data)}
		rsc := NewReadSeekCloser(buf)

		_, err := rsc.Seek(3, io.SeekStart)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "seek not supported")
	})

	t.Run("seek to end", func(t *testing.T) {
		data := []byte("test")
		buf := bytes.NewReader(data)
		rsc := NewReadSeekCloser(buf)

		_, err := rsc.Seek(0, io.SeekEnd)
		require.NoError(t, err)

		b := make([]byte, 1)
		n, err := rsc.Read(b)
		assert.Equal(t, 0, n)
		assert.Equal(t, io.EOF, err)
	})
}

// TestReadSeekCloser_Close tests the Close method
func TestReadSeekCloser_Close(t *testing.T) {
	t.Run("closes when underlying reader is Closer", func(t *testing.T) {
		buf := &trackedCloser{buf: bytes.NewBufferString("test")}
		rsc := NewReadSeekCloser(buf)

		err := rsc.Close()
		require.NoError(t, err)
		assert.True(t, buf.closed, "Should have closed")
	})

	t.Run("no error when not a Closer", func(t *testing.T) {
		data := []byte("test")
		buf := bytes.NewReader(data)
		rsc := NewReadSeekCloser(buf)

		err := rsc.Close()
		require.NoError(t, err)
	})
}

// TestReadSeekCloser tests the full interface
func TestReadSeekCloser(t *testing.T) {
	t.Run("full ReadSeekCloser workflow", func(t *testing.T) {
		data := []byte("Hello, World!")
		buf := bytes.NewReader(data)

		rsc := NewReadSeekCloser(buf)

		// Read some data
		p := make([]byte, 5)
		n, err := rsc.Read(p)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("Hello"), p)

		// Seek back
		pos, err := rsc.Seek(0, io.SeekStart)
		require.NoError(t, err)
		assert.Equal(t, int64(0), pos)

		// Read again
		p2 := make([]byte, 12)
		n, err = rsc.Read(p2)
		require.NoError(t, err)
		assert.Equal(t, 12, n)
		assert.Equal(t, []byte("Hello, World"), p2)

		// Close
		err = rsc.Close()
		require.NoError(t, err)
	})
}

// getPos is a helper to get the current position
func getPos(t *testing.T, seeker io.Seeker) int64 {
	t.Helper()
	pos, err := seeker.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	return pos
}

// trackedCloser tracks whether Close was called
type trackedCloser struct {
	buf    *bytes.Buffer
	closed bool
}

func (t *trackedCloser) Read(p []byte) (int, error) {
	return t.buf.Read(p)
}

func (t *trackedCloser) Close() error {
	t.closed = true
	return nil
}
