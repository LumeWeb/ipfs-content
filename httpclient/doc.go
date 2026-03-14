// Package httpclient provides HTTP client factory for creating configured clients.
//
// This package provides a factory for creating HTTP clients with sensible defaults
// and configurable options including timeout, max retries, and keep-alive settings.
//
// The factory uses the functional options pattern for flexible configuration,
// and provides a generic WithDefaultClient helper for creating service clients.
//
// Basic Usage:
//
//	import "go.lumeweb.com/ipfs-content/httpclient"
//
//	// Create client with custom options using functional options pattern
//	client := httpclient.CreateDefaultClient(
//	    func(opts *httpclient.FactoryOptions) {
//	        opts.WithTimeout(60 * time.Second)
//	        opts.WithKeepAlives(true)
//	        opts.WithMaxRetries(5)
//	    },
//	)
//
//	// Service client factory pattern
//	type MyService interface {
//	    DoSomething(ctx context.Context) error
//	}
//
//	createService := httpclient.WithDefaultClient(func(baseURL string, client *http.Client) (MyService, error) {
//	    return NewMyServiceClient(baseURL, client), nil
//	})
//
//	svc, err := createService("https://api.example.com")
package httpclient
