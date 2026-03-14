// Package main demonstrates HTTP client factory usage.
//
// This example shows how to create configured HTTP clients and service clients
// using the factory pattern.
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"go.lumeweb.com/ipfs-content/httpclient"
)

func main() {
	// Example 1: Create client with default options
	fmt.Println("Example 1: Default HTTP Client")
	defaultClient := httpclient.CreateDefaultClient()

	resp, err := defaultClient.Get("https://httpbin.org/get")
	if err != nil {
		log.Fatalf("Failed to make request: %v", err)
	}
	resp.Body.Close()
	fmt.Printf("  Status: %s\n", resp.Status)

	// Example 2: Create client with custom options
	fmt.Println("\nExample 2: Custom HTTP Client")
	customClient := httpclient.CreateDefaultClient(
		func(opts *httpclient.FactoryOptions) {
			opts.WithTimeout(30 * time.Second)
			opts.WithKeepAlives(true)
			opts.WithMaxRetries(3)
		},
	)

	resp2, err := customClient.Get("https://httpbin.org/get")
	if err != nil {
		log.Fatalf("Failed to make request: %v", err)
	}
	resp2.Body.Close()
	fmt.Printf("  Status: %s\n", resp2.Status)

	// Example 3: Service client factory pattern
	fmt.Println("\nExample 3: Service Client Factory")

	// Create service using factory
	createService := httpclient.WithDefaultClient(func(baseURL string, client *http.Client) (myServiceImpl, error) {
		return NewMyServiceClient(baseURL, client), nil
	})

	svc, err := createService("https://httpbin.org")
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}

	fmt.Printf("  Created service for %s\n", svc.baseURL)

	fmt.Println("\nDone!")
}

// NewMyServiceClient creates a new service client
func NewMyServiceClient(baseURL string, client *http.Client) myServiceImpl {
	return myServiceImpl{
		client:  client,
		baseURL: baseURL,
	}
}

// myServiceImpl implements a service client
type myServiceImpl struct {
	client  *http.Client
	baseURL string
}
