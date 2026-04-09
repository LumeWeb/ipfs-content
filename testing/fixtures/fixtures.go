// Package fixtures provides centralized test fixture discovery and access.
//
// This package provides Go utilities for finding test fixtures by leveraging
// Go's module resolution system. Since the Go compiler has already resolved
// where this package is (either in mod cache, vendor, or local), we can simply
// navigate from this file's location to find the fixtures directory.
//
// Usage:
//
//	fixturesDir, err := fixtures.FindFixturesDir()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Or get a specific file
//	libSh, err := fixtures.GetLibSh()
package fixtures

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	// Side-effect import: ensures internal/testing/fixtures are vendored
	// when this package is imported
	_ "go.lumeweb.com/ipfs-content/internal/testing/fixtures"

	// Additional side-effect import: ensures go-car/v2 dependencies are properly vendored
	_ "github.com/ipld/go-car/v2/blockstore"
)

const (
	// FixturesRelPath is the relative path from this file to fixtures directory
	FixturesRelPath = "../../internal/testing/fixtures"
)

// FindFixturesDir finds the ipfs-content fixtures directory.
//
// Since this package is imported from the consuming project, Go has already
// resolved the module location (either in GOPATH/mod, vendor, or local).
// We simply navigate from this file's location to find fixtures.
func FindFixturesDir() (string, error) {
	// Get the path to this file
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to get caller information")
	}

	// Navigate from this file to the fixtures directory
	fixturesDir := filepath.Join(filepath.Dir(filename), FixturesRelPath)

	// Resolve to absolute path
	absPath, err := filepath.Abs(fixturesDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Verify the directory exists
	if _, err := os.Stat(absPath); err != nil {
		return "", fmt.Errorf("fixtures directory not found at %s: %w", absPath, err)
	}

	return absPath, nil
}

// GetFixturesDir is a convenience wrapper that panics on error
func GetFixturesDir() string {
	dir, err := FindFixturesDir()
	if err != nil {
		panic(err)
	}
	return dir
}

// GetLibSh returns the path to lib.sh
func GetLibSh() (string, error) {
	dir, err := FindFixturesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lib.sh"), nil
}

// GetDataDir returns the path to the data directory
func GetDataDir() (string, error) {
	dir, err := FindFixturesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data"), nil
}

// GetCarsDir returns the path to the cars directory
func GetCarsDir() (string, error) {
	dir, err := FindFixturesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cars"), nil
}

// GetGenerateBlockScript returns the path to generate_block.sh
func GetGenerateBlockScript() (string, error) {
	dir, err := FindFixturesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "generate_block.sh"), nil
}

// GetGenerateCarScript returns the path to generate_car.sh
func GetGenerateCarScript() (string, error) {
	dir, err := FindFixturesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "generate_car.sh"), nil
}
