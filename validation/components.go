package validation

import (
	"fmt"
)

// ComponentValidator provides validation utilities for service components.
// It helps ensure that required dependencies are properly initialized before use.
type ComponentValidator struct{}

// NewComponentValidator creates a new component validator.
func NewComponentValidator() *ComponentValidator {
	return &ComponentValidator{}
}

// Component describes a required service component.
type Component struct {
	Name  string
	Value any
}

// ValidateRequired checks that required components are present and non-nil.
//
// This is a generic validation function that can be used to ensure all required
// dependencies are initialized before a service is used.
//
// Example:
//
//	validator := validation.NewComponentValidator()
//	dagService, blockstore := ...
//	if err := validator.ValidateRequired(
//		validation.Component{Name: "DAGService", Value: dagService},
//		validation.Component{Name: "Blockstore", Value: blockstore},
//	); err != nil {
//	    return err
//	}
func (cv *ComponentValidator) ValidateRequired(required ...Component) error {
	for _, comp := range required {
		if comp.Value == nil {
			return fmt.Errorf("%s is required", comp.Name)
		}
	}
	return nil
}

// NotNil is a helper that creates a Component with a nil-safe check.
//
// Example:
//
//	if err := validator.NotNil("DAGService", dagService); err != nil {
//	    return err
//	}
func (cv *ComponentValidator) NotNil(name string, value any) error {
	if value == nil {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

// AllNotNil returns an error if any of the provided values are nil.
//
// Example:
//
//	if err := validator.AllNotNil(
//	    "DAGService", dagService,
//	    "Blockstore", blockstore,
//	); err != nil {
//	    return err
//	}
func (cv *ComponentValidator) AllNotNil(nameValuePairs ...any) error {
	if len(nameValuePairs)%2 != 0 {
		return fmt.Errorf("AllNotNil requires an even number of arguments (name, value pairs)")
	}

	for i := 0; i < len(nameValuePairs); i += 2 {
		name, ok := nameValuePairs[i].(string)
		if !ok {
			return fmt.Errorf("component name at index %d must be string", i)
		}

		value := nameValuePairs[i+1]
		if value == nil {
			return fmt.Errorf("%s is required", name)
		}
	}

	return nil
}
