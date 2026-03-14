package retry

import (
	"context"
	"time"

	"github.com/avast/retry-go/v4"
)

// Options returns standard retry configuration for API calls.
//
// It provides a sensible default configuration with:
// - 3 attempts
// - Context-aware cancellation
// - Backoff delay with jitter
// - Maximum jitter of 5 seconds
// - Maximum delay of 30 seconds
//
// Example:
//
//	err := retry.Do(
//	    func() error {
//	        // Your operation here
//	        return someOperation()
//	    },
//	    retry.Options(ctx)...,
//	)
func Options(ctx context.Context) []retry.Option {
	return []retry.Option{
		retry.Attempts(3),
		retry.LastErrorOnly(true),
		retry.Context(ctx),
		retry.DelayType(retry.BackOffDelay),
		retry.MaxJitter(5 * time.Second),
		retry.MaxDelay(30 * time.Second),
	}
}

// OptionsWithConfig returns retry configuration with custom settings.
//
// Example:
//
//	opts := retry.OptionsConfig{Attempts: 5, MaxDelay: time.Minute}
//	err := retry.Do(op, retry.OptionsWithConfig(ctx, opts)...)
//
type OptionsConfig struct {
	Attempts  uint
	MaxDelay  time.Duration
	MaxJitter time.Duration
	DelayType retry.DelayTypeFunc
}

// OptionsWithConfig creates retry options with custom configuration.
func OptionsWithConfig(ctx context.Context, cfg OptionsConfig) []retry.Option {
	dopts := Options(ctx)

	// Override defaults with provided config
	if cfg.Attempts > 0 {
		dopts[0] = retry.Attempts(cfg.Attempts)
	}
	if cfg.MaxDelay > 0 {
		dopts[5] = retry.MaxDelay(cfg.MaxDelay)
	}
	if cfg.MaxJitter > 0 {
		dopts[4] = retry.MaxJitter(cfg.MaxJitter)
	}
	// DelayType requires special handling - can't override specific index
	if cfg.DelayType != nil {
		dopts[3] = retry.DelayType(cfg.DelayType)
	}

	return dopts
}
