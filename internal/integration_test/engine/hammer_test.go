package adhoc

import (
	"context"
	"sync"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/hammer"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/sys"
)

var hammers = map[string]testCase{
	// Tests here are similar to what's described in /RATIONALE.md, but deviate as they involve blocking functions.
	"close importing module while in use":                 {f: closeImportingModuleWhileInUse},
	"close imported module while in use":                  {f: closeImportedModuleWhileInUse},
	"concurrent compilation, instantiation and execution": {f: concurrentCompilationInstantiationExecution},
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

func concurrentCompilationInstantiationExecution(t *testing.T, r wazero.Runtime) {
	P := 16              // max count of goroutines
	if testing.Short() { // Adjust down if `-test.short`
		P = 4
	}

	hammer.NewHammer(t, P, 50).Run(func(p, n int) {
		var body []byte
		for i := 0; i < p*n; i++ {
			body = append(body, wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 1, wasm.OpcodeI32Add, wasm.OpcodeLocalSet, 0)
		}
		body = append(body, wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd)

		bin := binaryencoding.EncodeModule(&wasm.Module{
			TypeSection:     []wasm.FunctionType{{Results: []wasm.ValueType{i32}}},
			FunctionSection: []wasm.Index{0},
			CodeSection:     []wasm.Code{{LocalTypes: []wasm.ValueType{i32}, Body: body}},
			ExportSection:   []wasm.Export{{Index: 0, Type: wasm.ExternTypeFunc, Name: "f"}},
		})
		m, err := r.Instantiate(testCtx, bin)
		require.NoError(t, err)
		fn := m.ExportedFunction("f")
		require.NotNil(t, fn)
		res, err := fn.Call(testCtx)
		require.NoError(t, err)
		require.Equal(t, uint64(p*n), res[0])
	}, nil)
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
	hammer.NewHammer(t, P, 1).Run(func(p, n int) {
		// In all cases, the importing module is closed, so the error should have that as its module name.
		fn := i.ExportedFunction("call_return_input")
		require.NotNil(t, fn)
		_, err := fn.Call(testCtx, 3)
		require.Equal(t, sys.NewExitError(0), err)
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
	fn := importing.ExportedFunction("call_return_input")
	require.NotNil(t, fn)
	res, err := fn.Call(testCtx, 3)
	require.NoError(t, err)
	require.Equal(t, uint64(3), res[0])
}
