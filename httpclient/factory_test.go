package httpclient

import (
	"net/http"
	"testing"
	"time"
)

func TestDefaultFactoryOptions(t *testing.T) {
	opts := DefaultFactoryOptions()
	if opts == nil {
		t.Fatal("DefaultFactoryOptions() returned nil")
	}

	if opts.Timeout != DefaultTimeout {
		t.Errorf("Expected timeout %v, got %v", DefaultTimeout, opts.Timeout)
	}
	if opts.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", opts.MaxRetries)
	}
	if !opts.KeepAlives {
		t.Error("Expected KeepAlives to be true by default")
	}

}

func TestCreateDefaultClient(t *testing.T) {
	client := CreateDefaultClient()
	if client == nil {
		t.Fatal("CreateDefaultClient() returned nil")
	}

	if client.Timeout != DefaultTimeout {
		t.Errorf("Expected client timeout %v, got %v", DefaultTimeout, client.Timeout)
	}

	if client.Transport == nil {
		t.Fatal("Expected client.Transport to be set")
	}
}

func TestFactoryOptions(t *testing.T) {
	opts := DefaultFactoryOptions().
		WithTimeout(60 * time.Second).
		WithKeepAlives(false).
		WithMaxRetries(5)

	if opts.Timeout != 60*time.Second {
		t.Errorf("Expected timeout 60s, got %v", opts.Timeout)
	}
	if opts.KeepAlives {
		t.Error("Expected KeepAlives to be false")
	}
	if opts.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries 5, got %d", opts.MaxRetries)
	}
}

func TestCreateDefaultClientWithOptions(t *testing.T) {
	customTimeout := 90 * time.Second
	client := CreateDefaultClient(func(o *FactoryOptions) {
		o.WithTimeout(customTimeout)
	})

	if client == nil {
		t.Fatal("CreateDefaultClient() with options returned nil")
	}
	if client.Timeout != customTimeout {
		t.Errorf("Expected timeout %v, got %v", customTimeout, client.Timeout)
	}
}

func TestWithDefaultClient(t *testing.T) {
	type MockService struct {
		Endpoint string
	}

	createFactory := func(baseURL string, client *http.Client) (MockService, error) {
		_ = client // Use client to avoid unused warning
		return MockService{Endpoint: baseURL}, nil
	}

	factory := WithDefaultClient(createFactory)
	svc, err := factory("https://example.com")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if svc.Endpoint != "https://example.com" {
		t.Errorf("Expected endpoint https://example.com, got %s", svc.Endpoint)
	}
}

func TestWithCustomClient(t *testing.T) {
	type MockService struct {
		BaseURL string
	}

	createFactory := func(baseURL string, client *http.Client) (MockService, error) {
		if client == nil {
			t.Fatal("Expected client to be non-nil")
		}
		return MockService{BaseURL: baseURL}, nil
	}

	factory := WithCustomClient(createFactory)
	customClient := &http.Client{Timeout: 10 * time.Second}
	svc, err := factory("https://example.com", customClient)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if svc.BaseURL != "https://example.com" {
		t.Errorf("Expected base URL https://example.com, got %s", svc.BaseURL)
	}
}
