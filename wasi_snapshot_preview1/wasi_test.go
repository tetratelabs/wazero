package wasi_snapshot_preview1

import (
	"bytes"
	"context"
	_ "embed"
	"io"
	"math/rand"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	. "github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

const seed = int64(42) // fixed seed value

var deterministicRandomSource = func() io.Reader {
	return rand.New(rand.NewSource(seed))
}

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

const testMemoryPageSize = 1

// maskMemory sets the first memory in the store to '?' * size, so tests can see what's written.
func maskMemory(t *testing.T, ctx context.Context, mod api.Module, size int) {
	for i := uint32(0); i < uint32(size); i++ {
		require.True(t, mod.Memory().WriteByte(ctx, i, '?'))
	}
}

func requireModule(t *testing.T, config wazero.ModuleConfig) (api.Module, api.Closer, *bytes.Buffer) {
	var log bytes.Buffer

	// Set context to one that has an experimental listener
	ctx := context.WithValue(testCtx, FunctionListenerFactoryKey{}, logging.NewLoggingListenerFactory(&log))

	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())

	compiled, err := (&builder{r}).moduleBuilder().
		ExportMemoryWithMax("memory", 1, 1).
		Compile(ctx, wazero.NewCompileConfig())
	require.NoError(t, err)

	mod, err := r.InstantiateModule(ctx, compiled, config)
	require.NoError(t, err)
	return mod, r, &log
}

// requireErrnoNosys ensures a call of the given function returns errno. The log
// message returned can verify the output is wasm `-->` or a host `==>`
// function.
func requireErrnoNosys(t *testing.T, funcName string, params ...uint64) string {
	var log bytes.Buffer

	// Set context to one that has an experimental listener
	ctx := context.WithValue(testCtx, FunctionListenerFactoryKey{}, logging.NewLoggingListenerFactory(&log))

	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
	defer r.Close(ctx)

	mod, err := Instantiate(ctx, r)
	require.NoError(t, err)

	requireErrno(t, ErrnoNosys, mod, funcName, params...)
	return "\n" + log.String()
}

func requireErrno(t *testing.T, expectedErrno Errno, mod api.Closer, funcName string, params ...uint64) {
	results, err := mod.(api.Module).ExportedFunction(funcName).Call(testCtx, params...)
	require.NoError(t, err)
	errno := Errno(results[0])
	require.Equal(t, expectedErrno, errno, ErrnoName(errno))
}
