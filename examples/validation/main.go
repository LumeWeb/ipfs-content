// Package main demonstrates path and component validation.
//
// This example shows how to validate archive paths to prevent zip-slip attacks
// and validate component dependencies.
package main

import (
	"fmt"
	"log"

	"go.lumeweb.com/ipfs-content/validation"
)

func main() {
	// Example 1: Archive path validation
	fmt.Println("Example 1: Archive Path Validation")

	testPaths := []string{
		"safe/inside/path.txt",
		"../escape.txt",            // Malicious: path traversal
		"../../etc/passwd",         // Malicious: path traversal
		"./current-dir.txt",        // Malicious: dot relative path
		"//absolute/path.txt",      // Malicious: absolute path
		"back\\slash.txt",          // Malicious: Windows path separator
		"safe.txt",
		"also/ok.txt",
	}

	for _, path := range testPaths {
		err := validation.ValidateArchivePath(path)
		if err != nil {
			fmt.Printf("  REJECTED: %s - %v\n", path, err)
		} else {
			fmt.Printf("  ACCEPTED: %s\n", path)
		}
	}

	// Example 2: Component validation with fluent API
	fmt.Println("\nExample 2: Component Validation")

	// Define some test components
	dagService := "mock DAG service"
	blockstore := "mock blockstore"

	validator := validation.NewComponentValidator()

	// Validate required components
	err := validator.ValidateRequired(
		validation.Component{Name: "DAGService", Value: dagService},
		validation.Component{Name: "Blockstore", Value: blockstore},
	)

	if err != nil {
		log.Fatalf("Component validation failed: %v", err)
	}
	fmt.Println("  All required components are present")

	// Example 3: Nil validation
	fmt.Println("\nExample 3: Nil Validation")

	var nilValue interface{} = nil
	var validValue interface{} = "value"

	err = validator.NotNil("nilValue", nilValue)
	if err != nil {
		fmt.Printf("  Correctly detected nil value: %v\n", err)
	}

	err = validator.NotNil("validValue", validValue)
	if err != nil {
		log.Fatalf("Unexpected error for valid value: %v", err)
	}
	fmt.Println("  Valid value check passed")

	// Example 4: AllNotNil helper
	fmt.Println("\nExample 4: AllNotNil Helper")

	value1 := "data1"
	value2 := "data2"

	err = validator.AllNotNil("value1", value1, "value2", value2)
	if err != nil {
		log.Fatalf("AllNotNil failed: %v", err)
	}
	fmt.Println("  All values are non-nil")

	err = validator.AllNotNil("value1", value1, "nilValue", nilValue)
	if err != nil {
		fmt.Printf("  Correctly detected nil value: %v\n", err)
	}

	fmt.Println("\nDone!")
}
