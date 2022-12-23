package wasi_snapshot_preview1

import (
	"bytes"
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	. "github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/proxy"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

const testMemoryPageSize = 1

// exitOnStartUnstableWasm was generated by the following:
//
//	cd testdata; wat2wasm --debug-names exit_on_start_unstable.wat
//
//go:embed testdata/exit_on_start_unstable.wasm
var exitOnStartUnstableWasm []byte

func TestNewFunctionExporter(t *testing.T) {
	t.Run("export as wasi_unstable", func(t *testing.T) {
		r := wazero.NewRuntime(testCtx)
		defer r.Close(testCtx)

		// Instantiate the current WASI functions under the wasi_unstable
		// instead of wasi_snapshot_preview1.
		wasiBuilder := r.NewHostModuleBuilder("wasi_unstable")
		NewFunctionExporter().ExportFunctions(wasiBuilder)
		_, err := wasiBuilder.Instantiate(testCtx, r)
		require.NoError(t, err)

		// Instantiate our test binary, but using the old import names.
		_, err = r.InstantiateModuleFromBinary(testCtx, exitOnStartUnstableWasm)

		// Ensure the test binary worked. It should return exit code 2.
		require.Equal(t, uint32(2), err.(*sys.ExitError).ExitCode())
	})

	t.Run("override function", func(t *testing.T) {
		r := wazero.NewRuntime(testCtx)
		defer r.Close(testCtx)

		// Export the default WASI functions
		wasiBuilder := r.NewHostModuleBuilder(ModuleName)
		NewFunctionExporter().ExportFunctions(wasiBuilder)

		// Override proc_exit to prove the point that you can add or replace
		// functions like this.
		wasiBuilder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, mod api.Module, exitCode uint32) {
				require.Equal(t, uint32(2), exitCode)
				// ignore the code instead!
				mod.Close(ctx)
			}).Export("proc_exit")

		_, err := wasiBuilder.Instantiate(testCtx, r)
		require.NoError(t, err)

		// Instantiate our test binary which will use our modified WASI.
		_, err = r.InstantiateModuleFromBinary(testCtx, exitOnStartWasm)

		// Ensure the modified function was used!
		require.Zero(t, err.(*sys.ExitError).ExitCode())
	})
}

// maskMemory sets the first memory in the store to '?' * size, so tests can see what's written.
func maskMemory(t *testing.T, mod api.Module, size int) {
	for i := uint32(0); i < uint32(size); i++ {
		require.True(t, mod.Memory().WriteByte(i, '?'))
	}
}

func requireProxyModule(t *testing.T, config wazero.ModuleConfig) (api.Module, api.Closer, *bytes.Buffer) {
	var log bytes.Buffer

	// Set context to one that has an experimental listener
	ctx := context.WithValue(testCtx, FunctionListenerFactoryKey{}, proxy.NewLoggingListenerFactory(&log))

	r := wazero.NewRuntime(ctx)

	wasiModuleCompiled, err := (&builder{r}).hostModuleBuilder().Compile(ctx)
	require.NoError(t, err)

	_, err = r.InstantiateModule(ctx, wasiModuleCompiled, config)
	require.NoError(t, err)

	proxyBin := proxy.NewModuleBinary(ModuleName, wasiModuleCompiled)

	proxyCompiled, err := r.CompileModule(ctx, proxyBin)
	require.NoError(t, err)

	mod, err := r.InstantiateModule(ctx, proxyCompiled, config)
	require.NoError(t, err)

	return mod, r, &log
}

// requireErrnoNosys ensures a call of the given function returns errno. The log
// message returned can verify the output is wasm `-->` or a host `==>`
// function.
func requireErrnoNosys(t *testing.T, funcName string, params ...uint64) string {
	var log bytes.Buffer

	// Set context to one that has an experimental listener
	ctx := context.WithValue(testCtx, FunctionListenerFactoryKey{}, proxy.NewLoggingListenerFactory(&log))

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	// Instantiate the wasi module.
	wasiModuleCompiled, err := (&builder{r}).hostModuleBuilder().Compile(ctx)
	require.NoError(t, err)

	_, err = r.InstantiateModule(ctx, wasiModuleCompiled, wazero.NewModuleConfig())
	require.NoError(t, err)

	proxyBin := proxy.NewModuleBinary(ModuleName, wasiModuleCompiled)

	proxyCompiled, err := r.CompileModule(ctx, proxyBin)
	require.NoError(t, err)

	mod, err := r.InstantiateModule(ctx, proxyCompiled, wazero.NewModuleConfig())
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
