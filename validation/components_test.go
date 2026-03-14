package validation

import (
	"testing"
)

func TestValidateRequired(t *testing.T) {
	cv := NewComponentValidator()

	type TestType struct{}

	tests := []struct {
		name     string
		required []Component
		wantErr  bool
	}{
		{
			name:     "all valid",
			required: []Component{{Name: "Service", Value: &TestType{}}},
			wantErr:  false,
		},
		{
			name:     "nil component",
			required: []Component{{Name: "Service", Value: nil}},
			wantErr:  true,
		},
		{
			name: "multiple components all valid",
			required: []Component{
				{Name: "Service1", Value: &TestType{}},
				{Name: "Service2", Value: &TestType{}},
			},
			wantErr: false,
		},
		{
			name: "multiple components one nil",
			required: []Component{
				{Name: "Service1", Value: &TestType{}},
				{Name: "Service2", Value: nil},
			},
			wantErr: true,
		},
		{
			name:     "empty components",
			required: []Component{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cv.ValidateRequired(tt.required...)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRequired() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNotNil(t *testing.T) {
	cv := NewComponentValidator()

	type TestType struct{}

	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{
			name:  "valid value",
			value: &TestType{},
			want:  false,
		},
		{
			name:  "nil value",
			value: nil,
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cv.NotNil("TestComponent", tt.value)
			if (err != nil) != tt.want {
				t.Errorf("NotNil() error = %v, want error %v", err, tt.want)
			}
		})
	}
}

func TestAllNotNil(t *testing.T) {
	cv := NewComponentValidator()

	type TestType struct{}

	tests := []struct {
		name          string
		nameValuePairs []any
		wantErr       bool
	}{
		{
			name:          "all valid",
			nameValuePairs: []any{"Service1", &TestType{}, "Service2", &TestType{}},
			wantErr:       false,
		},
		{
			name:          "one nil",
			nameValuePairs: []any{"Service1", &TestType{}, "Service2", nil},
			wantErr:       true,
		},
		{
			name:          "odd number of arguments",
			nameValuePairs: []any{"Service1", &TestType{}, "Service2"},
			wantErr:       true,
		},
		{
			name:          "non-string name",
			nameValuePairs: []any{123456, &TestType{}},
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cv.AllNotNil(tt.nameValuePairs...)
			if (err != nil) != tt.wantErr {
				t.Errorf("AllNotNil() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewComponentValidator(t *testing.T) {
	cv := NewComponentValidator()
	if cv == nil {
		t.Error("NewComponentValidator() returned nil")
	}
}

func ExampleComponentValidator_ValidateRequired() {
	cv := NewComponentValidator()

	type Service struct{}
	service := &Service{}

	err := cv.ValidateRequired(
		Component{Name: "Service", Value: service},
	)
	if err != nil {
		panic(err)
	}
	// components validated successfully
}

func ExampleComponentValidator_NotNil() {
	cv := NewComponentValidator()

	type Service struct{}
	service := &Service{}

	err := cv.NotNil("Service", service)
	if err != nil {
		panic(err)
	}
	// component is not nil
}

func ExampleComponentValidator_AllNotNil() {
	cv := NewComponentValidator()

	type Service struct{}
	service := &Service{}

	err := cv.AllNotNil("Service", service)
	if err != nil {
		panic(err)
	}
	// component is not nil
}

func TestComponentValidator_Errors(t *testing.T) {
	cv := NewComponentValidator()

	t.Run("ValidateRequired returns informative error", func(t *testing.T) {
		err := cv.ValidateRequired(Component{Name: "TestService", Value: nil})
		if err == nil {
			t.Fatal("Expected error")
		}
		expectedMsg := "TestService is required"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error message %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("NotNil returns informative error", func(t *testing.T) {
		err := cv.NotNil("TestComponent", nil)
		if err == nil {
			t.Fatal("Expected error")
		}
		expectedMsg := "TestComponent is required"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error message %q, got %q", expectedMsg, err.Error())
		}
	})
}
