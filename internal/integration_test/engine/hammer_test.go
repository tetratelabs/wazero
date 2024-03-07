package adhoc

import (
	"context"
	"sync"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/hammer"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

var hammers = map[string]testCase{
	// Tests here are similar to what's described in /RATIONALE.md, but deviate as they involve blocking functions.
	"close importing module while in use":         {f: closeImportingModuleWhileInUse},
	"close imported module while in use":          {f: closeImportedModuleWhileInUse},
	"linking a closed module should not segfault": {f: testLinkingHammer},
}

func TestEngineCompiler_hammer(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	runAllTests(t, hammers, wazero.NewRuntimeConfigCompiler(), false)
}

func TestEngineInterpreter_hammer(t *testing.T) {
	runAllTests(t, hammers, wazero.NewRuntimeConfigInterpreter(), false)
}

func closeImportingModuleWhileInUse(t *testing.T, r wazero.Runtime) {
	closeModuleWhileInUse(t, r, func(imported, importing api.Module) (api.Module, api.Module) {
		// Close the importing module, despite calls being in-flight.
		require.NoError(t, importing.Close(testCtx))

		// Prove a module can be redefined even with in-flight calls.
		binary := callReturnImportWasm(t, imported.Name(), importing.Name(), i32)
		importing, err := r.Instantiate(testCtx, binary)
		require.NoError(t, err)
		return imported, importing
	})
}

func closeImportedModuleWhileInUse(t *testing.T, r wazero.Runtime) {
	closeModuleWhileInUse(t, r, func(imported, importing api.Module) (api.Module, api.Module) {
		// Close the importing and imported module, despite calls being in-flight.
		require.NoError(t, importing.Close(testCtx))
		require.NoError(t, imported.Close(testCtx))

		// Redefine the imported module, with a function that no longer blocks.
		imported, err := r.NewHostModuleBuilder(imported.Name()).
			NewFunctionBuilder().
			WithFunc(func(ctx context.Context, x uint32) uint32 {
				return x
			}).
			Export("return_input").
			Instantiate(testCtx)
		require.NoError(t, err)

		// Redefine the importing module, which should link to the redefined host module.
		binary := callReturnImportWasm(t, imported.Name(), importing.Name(), i32)
		importing, err = r.Instantiate(testCtx, binary)
		require.NoError(t, err)

		return imported, importing
	})
}

func closeModuleWhileInUse(t *testing.T, r wazero.Runtime, closeFn func(imported, importing api.Module) (api.Module, api.Module)) {
	P := 8               // max count of goroutines
	if testing.Short() { // Adjust down if `-test.short`
		P = 4
	}

	// To know return path works on a closed module, we need to block calls.
	var calls sync.WaitGroup
	calls.Add(P)
	blockAndReturn := func(ctx context.Context, x uint32) uint32 {
		calls.Wait()
		return x
	}

	// Create the host module, which exports the blocking function.
	imported, err := r.NewHostModuleBuilder(t.Name() + "-imported").
		NewFunctionBuilder().WithFunc(blockAndReturn).Export("return_input").
		Instantiate(testCtx)
	require.NoError(t, err)
	defer imported.Close(testCtx)

	// Import that module.
	binary := callReturnImportWasm(t, imported.Name(), t.Name()+"-importing", i32)
	importing, err := r.Instantiate(testCtx, binary)
	require.NoError(t, err)
	defer importing.Close(testCtx)

	// As this is a blocking function call, only run 1 per goroutine.
	i := importing // pin the module used inside goroutines
	hammer.NewHammer(t, P, 1).Run(func(name string) {
		// In all cases, the importing module is closed, so the error should have that as its module name.
		requireFunctionCallExits(t, i.ExportedFunction("call_return_input"))
	}, func() { // When all functions are in-flight, re-assign the modules.
		imported, importing = closeFn(imported, importing)
		// Unblock all the calls
		calls.Add(-P)
	})
	// As references may have changed, ensure we close both.
	defer imported.Close(testCtx)
	defer importing.Close(testCtx)
	if t.Failed() {
		return // At least one test failed, so return now.
	}

	// If unloading worked properly, a new function call should route to the newly instantiated module.
	requireFunctionCall(t, importing.ExportedFunction("call_return_input"))
}

// testLinkingHammer links two modules where the first exports a table and the second module imports it,
// overwriting one of the table entries with one of its functions.
// The first module exposes a function that invokes the functionref in the table.
// If the functionref belongs to failed or closed module, then the call should not fail.
func testLinkingHammer(t *testing.T, r wazero.Runtime) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	hammer.NewHammer(t, 1, 10).Run(func(name string) {
		testLinking(t, r)
	}, func() {})
}

func requireFunctionCall(t *testing.T, fn api.Function) {
	res, err := fn.Call(testCtx, 3)
	require.NoError(t, err)
	require.Equal(t, uint64(3), res[0])
}

func requireFunctionCallExits(t *testing.T, fn api.Function) {
	_, err := fn.Call(testCtx, 3)
	require.Equal(t, sys.NewExitError(0), err)
}
