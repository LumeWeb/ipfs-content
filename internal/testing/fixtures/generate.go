// Package fixtures provides test fixture generation utilities.
// This file contains go:generate directives to rebuild test fixtures.
//
// Run: go generate ./internal/testing/fixtures
package fixtures

//go:generate ./generate_car.sh
//go:generate ./generate_block.sh
//go:generate go run ../../testing/fixtures/cmd/invalid-car-generator/main.go
//go:generate go run ../../testing/fixtures/cmd/empty-car-generator/main.go
//go:generate go run ../../testing/fixtures/cmd/protobuf-generator/main.go
