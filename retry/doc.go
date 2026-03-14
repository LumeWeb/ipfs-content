// Package retry provides configurable retry logic with backoff strategies.
//
// This package wraps github.com/avast/retry-go/v4 to provide sensible defaults
// for IPFS content operations, including exponential backoff with jitter to
// handle transient failures.
//
// Default_RETRY configuration: 3 attempts, exponential backoff with 5s max jitter,
// 30s max delay. Custom configurations allow control over attempts, delays,
// and jitter timing.
//
// Basic Usage:
//
//	import "go.lumeweb.com/ipfs-content/retry"
//	import "github.com/avast/retry-go/v4"
//
//	// Default retry (3 attempts, exponential backoff with 5s max jitter, 30s max delay)
//	err := retry.Do(
//	    func() error { return someOperation() },
//	    retry.Options(ctx)...,
//	)
//
//	// Custom retry configuration
//	cfg := retry.OptionsConfig{
//	    Attempts:  5,
//	    MaxDelay:  time.Minute,
//	    MaxJitter: 10 * time.Second,
//	}
//	err := retry.Do(
//	    func() error { return someOperation() },
//	    retry.OptionsWithConfig(ctx, cfg)...,
//	)
package retry
