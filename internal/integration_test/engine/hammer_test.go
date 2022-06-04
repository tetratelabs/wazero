package adhoc

import (
	"sync"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/hammer"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

var hammers = map[string]func(t *testing.T, r wazero.Runtime){
	// Tests here are similar to what's described in /RATIONALE.md, but deviate as they involve blocking functions.
	"close importing module while in use": closeImportingModuleWhileInUse,
	"close imported module while in use":  closeImportedModuleWhileInUse,
}

func TestEngineCompiler_hammer(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	runAllTests(t, hammers, wazero.NewRuntimeConfigCompiler())
}

func TestEngineInterpreter_hammer(t *testing.T) {
	runAllTests(t, hammers, wazero.NewRuntimeConfigInterpreter())
}

func closeImportingModuleWhileInUse(t *testing.T, r wazero.Runtime) {
	closeModuleWhileInUse(t, r, func(imported, importing api.Module) (api.Module, api.Module) {
		// Close the importing module, despite calls being in-flight.
		require.NoError(t, importing.Close(testCtx))

		// Prove a module can be redefined even with in-flight calls.
		binary := callReturnImportWasm(imported.Name(), importing.Name())
		importing, err := r.InstantiateModuleFromBinary(testCtx, binary)
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
		imported, err := r.NewModuleBuilder(imported.Name()).ExportFunction("return_input", func(x uint32) uint32 {
			return x
		}).Instantiate(testCtx, r)
		require.NoError(t, err)

		// Redefine the importing module, which should link to the redefined host module.
		binary := callReturnImportWasm(imported.Name(), importing.Name())
		importing, err = r.InstantiateModuleFromBinary(testCtx, binary)
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
	blockAndReturn := func(x uint32) uint32 {
		calls.Wait()
		return x
	}

	// Create the host module, which exports the blocking function.
	imported, err := r.NewModuleBuilder(t.Name()+"-imported").
		ExportFunction("return_input", blockAndReturn).Instantiate(testCtx, r)
	require.NoError(t, err)
	defer imported.Close(testCtx)

	// Import that module.
	binary := callReturnImportWasm(imported.Name(), t.Name()+"-importing")
	importing, err := r.InstantiateModuleFromBinary(testCtx, binary)
	require.NoError(t, err)
	defer importing.Close(testCtx)

	// As this is a blocking function call, only run 1 per goroutine.
	i := importing // pin the module used inside goroutines
	hammer.NewHammer(t, P, 1).Run(func(name string) {
		// In all cases, the importing module is closed, so the error should have that as its module name.
		requireFunctionCallExits(t, i.Name(), i.ExportedFunction("call_return_import"))
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
	requireFunctionCall(t, importing.ExportedFunction("call_return_import"))
}

func requireFunctionCall(t *testing.T, fn api.Function) {
	res, err := fn.Call(testCtx, 3)
	require.NoError(t, err)
	require.Equal(t, uint64(3), res[0])
}

func requireFunctionCallExits(t *testing.T, moduleName string, fn api.Function) {
	_, err := fn.Call(testCtx, 3)
	require.Equal(t, sys.NewExitError(moduleName, 0), err)
}
