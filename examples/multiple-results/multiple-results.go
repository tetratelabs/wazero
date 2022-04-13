package main

import (
	_ "embed"
	"fmt"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// main implements functions with multiple returns values, using both an approach portable with any WebAssembly 1.0
// (20191205) runtime, as well one dependent on the "multiple-results" feature.
//
// The portable approach uses parameters to return additional results. The parameter value is a memory offset to write
// the next value. This is the same approach used by WASI snapshot-01!
//  * resultOffsetWasmFunctions
//  * resultOffsetHostFunctions
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
//
// Another approach is to enable the "multiple-results" feature. While "multiple-results" is not yet a W3C recommendation, most
// WebAssembly runtimes support it by default.
//  * multiValueWasmFunctions
//  * multiValueHostFunctions
// See https://github.com/WebAssembly/spec/blob/main/proposals/multi-value/Overview.md
func main() {
	// Create a portable WebAssembly Runtime.
	runtime := wazero.NewRuntime()

	// Add a module that uses offset parameters for multiple results, with functions defined in WebAssembly.
	wasm, err := resultOffsetWasmFunctions(runtime)
	if err != nil {
		log.Fatal(err)
	}
	defer wasm.Close()

	// Add a module that uses offset parameters for multiple results, with functions defined in Go.
	host, err := resultOffsetHostFunctions(runtime)
	if err != nil {
		log.Fatal(err)
	}
	defer host.Close()

	// wazero enables only W3C recommended features by default. Opt-in to other features like so:
	runtimeWithMultiValue := wazero.NewRuntimeWithConfig(
		wazero.NewRuntimeConfig().WithFeatureMultiValue(true),
		// ^^ Note: You can enable all features via WithFinishedFeatures.
	)

	// Add a module that uses multiple results values, with functions defined in WebAssembly.
	wasmWithMultiValue, err := multiValueWasmFunctions(runtimeWithMultiValue)
	if err != nil {
		log.Fatal(err)
	}
	defer wasmWithMultiValue.Close()

	// Add a module that uses multiple results values, with functions defined in Go.
	hostWithMultiValue, err := multiValueHostFunctions(runtimeWithMultiValue)
	if err != nil {
		log.Fatal(err)
	}
	defer hostWithMultiValue.Close()

	// Call the same function in all modules and print the results to the console.
	for _, mod := range []api.Module{wasm, host, wasmWithMultiValue, hostWithMultiValue} {
		getAge := mod.ExportedFunction("call_get_age")
		results, err := getAge.Call(nil)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("%s: age=%d\n", mod.Name(), results[0])
	}
}

// resultOffsetWasmFunctions defines Wasm functions that illustrate multiple results using a technique compatible
// with any WebAssembly 1.0 (20191205) runtime.
//
// To return a value in WASM written to a result parameter, you have to define memory and pass a location to write
// the result. At the end of your function, you load that location.
func resultOffsetWasmFunctions(r wazero.Runtime) (api.Module, error) {
	return r.InstantiateModuleFromCode([]byte(`(module $result-offset/wasm
  ;; To use result parameters, we need scratch memory. Allocate the least possible: 1 page (64KB).
  (memory 1 1)

  ;; Define a function that returns a result, while a second result is written to memory.
  (func $get_age (param $result_offset.age i32) (result (;errno;) i32)
    local.get 0   ;; stack = [$result_offset.age]
    i64.const 37  ;; stack = [$result_offset.age, 37]
    i64.store     ;; stack = []
    i32.const 0   ;; stack = [0]
  )

  ;; Now, define a function that shows the Wasm mechanics returning something written to a result parameter.
  ;; The caller provides a memory offset to the callee, so that it knows where to write the second result.
  (func $call_get_age (result i64)
    i32.const 8   ;; stack = [8] $result_offset.age parameter to get_age (arbitrary memory offset 8)
    call $get_age ;; stack = [errno] result of get_age
    drop          ;; stack = []

    i32.const 8   ;; stack = [8] same value as the $result_offset.age parameter
    i64.load      ;; stack = [age]
  )

  ;; Export the function, so that we can test it!
  (export "call_get_age" (func $call_get_age))
)`))
}

// resultOffsetHostFunctions defines host functions that illustrate multiple results using a technique compatible
// with any WebAssembly 1.0 (20191205) runtime.
//
// To return a value in WASM written to a result parameter, you have to define memory and pass a location to write
// the result. At the end of your function, you load that location.
func resultOffsetHostFunctions(r wazero.Runtime) (api.Module, error) {
	return r.NewModuleBuilder("result-offset/host").
		// To use result parameters, we need scratch memory. Allocate the least possible: 1 page (64KB).
		ExportMemoryWithMax("mem", 1, 1).
		// Define a function that returns a result, while a second result is written to memory.
		ExportFunction("get_age", func(m api.Module, resultOffsetAge uint32) (errno uint32) {
			if m.Memory().WriteUint64Le(resultOffsetAge, 37) {
				return 0
			}
			return 1 // overflow
		}).
		// Now, define a function that shows the Wasm mechanics returning something written to a result parameter.
		// The caller provides a memory offset to the callee, so that it knows where to write the second result.
		ExportFunction("call_get_age", func(m api.Module) (age uint64) {
			resultOffsetAge := uint32(8) // arbitrary memory offset (in bytes)
			_, _ = m.ExportedFunction("get_age").Call(m, uint64(resultOffsetAge))
			age, _ = m.Memory().ReadUint64Le(resultOffsetAge)
			return
		}).Instantiate()
}

// multiValueWasmFunctions defines Wasm functions that illustrate multiple results using the "multiple-results" feature.
func multiValueWasmFunctions(r wazero.Runtime) (api.Module, error) {
	return r.InstantiateModuleFromCode([]byte(`(module $multi-value/wasm

  ;; Define a function that returns two results
  (func $get_age (result (;age;) i64 (;errno;) i32)
    i64.const 37  ;; stack = [37]
    i32.const 0   ;; stack = [37, 0]
  )

  ;; Now, define a function that returns only the first result.
  (func $call_get_age (result i64)
    call $get_age ;; stack = [37, errno] result of get_age
    drop          ;; stack = [37]
  )

  ;; Export the function, so that we can test it!
  (export "call_get_age" (func $call_get_age))
)`))
}

// multiValueHostFunctions defines Wasm functions that illustrate multiple results using the "multiple-results" feature.
func multiValueHostFunctions(r wazero.Runtime) (api.Module, error) {
	return r.NewModuleBuilder("multi-value/host").
		// Define a function that returns two results
		ExportFunction("get_age", func() (age uint64, errno uint32) {
			age = 37
			errno = 0
			return
		}).
		// Now, define a function that returns only the first result.
		ExportFunction("call_get_age", func(m api.Module) (age uint64) {
			results, _ := m.ExportedFunction("get_age").Call(m)
			return results[0]
		}).Instantiate()
}
