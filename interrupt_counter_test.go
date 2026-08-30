package wazero_test

import (
	"context"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestInterruptCheckInterval_ContextTimeout verifies that an infinite loop is
// terminated by context timeout when the interrupt check interval is configured.
func TestInterruptCheckInterval_ContextTimeout(t *testing.T) {
	tests := []struct {
		name     string
		interval uint64
	}{
		{name: "interval=0 (every iteration)", interval: 0},
		{name: "interval=1", interval: 1},
		{name: "interval=16", interval: 16},
		{name: "interval=32", interval: 32},
		{name: "interval=64", interval: 64},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := experimental.WithInterruptCheckInterval(context.Background(), tc.interval)

			config := wazero.NewRuntimeConfig().
				WithCloseOnContextDone(true)

			r := wazero.NewRuntimeWithConfig(ctx, config)
			defer r.Close(ctx)

			moduleInstance, err := r.InstantiateWithConfig(ctx, infiniteLoopWasm,
				wazero.NewModuleConfig().WithName("test_module"))
			require.NoError(t, err)

			infiniteLoop := moduleInstance.ExportedFunction("infinite_loop")

			timeoutCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer cancel()

			_, err = infiniteLoop.Call(timeoutCtx)
			require.Error(t, err)
			require.Contains(t, err.Error(), "context deadline exceeded")
		})
	}
}

// TestInterruptCheckInterval_ContextCancel verifies that an infinite loop is
// terminated by context cancellation when the interrupt check interval is configured.
func TestInterruptCheckInterval_ContextCancel(t *testing.T) {
	ctx := experimental.WithInterruptCheckInterval(context.Background(), 32)

	config := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true)

	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer r.Close(ctx)

	moduleInstance, err := r.InstantiateWithConfig(ctx, infiniteLoopWasm,
		wazero.NewModuleConfig().WithName("test_module"))
	require.NoError(t, err)

	infiniteLoop := moduleInstance.ExportedFunction("infinite_loop")

	cancelCtx, cancel := context.WithCancel(ctx)
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	_, err = infiniteLoop.Call(cancelCtx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "context canceled")
}

// TestInterruptCheckInterval_ModuleClose verifies that an infinite loop is
// terminated by explicit module close when the interrupt check interval is configured.
func TestInterruptCheckInterval_ModuleClose(t *testing.T) {
	ctx := experimental.WithInterruptCheckInterval(context.Background(), 32)

	config := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true)

	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer r.Close(ctx)

	moduleInstance, err := r.InstantiateWithConfig(ctx, infiniteLoopWasm,
		wazero.NewModuleConfig().WithName("test_module"))
	require.NoError(t, err)

	infiniteLoop := moduleInstance.ExportedFunction("infinite_loop")

	go func() {
		time.Sleep(500 * time.Millisecond)
		_ = moduleInstance.CloseWithExitCode(ctx, 1)
	}()

	_, err = infiniteLoop.Call(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exit_code(1)")
}

// TestInterruptCheckInterval_NonPowerOfTwoPanics verifies that a non-power-of-two
// interval panics.
func TestInterruptCheckInterval_NonPowerOfTwoPanics(t *testing.T) {
	err := require.CapturePanic(func() {
		experimental.WithInterruptCheckInterval(context.Background(), 3)
	})
	require.EqualError(t, err, "interruptCheckInterval invalid: 3 is not zero or a power of two")
}
