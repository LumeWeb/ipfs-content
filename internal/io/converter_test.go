package io

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToByteReader(t *testing.T) {
	t.Run("reader already implements ByteReader", func(t *testing.T) {
		buf := bytes.NewReader([]byte("test"))
		var r io.Reader = buf
		br := ToByteReader(r)
		require.Same(t, buf, br)
	})

	t.Run("wraps pure Reader", func(t *testing.T) {
		buf := bytes.NewReader([]byte("test"))
		pr := &pureReader{buf}
		var r io.Reader = pr
		
		br := ToByteReader(r)
		require.NotNil(t, br)
		require.NotSame(t, pr, br)
		
		b, err := br.ReadByte()
		require.NoError(t, err)
		require.Equal(t, byte('t'), b)
	})
}

func TestToByteReadSeeker(t *testing.T) {
	t.Run("already implements ByteReadSeeker", func(t *testing.T) {
		buf := &testByteReadSeeker{&bytes.Buffer{}}
		brs := ToByteReadSeeker(buf)
		require.Same(t, buf, brs)
	})

	t.Run("wraps ReadSeeker without ByteReader", func(t *testing.T) {
		rs := &pureReadSeeker{bytes.NewReader([]byte("test"))}
		
		brs := ToByteReadSeeker(rs)
		require.NotNil(t, brs)
		require.NotSame(t, rs, brs)
		
		b, err := brs.ReadByte()
		require.NoError(t, err)
		require.Equal(t, byte('t'), b)
	})

	t.Run("wraps pure Reader", func(t *testing.T) {
		buf := &bytes.Buffer{}
		buf.WriteString("test")
		
		brs := ToByteReadSeeker(buf)
		require.NotNil(t, brs)
		require.NotSame(t, buf, brs)
		
		p := make([]byte, 4)
		n, err := brs.Read(p)
		require.NoError(t, err)
		require.Equal(t, 4, n)
		require.Equal(t, []byte("test"), p)
		
		_, err = brs.ReadByte()
		require.Error(t, err)
	})
}

func TestToReadSeeker(t *testing.T) {
	t.Run("already implements ReadSeeker", func(t *testing.T) {
		buf := bytes.NewReader([]byte("test"))
		rs := ToReadSeeker(buf)
		require.Same(t, buf, rs)
	})

	t.Run("wraps ReaderAt", func(t *testing.T) {
		data := []byte("test")
		ra := &testReaderAt{data}
		
		rs := ToReadSeeker(ra)
		require.NotNil(t, rs)
		require.NotSame(t, ra, rs)
		
		p := make([]byte, 4)
		n, err := rs.Read(p)
		require.NoError(t, err)
		require.Equal(t, 4, n)
		require.Equal(t, data, p)
	})
}

func TestToReaderAt(t *testing.T) {
	t.Run("already implements ReaderAt", func(t *testing.T) {
		rs := bytes.NewReader([]byte("test"))
		ra := ToReaderAt(rs)
		require.Same(t, rs, ra)
	})

	t.Run("wraps ReadSeeker", func(t *testing.T) {
		data := []byte("test")
		buf := bytes.NewReader(data)
		rs := &testReaderAtWrapper{buf}
		
		ra := ToReaderAt(rs)
		require.NotNil(t, ra)
		require.NotSame(t, rs, ra)
		
		p := make([]byte, 4)
		n, err := ra.ReadAt(p, 0)
		require.NoError(t, err)
		require.Equal(t, 4, n)
		require.Equal(t, []byte("test"), p)
	})
}

func TestReaderPlusByte_ReadByte(t *testing.T) {
	buf := bytes.NewBufferString("test")
	br := ToByteReader(buf)

	b, err := br.ReadByte()
	require.NoError(t, err)
	require.Equal(t, byte('t'), b)

	b, err = br.ReadByte()
	require.NoError(t, err)
	require.Equal(t, byte('e'), b)

	_, err = br.ReadByte()
	require.NoError(t, err)

	_, err = br.ReadByte()
	require.NoError(t, err)

	_, err = br.ReadByte()
	require.Error(t, err)
}

func TestReadSeekerPlusByte_ReadByte(t *testing.T) {
	rs := bytes.NewReader([]byte("test"))
	brs := ToByteReadSeeker(rs)

	b, err := brs.ReadByte()
	require.NoError(t, err)
	require.Equal(t, byte('t'), b)
}

func TestDiscardingReadSeekerPlusByte_ReadByte(t *testing.T) {
	var brs ByteReadSeeker = ToByteReadSeeker(bytes.NewBufferString("test"))
	
	p := make([]byte, 2)
	n, err := brs.Read(p)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	
	b, err := brs.ReadByte()
	require.NoError(t, err)
	require.Equal(t, byte('s'), b)
}

func TestDiscardingReadSeekerPlusByte_Seek(t *testing.T) {
	t.Run("SeekStart forward", func(t *testing.T) {
		rs := io.Reader(bytes.NewBufferString("test"))
		drsb := ToByteReadSeeker(rs).(*discardingReadSeekerPlusByte)
		
		// Read some bytes
		p := make([]byte, 2)
		_, _ = drsb.Read(p)
		
		// Seek forward from start - should skip bytes
		pos, err := drsb.Seek(3, io.SeekStart)
		require.NoError(t, err)
		require.Equal(t, int64(3), pos)
	})

	t.Run("SeekStart backwards not supported", func(t *testing.T) {
		rs := io.Reader(bytes.NewBufferString("test"))
		drsb := ToByteReadSeeker(rs).(*discardingReadSeekerPlusByte)
		
		// Read some bytes
		p := make([]byte, 2)
		_, _ = drsb.Read(p)
		
		// Seek backward - should error
		_, err := drsb.Seek(1, io.SeekStart)
		require.Error(t, err)
	})

	t.Run("SeekCurrent forward", func(t *testing.T) {
		rs := io.Reader(bytes.NewBufferString("test"))
		drsb := ToByteReadSeeker(rs).(*discardingReadSeekerPlusByte)
		
		// Read some bytes
		p := make([]byte, 2)
		_, _ = drsb.Read(p)
		
		// Seek forward from current
		pos, err := drsb.Seek(1, io.SeekCurrent)
		require.NoError(t, err)
		require.Equal(t, int64(3), pos)
	})

	t.Run("SeekCurrent zero", func(t *testing.T) {
		rs := io.Reader(bytes.NewBufferString("test"))
		drsb := ToByteReadSeeker(rs).(*discardingReadSeekerPlusByte)
		
		pos, err := drsb.Seek(0, io.SeekCurrent)
		require.NoError(t, err)
		require.Equal(t, int64(0), pos)
	})

	t.Run("SeekEnd not supported", func(t *testing.T) {
		rs := io.Reader(bytes.NewBufferString("test"))
		drsb := ToByteReadSeeker(rs).(*discardingReadSeekerPlusByte)
		
		_, err := drsb.Seek(0, io.SeekEnd)
		require.Error(t, err)
	})

	t.Run("SeekCurrent skips bytes", func(t *testing.T) {
		rs := io.Reader(bytes.NewBufferString("test"))
		drsb := ToByteReadSeeker(rs).(*discardingReadSeekerPlusByte)
		
		// Seek to skip first 2 bytes
		pos, err := drsb.Seek(2, io.SeekCurrent)
		require.NoError(t, err)
		require.Equal(t, int64(2), pos)
		
		// Read next byte (should be third byte)
		b, err := drsb.ReadByte()
		require.NoError(t, err)
		require.Equal(t, byte('s'), b)
	})
}

func TestReaderAtSeeker_Read(t *testing.T) {
	t.Run("reads sequentially", func(t *testing.T) {
		data := []byte("test1234")
		ra := &testReaderAt{data}
		rs := ToReadSeeker(ra)
		
		p1 := make([]byte, 4)
		n1, err := rs.Read(p1)
		require.NoError(t, err)
		require.Equal(t, 4, n1)
		require.Equal(t, []byte("test"), p1)
		
		p2 := make([]byte, 4)
		n2, err := rs.Read(p2)
		require.NoError(t, err)
		require.Equal(t, 4, n2)
		require.Equal(t, []byte("1234"), p2)
	})
}

func TestReaderAtSeeker_Seek(t *testing.T) {
	t.Run("SeekStart", func(t *testing.T) {
		data := []byte("test")
		ra := &testReaderAt{data}
		rs := ToReadSeeker(ra)
		
		pos, err := rs.Seek(2, io.SeekStart)
		require.NoError(t, err)
		require.Equal(t, int64(2), pos)
		
		p := make([]byte, 2)
		n, err := rs.Read(p)
		require.NoError(t, err)
		require.Equal(t, 2, n)
		require.Equal(t, []byte("st"), p)
	})

	t.Run("SeekCurrent forward", func(t *testing.T) {
		data := []byte("test")
		ra := &testReaderAt{data}
		rs := ToReadSeeker(ra)
		
		pos, err := rs.Seek(2, io.SeekStart)
		require.NoError(t, err)
		
		pos, err = rs.Seek(1, io.SeekCurrent)
		require.NoError(t, err)
		require.Equal(t, int64(3), pos)
		
		p := make([]byte, 1)
		n, err := rs.Read(p)
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Equal(t, []byte("t"), p)
	})

	t.Run("SeekCurrent backwards", func(t *testing.T) {
		data := []byte("test")
		ra := &testReaderAt{data}
		rs := ToReadSeeker(ra)
		
		pos, err := rs.Seek(3, io.SeekStart)
		require.NoError(t, err)
		
		pos, err = rs.Seek(-1, io.SeekCurrent)
		require.NoError(t, err)
		require.Equal(t, int64(2), pos)
		
		p := make([]byte, 2)
		n, err := rs.Read(p)
		require.NoError(t, err)
		require.Equal(t, 2, n)
		require.Equal(t, []byte("st"), p)
	})

	t.Run("SeekEnd not supported", func(t *testing.T) {
		data := []byte("test")
		ra := &testReaderAt{data}
		rs := ToReadSeeker(ra)
		
		_, err := rs.Seek(0, io.SeekEnd)
		require.Error(t, err)
	})
}

func TestReadSeekerAt_ReadAt(t *testing.T) {
	data := []byte("test1234")
	rs := bytes.NewReader(data)
	ra := ToReaderAt(rs)
	
	p := make([]byte, 4)
	n, err := ra.ReadAt(p, 2)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, []byte("st12"), p)
	
	// ReadAt should not position (can read from same position)
	n, err = ra.ReadAt(p, 2)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, []byte("st12"), p)
}

// Test helpers

type testByteReadSeeker struct {
	*bytes.Buffer
}

func (t *testByteReadSeeker) ReadByte() (byte, error) {
	return t.Buffer.ReadByte()
}

func (t *testByteReadSeeker) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}

type testReaderAt struct {
	data []byte
}

func (t *testReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(t.data)) {
		return 0, io.EOF
	}
	end := off + int64(len(p))
	if end > int64(len(t.data)) {
		end = int64(len(t.data))
	}
	copy(p, t.data[off:end])
	return int(end - off), nil
}

type testReaderAtWrapper struct {
	rs io.ReadSeeker
}

func (t *testReaderAtWrapper) Read(p []byte) (n int, err error) {
	return t.rs.Read(p)
}

func (t *testReaderAtWrapper) Seek(offset int64, whence int) (int64, error) {
	return t.rs.Seek(offset, whence)
}

type pureReader struct {
	r *bytes.Reader
}

func (p *pureReader) Read(b []byte) (n int, err error) {
	return p.r.Read(b)
}

type pureReadSeeker struct {
	rs *bytes.Reader
}

func (p *pureReadSeeker) Read(b []byte) (n int, err error) {
	return p.rs.Read(b)
}

func (p *pureReadSeeker) Seek(offset int64, whence int) (int64, error) {
	return p.rs.Seek(offset, whence)
}
