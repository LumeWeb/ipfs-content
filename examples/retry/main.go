// Package main demonstrates retry logic usage.
//
// This example shows how to use configurable retry logic with backoff strategies
// for handling transient failures.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	extern_retry "github.com/avast/retry-go/v4"
	hretry "go.lumeweb.com/ipfs-content/retry"
)

func main() {
	ctx := context.Background()

	// Example 1: Default retry configuration
	fmt.Println("Example 1: Default Retry (3 attempts)")
	attemptCount := 0

	err := extern_retry.Do(
		func() error {
			attemptCount++
			fmt.Printf("  Attempt %d\n", attemptCount)
			if attemptCount < 3 {
				return fmt.Errorf("transient error")
			}
			fmt.Println("  Success!")
			return nil
		},
		hretry.Options(ctx)...,
	)

	if err != nil {
		log.Fatalf("Retry failed: %v", err)
	}
	fmt.Printf("  Total attempts: %d\n\n", attemptCount)

	// Example 2: Custom retry configuration
	fmt.Println("Example 2: Custom Retry (5 attempts)")
	attemptCount2 := 0

	cfg := hretry.OptionsConfig{
		Attempts:  5,
		MaxDelay:  2 * time.Minute,
		MaxJitter: 10 * time.Second,
	}

	err = extern_retry.Do(
		func() error {
			attemptCount2++
			fmt.Printf("  Attempt %d\n", attemptCount2)
			if attemptCount2 < 3 {
				return fmt.Errorf("transient error")
			}
			fmt.Println("  Success!")
			return nil
		},
		hretry.OptionsWithConfig(ctx, cfg)...,
	)

	if err != nil {
		log.Fatalf("Retry failed: %v", err)
	}
	fmt.Printf("  Total attempts: %d\n\n", attemptCount2)

	// Example 3: Retry with specific error handling
	fmt.Println("Example 3: Retry with Error Handling")
	attemptCount3 := 0

	err = extern_retry.Do(
		func() error {
			attemptCount3++
			fmt.Printf("  Attempt %d\n", attemptCount3)

			// Simulate different errors
			switch attemptCount3 {
			case 1:
				return fmt.Errorf("request timeout")
			case 2:
				return fmt.Errorf("connection refused")
			default:
				fmt.Println("  Success!")
				return nil
			}
		},
		hretry.Options(ctx)...,
	)

	if err != nil {
		log.Fatalf("Retry failed: %v", err)
	}
	fmt.Printf("  Total attempts: %d\n\n", attemptCount3)

	fmt.Println("Done!")
}
