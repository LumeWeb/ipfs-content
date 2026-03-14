package validation

import (
	"testing"
)

func TestValidateArchivePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid relative path",
			path:    "relative/path/to/file.txt",
			wantErr: false,
		},
		{
			name:    "valid simple path",
			path:    "file.txt",
			wantErr: false,
		},
		{
			name:    "path with backslash - invalid",
			path:    "path\\to\\file.txt",
			wantErr: true,
		},
		{
			name:    "path with double slash - invalid",
			path:    "path//to//file.txt",
			wantErr: true,
		},
		{
			name:    "path with ./ - invalid",
			path:    "./path/to/file.txt",
			wantErr: true,
		},
		{
			name:    "path traversal attempt - invalid",
			path:    "../malicious.txt",
			wantErr: true,
		},
		{
			name:    "aggressive path traversal - invalid",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "mixed traversal - invalid",
			path:    "folder/../../../secret",
			wantErr: true,
		},
		{
			name:    "absolute path - invalid",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "empty path - valid",
			path:    "",
			wantErr: false,
		},
		{
			name:    "nested directory structure",
			path:    "deepest/nested/directory/structure/file.txt",
			wantErr: false,
		},
		{
			name:    "complex but valid path",
			path:    "a/b/c/d/e/f.txt",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateArchivePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateArchivePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateArchivePath_SuspiciousPatterns(t *testing.T) {
	patterns := []string{
		"\\",  // Windows path separators
		"//",  // Double slashes
		"./", // Current directory references
	}

	for _, pattern := range patterns {
		t.Run("pattern_"+pattern, func(t *testing.T) {
			path := "test" + pattern + "file.txt"
			if err := ValidateArchivePath(path); err == nil {
				t.Errorf("ValidateArchivePath(%q) expected error for pattern %s", path, pattern)
			}
		})
	}
}
