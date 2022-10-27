package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// main implements functions with multiple returns values, using both an
// approach portable with any WebAssembly 1.0 runtime, as well one dependent
// on the "multiple-results" feature.
//
// The portable approach uses parameters to return additional results. The
// parameter value is a memory offset to write the next value. This is the same
// approach used by WASI.
//   - resultOffsetWasmFunctions
//   - resultOffsetHostFunctions
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
//
// Another approach is to enable the "multiple-results" feature. While
// "multiple-results" is not yet a W3C recommendation, most WebAssembly
// runtimes support it by default, and it is include in the draft of 2.0.
//   - multiValueWasmFunctions
//   - multiValueHostFunctions
//
// See https://github.com/WebAssembly/spec/blob/wg-2.0.draft1/proposals/multi-value/Overview.md
func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Add a module that uses offset parameters for multiple results defined in WebAssembly.
	wasm, err := resultOffsetWasmFunctions(ctx, r)
	if err != nil {
		log.Panicln(err)
	}

	// wazero enables WebAssembly Core Specification 2.0 features by default.
	runtimeWithMultiValue := wazero.NewRuntime(ctx)

	// Add a module that uses multiple results values

	// ... defined in WebAssembly.
	wasmWithMultiValue, err := multiValueWasmFunctions(ctx, runtimeWithMultiValue)
	if err != nil {
		log.Panicln(err)
	}

	// ... defined in Go.
	multiValueFromImportedHost, err := multiValueFromImportedHostWasmFunctions(ctx, runtimeWithMultiValue)
	if err != nil {
		log.Panicln(err)
	}

	// Call the function from each module and print the results to the console.
	for _, mod := range []api.Module{wasm, wasmWithMultiValue, multiValueFromImportedHost} {
		getAge := mod.ExportedFunction("call_get_age")
		results, err := getAge.Call(ctx)
		if err != nil {
			log.Panicln(err)
		}

		fmt.Printf("%s: age=%d\n", mod.Name(), results[0])
	}
}

// resultOffsetWasm was generated by the following:
//
//	cd testdata; wat2wasm --debug-names result_offset.wat
//
//go:embed testdata/result_offset.wasm
var resultOffsetWasm []byte

// resultOffsetWasmFunctions are the WebAssembly equivalent of the Go-defined
// resultOffsetHostFunctions. The source is in testdata/result_offset.wat
func resultOffsetWasmFunctions(ctx context.Context, r wazero.Runtime) (api.Module, error) {
	return r.InstantiateModuleFromBinary(ctx, resultOffsetWasm)
}

// multiValueWasm was generated by the following:
//
//	cd testdata; wat2wasm --debug-names multi_value.wat
//
//go:embed testdata/multi_value.wasm
var multiValueWasm []byte

// multiValueWasmFunctions are the WebAssembly equivalent of the Go-defined
// multiValueHostFunctions. The source is in testdata/multi_value.wat
func multiValueWasmFunctions(ctx context.Context, r wazero.Runtime) (api.Module, error) {
	return r.InstantiateModuleFromBinary(ctx, multiValueWasm)
}

// multiValueWasm was generated by the following:
//
//	cd testdata; wat2wasm --debug-names multi_value_imported.wat
//
//go:embed testdata/multi_value_imported.wasm
var multiValueFromImportedHostWasm []byte

// multiValueFromImportedHostWasmFunctions return the WebAssembly which imports the Go-defined "get_age" function.
// The imported "get_age" function returns multiple results. The source is in testdata/multi_value_imported.wat
func multiValueFromImportedHostWasmFunctions(ctx context.Context, r wazero.Runtime) (api.Module, error) {
	// Instantiate the host module with the exported `get_age` function which returns multiple results.
	if _, err := r.NewHostModuleBuilder("multi-value/host").
		// Define a function that returns two results
		NewFunctionBuilder().
		WithFunc(func(context.Context) (age uint64, errno uint32) {
			age = 37
			errno = 0
			return
		}).
		Export("get_age").
		Instantiate(ctx, r); err != nil {
		return nil, err
	}
	// Then, creates the module which imports the `get_age` function from the `multi-value/host` module above.
	return r.InstantiateModuleFromBinary(ctx, multiValueFromImportedHostWasm)
}
