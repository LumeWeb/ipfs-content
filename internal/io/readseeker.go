package io

import (
	"io"
	"io/fs"
	"time"
)

// ReadSeekCloser wraps an io.Reader to implement io.ReadSeekCloser and fs.File.
// It provides Read, Seek, Close, and Stat methods. If the underlying reader
// supports Seek, it will be used. Otherwise, seeking returns an error.
type ReadSeekCloser struct {
	r    io.Reader
	s    io.Seeker
	name string
	size int64
	mode fs.FileMode
	mod  time.Time
}

// NewReadSeekCloser creates a new ReadSeekCloser wrapping the given reader.
// The returned ReadSeekCloser implements io.ReadSeekCloser and fs.File.
// If the reader also implements io.Seeker, seeking will be supported.
func NewReadSeekCloser(r io.Reader) *ReadSeekCloser {
	s, _ := r.(io.Seeker)
	return &ReadSeekCloser{
		r:    r,
		s:    s,
		name: "",
		size: 0,
		mode: 0600,
		mod:  time.Now(),
	}
}

// NewReadSeekCloserWithInfo creates a new ReadSeekCloser with file metadata.
func NewReadSeekCloserWithInfo(r io.Reader, name string, size int64, mode fs.FileMode, mod time.Time) *ReadSeekCloser {
	s, _ := r.(io.Seeker)
	return &ReadSeekCloser{
		r:    r,
		s:    s,
		name: name,
		size: size,
		mode: mode,
		mod:  mod,
	}
}

func (rsc *ReadSeekCloser) Read(p []byte) (n int, err error) {
	return rsc.r.Read(p)
}

func (rsc *ReadSeekCloser) Seek(offset int64, whence int) (int64, error) {
	if rsc.s == nil {
		return 0, &ioError{msg: "seek not supported"}
	}
	return rsc.s.Seek(offset, whence)
}

func (rsc *ReadSeekCloser) Close() error {
	if c, ok := rsc.r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// Stat implements fs.File.Stat.
// Returns file info based on the metadata provided during construction.
// If no metadata was provided, returns minimal default info.
func (rsc *ReadSeekCloser) Stat() (fs.FileInfo, error) {
	return &readSeekerFileInfo{
		name:  rsc.name,
		size:  rsc.size,
		mode:  rsc.mode,
		mod:   rsc.mod,
		isDir: false,
	}, nil
}

// readSeekerFileInfo implements fs.FileInfo for ReadSeekCloser.
type readSeekerFileInfo struct {
	name  string
	size  int64
	mode  fs.FileMode
	mod   time.Time
	isDir bool
}

func (fi *readSeekerFileInfo) Name() string       { return fi.name }
func (fi *readSeekerFileInfo) Size() int64        { return fi.size }
func (fi *readSeekerFileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi *readSeekerFileInfo) ModTime() time.Time { return fi.mod }
func (fi *readSeekerFileInfo) IsDir() bool        { return fi.isDir }
func (fi *readSeekerFileInfo) Sys() any           { return nil }

// ioError provides a simple error type for io operations.
type ioError struct {
	msg string
}

func (e *ioError) Error() string {
	return e.msg
}
