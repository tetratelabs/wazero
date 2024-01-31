package opt_test

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/opt"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestUseOptimizingCompiler(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	c := opt.NewRuntimeConfigOptimizingCompiler()
	r := wazero.NewRuntimeWithConfig(context.Background(), c)
	require.NoError(t, r.Close(context.Background()))
}
