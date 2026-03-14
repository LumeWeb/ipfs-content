package archive

import (
	"bytes"
	"errors"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	carv1 "github.com/lumeweb/ipfs-content/internal/carv1"
)

// CarTestSuite provides a suite for CAR-related tests
func TestCarTestSuite(t *testing.T) {
	suite.Run(t, new(CarTestSuite))
}

type CarTestSuite struct {
	suite.Suite
}

// createValidCARHeader creates a minimal valid CARv1 for testing
func createValidCARHeader(rootCID cid.Cid) []byte {
	header := &carv1.CarHeader{
		Roots:   []cid.Cid{rootCID},
		Version: 1,
	}
	
	buf := new(bytes.Buffer)
	err := carv1.WriteHeader(header, buf)
	if err != nil {
		panic("failed to create CAR header: " + err.Error())
	}
	
	return buf.Bytes()
}

func (s *CarTestSuite) TestDetectCAR_WithValidCARData() {
	// Create a block and get its CID
	block := blocks.NewBlock([]byte("test data"))
	rootCID := block.Cid()
	
	// Create valid CAR header
	header := createValidCARHeader(rootCID)
	
	// Add a valid block after header
	buf := new(bytes.Buffer)
	buf.Write(header)
	err := carv1.WriteBlock(buf, block.Cid(), block.RawData())
	s.Require().NoError(err)
	
	reader := bytes.NewReader(buf.Bytes())
	format, detected := detectCAR(reader)
	
	assert.True(s.T(), detected)
	assert.Equal(s.T(), FormatCAR, format)
}

func (s *CarTestSuite) TestDetectCAR_WithEmptyData() {
	reader := bytes.NewReader([]byte{})
	format, detected := detectCAR(reader)
	
	assert.False(s.T(), detected)
	assert.Equal(s.T(), FormatUnknown, format)
}

func (s *CarTestSuite) TestDetectCAR_WithShortData() {
	// Data shorter than minimum CAR header size (12 bytes)
	reader := bytes.NewReader([]byte{0x01, 0x02, 0x03, 0x04, 0x05})
	format, detected := detectCAR(reader)
	
	assert.False(s.T(), detected)
	assert.Equal(s.T(), FormatUnknown, format)
}

func (s *CarTestSuite) TestDetectCAR_WithInvalidData() {
	// Random bytes that don't form a valid CAR header
	invalidData := []byte{
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xaa, 0xaa, 0xaa, 0xaa,
	}
	
	reader := bytes.NewReader(invalidData)
	format, detected := detectCAR(reader)
	
	assert.False(s.T(), detected)
	assert.Equal(s.T(), FormatUnknown, format)
}

func (s *CarTestSuite) TestDetectCAR_WithPartialValidData() {
	// Data that's valid but only partial (less than full header, but > 12 bytes)
	// Create incomplete CAR data
	block := blocks.NewBlock([]byte("test data"))
	rootCID := block.Cid()
	header := createValidCARHeader(rootCID)
	
	// Take only first 10 bytes
	partialData := header[:10]
	
	reader := bytes.NewReader(partialData)
	format, detected := detectCAR(reader)
	
	// Should not detect as CAR since we can't read the full header
	assert.False(s.T(), detected)
	assert.Equal(s.T(), FormatUnknown, format)
}

func (s *CarTestSuite) TestDetectCAR_WithReaderError() {
	// Simulate a reader that returns an error
	errReader := &errorReader{err: errors.New("read error")}
	
	format, detected := detectCAR(errReader)
	
	assert.False(s.T(), detected)
	assert.Equal(s.T(), FormatUnknown, format)
}

func (s *CarTestSuite) TestDetectCAR_WithInvalidCBORHeader() {
	// Create header with CBOR that's invalid for CAR format
	invalidCBOR := []byte{
		0x12, // Invalid CBOR tag
		0x34,
		0x56,
		0x78,
		0x90,
		0xab,
		0xcd,
		0xef,
	}
	
	reader := bytes.NewReader(invalidCBOR)
	format, detected := detectCAR(reader)
	
	assert.False(s.T(), detected)
	assert.Equal(s.T(), FormatUnknown, format)
}

func (s *CarTestSuite) TestDetectCAR_WithZeroLengthHeader() {
	// Use a length prefix of 0 followed by minimal data
	block := blocks.NewBlock([]byte("test data"))
	rootCID := block.Cid()
	header := createValidCARHeader(rootCID)
	
	// Create a buffer with only the header (which includes the length prefix)
	reader := bytes.NewReader(header)
	format, detected := detectCAR(reader)
	
	// Should detect valid CAR
	assert.True(s.T(), detected)
	assert.Equal(s.T(), FormatCAR, format)
}

func (s *CarTestSuite) TestDetectCAR_DetectsMinimalCAR() {
	// Test with just a minimal valid CAR (header only, no blocks)
	block := blocks.NewBlock([]byte("x"))
	rootCID := block.Cid()
	
	header := &carv1.CarHeader{
		Roots:   []cid.Cid{rootCID},
		Version: 1,
	}
	
	buf := new(bytes.Buffer)
	err := carv1.WriteHeader(header, buf)
	s.Require().NoError(err)
	
	// Don't add any blocks - just the header
	reader := bytes.NewReader(buf.Bytes())
	format, detected := detectCAR(reader)
	
	assert.True(s.T(), detected)
	assert.Equal(s.T(), FormatCAR, format)
}

func (s *CarTestSuite) TestDetectCAR_RejectsInvalidVersion() {
	// CAR format requires version 1
	// Create a header with version 2 (invalid)
	block := blocks.NewBlock([]byte("test data"))
	rootCID := block.Cid()
	
	header := &carv1.CarHeader{
		Roots:   []cid.Cid{rootCID},
		Version: 2, // Invalid version
	}
	
	buf := new(bytes.Buffer)
	err := carv1.WriteHeader(header, buf)
	s.Require().NoError(err)
	
	reader := bytes.NewReader(buf.Bytes())
	format, detected := detectCAR(reader)
	
	// Should reject CAR with invalid version
	assert.False(s.T(), detected)
	assert.Equal(s.T(), FormatUnknown, format)
}

func (s *CarTestSuite) TestDetectCAR_AcceptsEmptyRoots() {
	// CAR files with empty roots are still valid CAR format (just empty)
	header := &carv1.CarHeader{
		Roots:   []cid.Cid{}, // Empty roots
		Version: 1,
	}
	
	buf := new(bytes.Buffer)
	err := carv1.WriteHeader(header, buf)
	s.Require().NoError(err)
	
	reader := bytes.NewReader(buf.Bytes())
	format, detected := detectCAR(reader)
	
	// Should accept CAR with empty roots (it's a valid, if empty, CAR format)
	assert.True(s.T(), detected)
	assert.Equal(s.T(), FormatCAR, format)
}

func (s *CarTestSuite) TestDetectCAR_WithLargeCARData() {
	// Test with CAR data larger than 512 bytes (detection buffer size)
	block := blocks.NewBlock([]byte("test data that is longer than normal"))
	rootCID := block.Cid()
	
	buf := new(bytes.Buffer)
	
	// Write header
	header := &carv1.CarHeader{
		Roots:   []cid.Cid{rootCID},
		Version: 1,
	}
	err := carv1.WriteHeader(header, buf)
	s.Require().NoError(err)
	
	// Write many blocks to exceed 512 bytes
	for i := 0; i < 100; i++ {
		block := blocks.NewBlock([]byte("padding to make the CAR larger than 512 bytes"))
		err := carv1.WriteBlock(buf, block.Cid(), block.RawData())
		s.Require().NoError(err)
	}
	
	reader := bytes.NewReader(buf.Bytes())
	format, detected := detectCAR(reader)
	
	// Should still detect correctly even with large data
	assert.True(s.T(), detected)
	assert.Equal(s.T(), FormatCAR, format)
}

// errorReader implements io.Reader and always returns an error
type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, e.err
}
