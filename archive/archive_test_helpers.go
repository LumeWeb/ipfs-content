package archive

import (
	"io"
	"io/fs"
	"time"
)

// testFileInfo implements fs.FileInfo for testing
type testFileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (t *testFileInfo) Name() string       { return t.name }
func (t *testFileInfo) Size() int64        { return t.size }
func (t *testFileInfo) Mode() fs.FileMode  { return 0644 }
func (t *testFileInfo) ModTime() time.Time { return t.modTime }
func (t *testFileInfo) IsDir() bool        { return false }
func (t *testFileInfo) Sys() interface{}   { return nil }

// testMemFile implements fs.File for testing archives
type testMemFile struct {
	name    string
	content []byte
	offset  int
}

func (t *testMemFile) Stat() (fs.FileInfo, error) {
	return &testFileInfo{
		name:    t.name,
		size:    int64(len(t.content)),
		modTime: time.Now(),
	}, nil
}
func (t *testMemFile) Read(p []byte) (n int, err error) {
	if t.offset >= len(t.content) {
		return 0, io.EOF
	}
	n = copy(p, t.content[t.offset:])
	t.offset += n
	return n, nil
}
func (t *testMemFile) Close() error { return nil }

// testFileHeader implements interfaces for TAR headers
type testFileHeader struct {
	name  string
	size  int64
	mode  int64
	mtime time.Time
}

func (h *testFileHeader) Name() string       { return h.name }
func (h *testFileHeader) Size() int64        { return h.size }
func (h *testFileHeader) Mode() int64        { return h.mode }
func (h *testFileHeader) ModTime() time.Time { return h.mtime }
