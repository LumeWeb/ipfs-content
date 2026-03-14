package httpclient

import (
	"net/http"
	"time"
)

// DefaultTimeout is the default timeout for HTTP clients.
const DefaultTimeout = 30 * time.Second

// FactoryOptions configures HTTP client creation.
type FactoryOptions struct {
	Timeout       time.Duration
	RetryOn       []int
	MaxRetries    int
	KeepAlives    bool
	DisableCookie *http.CookieJar
}

// DefaultFactoryOptions returns sensible defaults for HTTP client configuration.
func DefaultFactoryOptions() *FactoryOptions {
	return &FactoryOptions{
		Timeout:    DefaultTimeout,
		MaxRetries: 3,
		KeepAlives: true,
	}
}

// WithTimeout sets the timeout for HTTP requests.
func (o *FactoryOptions) WithTimeout(timeout time.Duration) *FactoryOptions {
	o.Timeout = timeout
	return o
}

// WithKeepAlives enables or disables HTTP keep-alives.
func (o *FactoryOptions) WithKeepAlives(enabled bool) *FactoryOptions {
	o.KeepAlives = enabled
	return o
}

// WithMaxRetries sets the maximum number of retries.
func (o *FactoryOptions) WithMaxRetries(maxRetries int) *FactoryOptions {
	o.MaxRetries = maxRetries
	return o
}

// CreateDefaultClient creates an HTTP client with default options.
func CreateDefaultClient(opts ...func(*FactoryOptions)) *http.Client {
	options := DefaultFactoryOptions()

	for _, opt := range opts {
		opt(options)
	}

	return &http.Client{
		Timeout: options.Timeout,
		Transport: &http.Transport{
			DisableKeepAlives: !options.KeepAlives,
			MaxIdleConns:     100,
			IdleConnTimeout:  90 * time.Second,
			MaxConnsPerHost:  10,
		},
	}
}

// ClientFunc is a factory function type for creating service clients with an optional HTTP client.
// This is useful for openapi-generated clients and similar patterns.
//
// Example:
//
//	serviceFactory := func(baseURL string, client *http.Client) (MyService, error) {
//	    opts := []client.Option{client.WithBaseURL(baseURL)}
//	    if client != nil {
//	        opts = append(opts, client.WithHTTPClient(client))
//	    }
//	    return client.NewClientWithResponses("", opts...)
//	}
//
//	service, err := serviceFactory("https://api.example.com", nil)
type ClientFunc[T any] func(baseURL string, httpClient *http.Client) (T, error)

// WithDefaultClient creates an HTTP client factory function that creates services with a default HTTP client.
//
// Example:
//
//	createService := httpClient.WithDefaultClient(func(baseURL string, client *http.Client) (MyService, error) {
//	    return NewMyServiceClient(baseURL, client)
//	})
//
//	svc, err := createService("https://api.example.com")
func WithDefaultClient[T any](factory ClientFunc[T]) func(baseURL string) (T, error) {
	return func(baseURL string) (T, error) {
		return factory(baseURL, CreateDefaultClient())
	}
}

// WithCustomClient creates an HTTP client factory function that allows passing a custom HTTP client.
//
// Example:
//
//	customClient := &http.Client{Timeout: 60 * time.Second}
//	createService := httpClient.WithCustomClient(func(baseURL string, client *http.Client) (MyService, error) {
//	    return NewMyServiceClient(baseURL, client)
//	})
//
//	svc, err := createService("https://api.example.com", customClient)
func WithCustomClient[T any](factory ClientFunc[T]) func(baseURL string, httpClient *http.Client) (T, error) {
	return func(baseURL string, client *http.Client) (T, error) {
		return factory(baseURL, client)
	}
}
