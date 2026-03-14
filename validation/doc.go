// Package validation provides path and component validation utilities.
//
// This package provides security-focused validation for archive paths and component
// validation for checking required dependencies and nil values.
//
// Path Validation:
// ValidateArchivePath prevents zip-slip attacks by validating file paths extracted
// from archives. It checks for suspicious patterns (\, //, ./) and ensures paths
// do not escape the archive root.
//
// Component Validation:
// Provides a fluent API for validating configuration components, checking for nil
// values and ensuring required dependencies are present.
//
// Basic Usage:
//
//	import "go.lumeweb.com/ipfs-content/validation"
//
//	// Validate archive paths to prevent zip-slip attacks
//	err := validation.ValidateArchivePath("path/within/archive")
//	if err != nil {
//	    // Reject malicious path
//	}
//
//	// Check required components
//	validator := validation.NewComponentValidator()
//	err = validator.ValidateRequired(
//	    validation.Component{Name: "DAGService", Value: dagService},
//	    validation.Component{Name: "Blockstore", Value: blockstore},
//	)
//
//	// Or use helper methods
//	err := validator.NotNil("DAGService", dagService)
//	err := validator.AllNotNil("DAGService", dagService, "Blockstore", blockstore)
package validation
