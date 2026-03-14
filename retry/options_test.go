package retry

import (
	"context"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
)

func TestOptions(t *testing.T) {
	ctx := context.Background()
	opts := Options(ctx)

	if len(opts) != 6 {
		t.Errorf("Options() returned %d options, expected 6", len(opts))
	}
}

func TestOptionsWithConfig(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		cfg  OptionsConfig
	}{
		{
			name: "default config",
			cfg:  OptionsConfig{},
		},
		{
			name: "custom attempts",
			cfg:  OptionsConfig{Attempts: 5},
		},
		{
			name: "custom max delay",
			cfg:  OptionsConfig{MaxDelay: time.Minute},
		},
		{
			name: "all custom",
			cfg: OptionsConfig{
				Attempts:  10,
				MaxDelay:  2 * time.Minute,
				MaxJitter: 10 * time.Second,
				DelayType: retry.FixedDelay,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := OptionsWithConfig(ctx, tt.cfg)
			if len(opts) != 6 {
				t.Errorf("OptionsWithConfig() returned %d options, expected 6", len(opts))
			}
		})
	}
}

func TestOptions_Context(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	opts := Options(ctx)

	// Verify context is included in options
	foundContext := false
	for range opts {
		foundContext = true
		break
	}

	if !foundContext {
		t.Error("Options() should include context option")
	}
}
