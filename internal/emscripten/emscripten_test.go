package emscripten

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental/wazerotest"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_callOnPanic(t *testing.T) {
	const exists = "f"
	var called bool
	f := wazerotest.NewFunction(func(context.Context, api.Module) { called = true })
	f.ExportNames = []string{exists}
	m := wazerotest.NewModule(nil, f)
	t.Run("exists", func(t *testing.T) {
		callOrPanic(context.Background(), m, exists, nil)
		require.True(t, called)
	})
	t.Run("not exist", func(t *testing.T) {
		err := require.CapturePanic(func() { callOrPanic(context.Background(), m, "not-exist", nil) })
		require.EqualError(t, err, "not-exist not exported")
	})
}
