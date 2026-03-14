package internal

import (
	"fmt"
	"io"
)

// PrepareReaderPreservePos prepares a reader for operations while preserving the current position
// Returns the current position and ensures the reader is at the beginning
func PrepareReaderPreservePos(seeker io.Seeker) (int64, error) {
	// Get current position
	pos, err := seeker.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, fmt.Errorf("failed to get reader position: %w", err)
	}

	// Seek to beginning for processing
	_, err = seeker.Seek(0, io.SeekStart)
	if err != nil {
		return 0, fmt.Errorf("failed to seek to beginning: %w", err)
	}

	return pos, nil
}

// RestoreReaderPos restores a reader to its original position
func RestoreReaderPos(seeker io.Seeker, pos int64) error {
	_, err := seeker.Seek(pos, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to restore original reader position: %w", err)
	}
	return nil
}
