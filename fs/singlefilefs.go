package fs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"
)

// SingleFileFS implements fs.FS to wrap a single file from an existing file handle.
// This allows using *os.File or any fs.File with.UploadFromFS directly.
//
// Example usage with os.File:
//
//	file, err := os.Open("path/to/file.txt")
//	if err != nil {
//	    return err
//	}
//	defer file.Close()
//
//	filesystem := NewSingleFileFS(file, "file.txt")
//	result, err := uploadService.UploadFromFS(ctx, filesystem, "file.txt", opts)
type SingleFileFS struct {
	file     fs.File
	filename string
}

// NewSingleFileFS creates a new filesystem containing a single file from an existing file handle.
// The file must be opened for reading. The caller is responsible for closing the file
// after the upload operation completes.
//
// file is the open file handle to wrap.
// filename is the name to use for the file in the filesystem.
func NewSingleFileFS(file fs.File, filename string) *SingleFileFS {
	return &SingleFileFS{
		file:     file,
		filename: filename,
	}
}

// Open implements fs.FS.Open.
// Only the root "." and the single filename are valid paths.
// For "." we return the file itself since SingleFileFS represents a single file,
// not a directory containing the file.
//
// Note: Each call to Open() returns the same underlying file handle. The file position
// is reset to 0 on each open call to support CAR generation's two-pass pattern where
// the file is read, then reopened and read again from the beginning.
func (s *SingleFileFS) Open(name string) (fs.File, error) {
	if name == "." || name == s.filename {
		// Seek to the beginning so reopens start from a clean state
		if seeker, ok := s.file.(io.Seeker); ok {
			if _, err := seeker.Seek(0, io.SeekStart); err != nil {
				return nil, fmt.Errorf("failed to reset file position for CAR generation: %w", err)
			}
		}
		return s.file, nil
	}
	return nil, fs.ErrNotExist
}

// Stat implements fs.StatFS.Stat.
// Returns file info for "." or the single filename.
func (s *SingleFileFS) Stat(name string) (fs.FileInfo, error) {
	if name == "." || name == s.filename {
		info, err := s.file.Stat()
		if err != nil {
			// Wrap any error as fs.ErrNotExist for consistency with BytesFS
			return nil, fs.ErrNotExist
		}
		// Ensure IsDir returns false for the wrapped file
		return &singleFileInfo{
			name:  info.Name(),
			size:  info.Size(),
			mode:  info.Mode(),
			mtime: info.ModTime(),
			isDir: false,
		}, nil
	}
	return nil, fs.ErrNotExist
}

// singleFile adapts a *os.File to implement Seek for fs.FS compatibility.
// Most fs.File implementations support Seek, but we ensure it here.
type singleFile struct {
	fs.File
	stat *singleFileInfo
}

// Stat implements fs.File.Stat.
func (f *singleFile) Stat() (fs.FileInfo, error) {
	if f.stat != nil {
		return f.stat, nil
	}
	info, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	// Cast to ensure isDir is false for regular files
	if stat, ok := info.(*singleFileInfo); ok {
		f.stat = stat
		return stat, nil
	}
	// Wrap the FileInfo to ensure IsDir returns false
	f.stat = &singleFileInfo{
		name:  info.Name(),
		size:  info.Size(),
		mode:  info.Mode(),
		mtime: info.ModTime(),
		isDir: false,
	}
	return f.stat, nil
}

// Seek implements io.Seeker for repositioning within the file.
// Supports SeekStart, SeekCurrent, and SeekEnd.
func (f *singleFile) Seek(offset int64, whence int) (int64, error) {
	// Try to use the underlying Seek if available
	if seeker, ok := f.File.(io.Seeker); ok {
		return seeker.Seek(offset, whence)
	}
	// If the file doesn't support Seek, we can't implement this
	return 0, fmt.Errorf("file does not support seeking")
}

// singleFileInfo implements fs.FileInfo with a fixed IsDir=false.
type singleFileInfo struct {
	name  string
	size  int64
	mode  fs.FileMode
	mtime time.Time
	isDir bool
}

func (fi *singleFileInfo) Name() string       { return fi.name }
func (fi *singleFileInfo) Size() int64        { return fi.size }
func (fi *singleFileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi *singleFileInfo) ModTime() time.Time { return fi.mtime }
func (fi *singleFileInfo) IsDir() bool        { return fi.isDir }
func (fi *singleFileInfo) Sys() any           { return nil }

// FileFromStat wraps a file Stat result to ensure IsDir returns false.
// This is useful when you have a *os.File and want to use it with
// SingleFileFS but need to ensure the FileInfo reports isDir=false.
func FileFromStat(file fs.File, stat os.FileInfo) fs.File {
	if stat == nil {
		return file
	}
	// Wrap to ensure proper IsDir behavior
	return &singleFile{
		File: file,
		stat: &singleFileInfo{
			name:  stat.Name(),
			size:  stat.Size(),
			mode:  stat.Mode(),
			mtime: stat.ModTime(),
			isDir: false,
		},
	}
}
