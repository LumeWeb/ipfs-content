// Package fixtures provides a side-effect import to ensure internal testing fixtures
// are included when vendoring.
//
// Go's vendor tooling won't include internal packages unless they are referenced by
// a public package. By importing this package in test files, the internal/testing
// directory will be included in vendor output.
//
// Usage in test files:
//
//	import _ "go.lumeweb.com/ipfs-content/testing/fixtures"
package fixtures

import (
	_ "go.lumeweb.com/ipfs-content/internal/testing/fixtures"
)

// This file intentionally empty - the purpose is only the side-effect import above.
