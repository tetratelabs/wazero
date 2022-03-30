package adhoc

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/hammer"
	"github.com/tetratelabs/wazero/wasm"
)

var hammers = map[string]func(t *testing.T, r wazero.Runtime){
	// Tests here are similar to what's described in /RATIONALE.md, but deviate as they involve blocking functions.
	"close importing module while in use": closeImportingModuleWhileInUse,
	"close imported module while in use":  closeImportedModuleWhileInUse,
}

func TestEngineJIT_hammer(t *testing.T) {
	if !wazero.JITSupported {
		t.Skip()
	}
	runAllTests(t, hammers, wazero.NewRuntimeConfigJIT())
}

func TestEngineInterpreter_hammer(t *testing.T) {
	runAllTests(t, hammers, wazero.NewRuntimeConfigInterpreter())
}

func closeImportingModuleWhileInUse(t *testing.T, r wazero.Runtime) {
	closeModuleWhileInUse(t, r, func(hostFunctionClosed *uint32, imported, importing wasm.Module) (wasm.Module, wasm.Module) {
		// Close the importing module, despite calls being in-flight.
		require.NoError(t, importing.Close())

		// Prove a module can be redefined even with in-flight calls.
		source := callImportAfterAddSource(imported.Name(), importing.Name())
		importing, err := r.InstantiateModuleFromSource(source)
		require.NoError(t, err)
		return imported, importing
	})
}

func closeImportedModuleWhileInUse(t *testing.T, r wazero.Runtime) {
	closeModuleWhileInUse(t, r, func(hostFunctionClosed *uint32, imported, importing wasm.Module) (wasm.Module, wasm.Module) {
		// Close the underlying host function, which causes future calls to it to fail.
		atomic.StoreUint32(hostFunctionClosed, 1)

		// Validate new calls to the imported function fail, since it was closed.
		_, err := imported.ExportedFunction("return_input").Call(nil, 1)
		require.Contains(t, err.Error(), "wasm runtime error: function closed")

		// Close both the importing and the imported module, despite calls being in-flight
		require.NoError(t, imported.Close())
		require.NoError(t, importing.Close())

		// Redefine the imported module, with a function that no longer blocks.
		imported, err = r.NewModuleBuilder(imported.Name()).ExportFunction("return_input", func(x uint32) uint32 {
			return x
		}).Instantiate()
		require.NoError(t, err)

		// Redefine the importing module, which should link to the redefined host module.
		source := callImportAfterAddSource(imported.Name(), importing.Name())
		importing, err = r.InstantiateModuleFromSource(source)
		require.NoError(t, err)

		return imported, importing
	})
}

func closeModuleWhileInUse(t *testing.T, r wazero.Runtime, closeFn func(hostFunctionClosed *uint32, imported, importing wasm.Module) (wasm.Module, wasm.Module)) {
	args := []uint64{1, 123}
	exp := args[0] + args[1]

	P := 8               // max count of goroutines
	if testing.Short() { // Adjust down if `-test.short`
		P = 4
	}

	var hostFunctionClosed uint32
	// To know return path works on a closed module, we need to block calls.
	var calls sync.WaitGroup
	calls.Add(P)
	blockAndReturn := func(x uint32) uint32 {
		if atomic.LoadUint32(&hostFunctionClosed) == 1 { // Not require.False as we don't want to fail the test.
			panic(errors.New("function closed"))
		}
		calls.Wait()
		return x
	}

	// Create the host module, which exports the blocking function.
	imported, err := r.NewModuleBuilder(t.Name()+"-imported").
		ExportFunction("return_input", blockAndReturn).Instantiate()
	require.NoError(t, err)
	defer imported.Close()

	// Import that module.
	source := callImportAfterAddSource(imported.Name(), t.Name()+"-importing")
	importing, err := r.InstantiateModuleFromSource(source)
	require.NoError(t, err)
	defer importing.Close()

	// As this is a blocking function call, only run 1 per goroutine.
	hammer.NewHammer(t, P, 1).Run(func(name string) {
		requireFunctionCall(t, importing.ExportedFunction("call_import_after_add"), args, exp)
	}, func() { // When all functions are in-flight, re-assign the modules.
		imported, importing = closeFn(&hostFunctionClosed, imported, importing)
		// Unblock all the calls
		calls.Add(-P)
	})
	// As references may have changed, ensure we close both.
	defer imported.Close()
	defer importing.Close()
	if t.Failed() {
		return // At least one test failed, so return now.
	}

	// If unloading worked properly, a new function call should route to the newly instantiated module.
	requireFunctionCall(t, importing.ExportedFunction("call_import_after_add"), args, exp)
}

func requireFunctionCall(t *testing.T, fn wasm.Function, args []uint64, exp uint64) {
	res, err := fn.Call(nil, args...)
	// We don't expect an error because there's currently no functionality to detect or fail on a closed module.
	require.NoError(t, err)
	require.Equal(t, exp, res[0])
}
