package format

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormat_String(t *testing.T) {
	tests := []struct {
		name     string
		format   Format
		expected string
	}{
		{"CAR format", FormatCAR, "car"},
		{"File format", FormatFile, "file"},
		{"ZIP format", FormatZIP, "zip"},
		{"RAR format", FormatRAR, "rar"},
		{"TAR format", FormatTAR, "tar"},
		{"TAR_GZ format", FormatTAR_GZ, "tar.gz"},
		{"TAR_BZ2 format", FormatTAR_BZ2, "tar.bz2"},
		{"7Z format", Format7Z, "7z"},
		{"Unknown format", FormatUnknown, "unknown"},
		{"Invalid format", Format(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.format.String()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Format
	}{
		{"parse car", "car", FormatCAR},
		{"parse file", "file", FormatFile},
		{"parse zip", "zip", FormatZIP},
		{"parse rar", "rar", FormatRAR},
		{"parse tar", "tar", FormatTAR},
		{"parse tar.gz", "tar.gz", FormatTAR_GZ},
		{"parse tar.bz2", "tar.bz2", FormatTAR_BZ2},
		{"parse 7z", "7z", Format7Z},
		{"parse unknown", "unknown", FormatUnknown},
		{"parse empty string", "", FormatUnknown},
		{"parse invalid", "invalid_format", FormatUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseFormat(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestFormat_IsUploadFormat(t *testing.T) {
	tests := []struct {
		name     string
		format   Format
		expected bool
	}{
		{"CAR is upload format", FormatCAR, true},
		{"File is not upload format", FormatFile, false},
		{"ZIP is not upload format", FormatZIP, false},
		{"RAR is not upload format", FormatRAR, false},
		{"TAR is not upload format", FormatTAR, false},
		{"TAR_GZ is not upload format", FormatTAR_GZ, false},
		{"TAR_BZ2 is not upload format", FormatTAR_BZ2, false},
		{"7Z is not upload format", Format7Z, false},
		{"Unknown is not upload format", FormatUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.format.IsUploadFormat()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestFormat_IsArchiveFormat(t *testing.T) {
	tests := []struct {
		name     string
		format   Format
		expected bool
	}{
		{"CAR is not archive format", FormatCAR, false},
		{"File is not archive format", FormatFile, false},
		{"ZIP is archive format", FormatZIP, true},
		{"RAR is archive format", FormatRAR, true},
		{"TAR is archive format", FormatTAR, true},
		{"TAR_GZ is archive format", FormatTAR_GZ, true},
		{"TAR_BZ2 is archive format", FormatTAR_BZ2, true},
		{"7Z is archive format", Format7Z, true},
		{"Unknown is not archive format", FormatUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.format.IsArchiveFormat()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestFormat_MarshalText(t *testing.T) {
	tests := []struct {
		name     string
		format   Format
		expected []byte
	}{
		{"marshal CAR", FormatCAR, []byte("car")},
		{"marshal File", FormatFile, []byte("file")},
		{"marshal ZIP", FormatZIP, []byte("zip")},
		{"marshal RAR", FormatRAR, []byte("rar")},
		{"marshal TAR", FormatTAR, []byte("tar")},
		{"marshal TAR_GZ", FormatTAR_GZ, []byte("tar.gz")},
		{"marshal TAR_BZ2", FormatTAR_BZ2, []byte("tar.bz2")},
		{"marshal 7Z", Format7Z, []byte("7z")},
		{"marshal Unknown", FormatUnknown, []byte("unknown")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.format.MarshalText()
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestFormat_UnmarshalText(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		expected   Format
		expectErr  bool
	}{
		{"unmarshal car", []byte("car"), FormatCAR, false},
		{"unmarshal file", []byte("file"), FormatFile, false},
		{"unmarshal zip", []byte("zip"), FormatZIP, false},
		{"unmarshal rar", []byte("rar"), FormatRAR, false},
		{"unmarshal tar", []byte("tar"), FormatTAR, false},
		{"unmarshal tar.gz", []byte("tar.gz"), FormatTAR_GZ, false},
		{"unmarshal tar.bz2", []byte("tar.bz2"), FormatTAR_BZ2, false},
		{"unmarshal 7z", []byte("7z"), Format7Z, false},
		{"unmarshal unknown", []byte("unknown"), FormatUnknown, true},
		{"unmarshal empty", []byte(""), FormatUnknown, true},
		{"unmarshal invalid", []byte("invalid_format"), FormatUnknown, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f Format
			err := f.UnmarshalText(tt.input)

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, f)
			}
		})
	}
}

func TestFormat_RoundTrip(t *testing.T) {
	formats := []Format{
		FormatCAR, FormatFile, FormatZIP, FormatRAR,
		FormatTAR, FormatTAR_GZ, FormatTAR_BZ2, Format7Z,
	}

	for _, f := range formats {
		t.Run(f.String(), func(t *testing.T) {
			// Marshal
			data, err := f.MarshalText()
			require.NoError(t, err)

			// Unmarshal
			var result Format
			err = result.UnmarshalText(data)
			require.NoError(t, err)

			// Verify we got the same format back
			require.Equal(t, f, result)
		})
	}
}
