package adhoc

import (
	"context"
	_ "embed"
	"fmt"
	"math"
	"strconv"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/watzero"
	"github.com/tetratelabs/wazero/sys"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

var memoryCapacityPages = uint32(2)

var compileConfig = wazero.NewCompileConfig().
	WithMemorySizer(func(minPages uint32, maxPages *uint32) (min, capacity, max uint32) {
		if maxPages != nil {
			return minPages, memoryCapacityPages, *maxPages
		}
		return minPages, memoryCapacityPages, memoryCapacityPages
	})

var moduleConfig = wazero.NewModuleConfig()

var tests = map[string]func(t *testing.T, r wazero.Runtime){
	"huge stack":                                        testHugeStack,
	"unreachable":                                       testUnreachable,
	"recursive entry":                                   testRecursiveEntry,
	"imported-and-exported func":                        testImportedAndExportedFunc,
	"host function with context parameter":              testHostFunctionContextParameter,
	"host function with nested context":                 testNestedGoContext,
	"host function with numeric parameter":              testHostFunctionNumericParameter,
	"close module with in-flight calls":                 testCloseInFlight,
	"multiple instantiation from same source":           testMultipleInstantiation,
	"exported function that grows memory":               testMemOps,
	"import functions with reference type in signature": testReftypeImports,
	"overflow integer addition":                         testOverflow,
	"un-signed extend global":                           testGlobalExtend,
}

func TestEngineCompiler(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	runAllTests(t, tests, wazero.NewRuntimeConfigCompiler().WithWasmCore2())
}

func TestEngineInterpreter(t *testing.T) {
	runAllTests(t, tests, wazero.NewRuntimeConfigInterpreter().WithWasmCore2())
}

func runAllTests(t *testing.T, tests map[string]func(t *testing.T, r wazero.Runtime), config wazero.RuntimeConfig) {
	config = config.WithFeatureReferenceTypes(true)
	for name, testf := range tests {
		name := name   // pin
		testf := testf // pin
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			testf(t, wazero.NewRuntimeWithConfig(config))
		})
	}
}

var (
	//go:embed testdata/unreachable.wasm
	unreachableWasm []byte
	//go:embed testdata/recursive.wasm
	recursiveWasm []byte
	//go:embed testdata/hugestack.wasm
	hugestackWasm []byte
	//go:embed testdata/reftype_imports.wasm
	reftypeImportsWasm []byte
	//go:embed testdata/overflow.wasm
	overflowWasm []byte
	//go:embed testdata/global_extend.wasm
	globalExtendWasm []byte
)

func testReftypeImports(t *testing.T, r wazero.Runtime) {
	type dog struct {
		name string
	}

	hostObj := &dog{name: "hello"}
	host, err := r.NewModuleBuilder("host").
		ExportFunctions(map[string]interface{}{
			"externref": func(externrefFromRefNull uintptr) uintptr {
				require.Zero(t, externrefFromRefNull)
				return uintptr(unsafe.Pointer(hostObj))
			},
		}).Instantiate(testCtx, r)
	require.NoError(t, err)
	defer host.Close(testCtx)

	module, err := r.InstantiateModuleFromBinary(testCtx, reftypeImportsWasm)
	require.NoError(t, err)
	defer module.Close(testCtx)

	actual, err := module.ExportedFunction("get_externref_by_host").Call(testCtx)
	require.NoError(t, err)

	// Verifies that the returned raw uintptr is the same as the one for the host object.
	require.Equal(t, uintptr(unsafe.Pointer(hostObj)), uintptr(actual[0]))
}

func testHugeStack(t *testing.T, r wazero.Runtime) {
	module, err := r.InstantiateModuleFromBinary(testCtx, hugestackWasm)
	require.NoError(t, err)
	defer module.Close(testCtx)

	fn := module.ExportedFunction("main")
	require.NotNil(t, fn)

	res, err := fn.Call(testCtx, 0, 0, 0, 0, 0, 0) // params ignored by wasm
	require.NoError(t, err)

	const resultNumInUint64 = 180
	require.Equal(t, resultNumInUint64, len(res))
	for i := uint64(1); i <= resultNumInUint64; i++ {
		require.Equal(t, i, res[i-1])
	}
}

// testOverflow ensures that adding one into the maximum integer results in the
// minimum one. See #636.
func testOverflow(t *testing.T, r wazero.Runtime) {
	module, err := r.InstantiateModuleFromBinary(testCtx, overflowWasm)
	require.NoError(t, err)
	defer module.Close(testCtx)

	for _, name := range []string{"i32", "i64"} {
		i32 := module.ExportedFunction(name)
		require.NotNil(t, i32)

		res, err := i32.Call(testCtx)
		require.NoError(t, err)

		require.Equal(t, uint64(1), res[0])
	}
}

// testGlobalExtend ensures that un-signed extension of i32 globals must be zero extended. See #656.
func testGlobalExtend(t *testing.T, r wazero.Runtime) {
	module, err := r.InstantiateModuleFromBinary(testCtx, globalExtendWasm)
	require.NoError(t, err)
	defer module.Close(testCtx)

	extend := module.ExportedFunction("extend")
	require.NotNil(t, extend)

	res, err := extend.Call(testCtx)
	require.NoError(t, err)

	require.Equal(t, uint64(0xffff_ffff), res[0])
}

func testUnreachable(t *testing.T, r wazero.Runtime) {
	callUnreachable := func(nil api.Module) {
		panic("panic in host function")
	}

	_, err := r.NewModuleBuilder("host").ExportFunction("cause_unreachable", callUnreachable).Instantiate(testCtx, r)
	require.NoError(t, err)

	module, err := r.InstantiateModuleFromBinary(testCtx, unreachableWasm)
	require.NoError(t, err)
	defer module.Close(testCtx)

	_, err = module.ExportedFunction("main").Call(testCtx)
	exp := `panic in host function (recovered by wazero)
wasm stack trace:
	host.cause_unreachable()
	.two()
	.one()
	.main()`
	require.Equal(t, exp, err.Error())
}

func testRecursiveEntry(t *testing.T, r wazero.Runtime) {
	hostfunc := func(mod api.Module) {
		_, err := mod.ExportedFunction("called_by_host_func").Call(testCtx)
		require.NoError(t, err)
	}

	_, err := r.NewModuleBuilder("env").ExportFunction("host_func", hostfunc).Instantiate(testCtx, r)
	require.NoError(t, err)

	module, err := r.InstantiateModuleFromBinary(testCtx, recursiveWasm)
	require.NoError(t, err)
	defer module.Close(testCtx)

	_, err = module.ExportedFunction("main").Call(testCtx, 1)
	require.NoError(t, err)
}

func TestImportedAndExportedFunc(t *testing.T) {
	r := wazero.NewRuntime()
	testImportedAndExportedFunc(t, r)
}

// testImportedAndExportedFunc fails if the engine cannot call an "imported-and-then-exported-back" function
// Notably, this uses memory, which ensures api.Module is valid in both interpreter and Compiler engines.
func testImportedAndExportedFunc(t *testing.T, r wazero.Runtime) {
	var memory *wasm.MemoryInstance
	storeInt := func(ctx context.Context, m api.Module, offset uint32, val uint64) uint32 {
		if !m.Memory().WriteUint64Le(ctx, offset, val) {
			return 1
		}
		// sneak a reference to the memory, so we can check it later
		memory = m.Memory().(*wasm.MemoryInstance)
		return 0
	}

	host, err := r.NewModuleBuilder("").ExportFunction("store_int", storeInt).Instantiate(testCtx, r)
	require.NoError(t, err)
	defer host.Close(testCtx)

	module, err := r.InstantiateModuleFromBinary(testCtx, wat2wasm(`(module $test
		(import "" "store_int"
			(func $store_int (param $offset i32) (param $val i64) (result (;errno;) i32)))
		(memory $memory 1 1)
		(export "memory" (memory $memory))
		;; store_int is imported from the environment, but it's also exported back to the environment
		(export "store_int" (func $store_int))
		)`))
	require.NoError(t, err)
	defer module.Close(testCtx)

	// Call store_int and ensure it didn't return an error code.
	fn := module.ExportedFunction("store_int")
	results, err := fn.Call(testCtx, 1, math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, uint64(0), results[0])

	// Since offset=1 and val=math.MaxUint64, we expect to have written exactly 8 bytes, with all bits set, at index 1.
	require.Equal(t, []byte{0x0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x0}, memory.Buffer[0:10])
}

// testNestedGoContext ensures context is updated when a function calls another.
func testNestedGoContext(t *testing.T, r wazero.Runtime) {
	var nestedCtx = context.WithValue(context.Background(), struct{}{}, "nested")

	importedName := t.Name() + "-imported"
	importingName := t.Name() + "-importing"

	var importing api.Module
	fns := map[string]interface{}{
		"inner": func(ctx context.Context, p uint32) uint32 {
			// We expect the initial context, testCtx, to be overwritten by "outer" when it called this.
			require.Equal(t, nestedCtx, ctx)
			return p + 1
		},
		"outer": func(ctx context.Context, module api.Module, p uint32) uint32 {
			require.Equal(t, testCtx, ctx)
			results, err := module.ExportedFunction("inner").Call(nestedCtx, uint64(p))
			require.NoError(t, err)
			return uint32(results[0]) + 1
		},
	}

	imported, err := r.NewModuleBuilder(importedName).ExportFunctions(fns).Instantiate(testCtx, r)
	require.NoError(t, err)
	defer imported.Close(testCtx)

	// Instantiate a module that uses Wasm code to call the host function.
	importing, err = r.InstantiateModuleFromBinary(testCtx, wat2wasm(fmt.Sprintf(`(module $%[1]s
	(import "%[2]s" "outer" (func $outer (param i32) (result i32)))
	(import "%[2]s" "inner" (func $inner (param i32) (result i32)))
	(func $call_outer (param i32) (result i32) local.get 0 call $outer)
	(export "call->outer" (func $call_outer))
	(func $call_inner (param i32) (result i32) local.get 0 call $inner)
	(export "inner" (func $call_inner))
)`, importingName, importedName)))
	require.NoError(t, err)
	defer importing.Close(testCtx)

	input := uint64(math.MaxUint32 - 2) // We expect two calls where each increment by one.
	results, err := importing.ExportedFunction("call->outer").Call(testCtx, input)
	require.NoError(t, err)
	require.Equal(t, uint64(math.MaxUint32), results[0])
}

// testHostFunctionContextParameter ensures arg0 is optionally a context.
func testHostFunctionContextParameter(t *testing.T, r wazero.Runtime) {
	importedName := t.Name() + "-imported"
	importingName := t.Name() + "-importing"

	var importing api.Module
	fns := map[string]interface{}{
		"no_context": func(p uint32) uint32 {
			return p + 1
		},
		"go_context": func(ctx context.Context, p uint32) uint32 {
			require.Equal(t, testCtx, ctx)
			return p + 1
		},
		"module_context": func(module api.Module, p uint32) uint32 {
			require.Equal(t, importing, module)
			return p + 1
		},
	}

	imported, err := r.NewModuleBuilder(importedName).ExportFunctions(fns).Instantiate(testCtx, r)
	require.NoError(t, err)
	defer imported.Close(testCtx)

	for test := range fns {
		t.Run(test, func(t *testing.T) {
			// Instantiate a module that uses Wasm code to call the host function.
			importing, err = r.InstantiateModuleFromBinary(testCtx, wat2wasm(fmt.Sprintf(`(module $%[1]s
	(import "%[2]s" "%[3]s" (func $%[3]s (param i32) (result i32)))
	(func $call_%[3]s (param i32) (result i32) local.get 0 call $%[3]s)
	(export "call->%[3]s" (func $call_%[3]s))
)`, importingName, importedName, test)))
			require.NoError(t, err)
			defer importing.Close(testCtx)

			results, err := importing.ExportedFunction("call->"+test).Call(testCtx, math.MaxUint32-1)
			require.NoError(t, err)
			require.Equal(t, uint64(math.MaxUint32), results[0])
		})
	}
}

// testHostFunctionNumericParameter ensures numeric parameters aren't corrupted
func testHostFunctionNumericParameter(t *testing.T, r wazero.Runtime) {
	importedName := t.Name() + "-imported"
	importingName := t.Name() + "-importing"

	fns := map[string]interface{}{
		"i32": func(p uint32) uint32 {
			return p + 1
		},
		"i64": func(p uint64) uint64 {
			return p + 1
		},
		"f32": func(p float32) float32 {
			return p + 1
		},
		"f64": func(p float64) float64 {
			return p + 1
		},
	}

	imported, err := r.NewModuleBuilder(importedName).ExportFunctions(fns).Instantiate(testCtx, r)
	require.NoError(t, err)
	defer imported.Close(testCtx)

	for _, test := range []struct {
		name            string
		input, expected uint64
	}{
		{
			name:     "i32",
			input:    math.MaxUint32 - 1,
			expected: math.MaxUint32,
		},
		{
			name:     "i64",
			input:    math.MaxUint64 - 1,
			expected: math.MaxUint64,
		},
		{
			name:     "f32",
			input:    api.EncodeF32(math.MaxFloat32 - 1),
			expected: api.EncodeF32(math.MaxFloat32),
		},
		{
			name:     "f64",
			input:    api.EncodeF64(math.MaxFloat64 - 1),
			expected: api.EncodeF64(math.MaxFloat64),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Instantiate a module that uses Wasm code to call the host function.
			importing, err := r.InstantiateModuleFromBinary(testCtx, wat2wasm(fmt.Sprintf(`(module $%[1]s
	(import "%[2]s" "%[3]s" (func $%[3]s (param %[3]s) (result %[3]s)))
	(func $call_%[3]s (param %[3]s) (result %[3]s) local.get 0 call $%[3]s)
	(export "call->%[3]s" (func $call_%[3]s))
)`, importingName, importedName, test.name)))
			require.NoError(t, err)
			defer importing.Close(testCtx)

			results, err := importing.ExportedFunction("call->"+test.name).Call(testCtx, test.input)
			require.NoError(t, err)
			require.Equal(t, test.expected, results[0])
		})
	}
}

func callReturnImportWasm(importedModule, importingModule string) []byte {
	return wat2wasm(fmt.Sprintf(`(module $%[1]s
	;; test an imported function by re-exporting it
	(import "%[2]s" "return_input" (func $return_input (param i32) (result i32)))
	(export "return_input" (func $return_input))

	;; test wasm, by calling an imported function
	(func $call_return_import (param i32) (result i32) local.get 0 call $return_input)
	(export "call_return_import" (func $call_return_import))
)`, importingModule, importedModule))
}

func wat2wasm(wat string) []byte {
	wasm, _ := watzero.Wat2Wasm(wat)
	return wasm
}

func testCloseInFlight(t *testing.T, r wazero.Runtime) {
	tests := []struct {
		name, function                        string
		closeImporting, closeImported         uint32
		closeImportingCode, closeImportedCode bool
	}{
		{ // Ex. WASI proc_exit or AssemblyScript abort handler.
			name:           "importing",
			function:       "call_return_import",
			closeImporting: 1,
		},
		// TODO: A module that re-exports a function (ex "return_input") can call it after it is closed!
		{ // Ex. A function that stops the runtime.
			name:           "both",
			function:       "call_return_import",
			closeImporting: 1,
			closeImported:  2,
		},
		{ // Ex. WASI proc_exit or AssemblyScript abort handler.
			name:              "importing",
			function:          "call_return_import",
			closeImporting:    1,
			closeImportedCode: true,
		},
		{ // Ex. WASI proc_exit or AssemblyScript abort handler.
			name:               "importing",
			function:           "call_return_import",
			closeImporting:     1,
			closeImportedCode:  true,
			closeImportingCode: true,
		},
		// TODO: A module that re-exports a function (ex "return_input") can call it after it is closed!
		{ // Ex. A function that stops the runtime.
			name:               "both",
			function:           "call_return_import",
			closeImporting:     1,
			closeImported:      2,
			closeImportingCode: true,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var importingCode, importedCode wazero.CompiledModule
			var imported, importing api.Module
			var err error
			closeAndReturn := func(ctx context.Context, x uint32) uint32 {
				if tc.closeImporting != 0 {
					require.NoError(t, importing.CloseWithExitCode(ctx, tc.closeImporting))
				}
				if tc.closeImported != 0 {
					require.NoError(t, imported.CloseWithExitCode(ctx, tc.closeImported))
				}
				if tc.closeImportedCode {
					importedCode.Close(testCtx)
				}
				if tc.closeImportingCode {
					importingCode.Close(testCtx)
				}
				return x
			}

			// Create the host module, which exports the function that closes the importing module.
			importedCode, err = r.NewModuleBuilder(t.Name()+"-imported").
				ExportFunction("return_input", closeAndReturn).Compile(testCtx, compileConfig)
			require.NoError(t, err)

			imported, err = r.InstantiateModule(testCtx, importedCode, moduleConfig)
			require.NoError(t, err)
			defer imported.Close(testCtx)

			// Import that module.
			binary := callReturnImportWasm(imported.Name(), t.Name()+"-importing")
			importingCode, err = r.CompileModule(testCtx, binary, compileConfig)
			require.NoError(t, err)

			importing, err = r.InstantiateModule(testCtx, importingCode, moduleConfig)
			require.NoError(t, err)
			defer importing.Close(testCtx)

			var expectedErr error
			if tc.closeImported != 0 && tc.closeImporting != 0 {
				// When both modules are closed, importing is the better one to choose in the error message.
				expectedErr = sys.NewExitError(importing.Name(), tc.closeImporting)
			} else if tc.closeImported != 0 {
				expectedErr = sys.NewExitError(imported.Name(), tc.closeImported)
			} else if tc.closeImporting != 0 {
				expectedErr = sys.NewExitError(importing.Name(), tc.closeImporting)
			} else {
				t.Fatal("invalid test case")
			}

			// Functions that return after being closed should have an exit error.
			_, err = importing.ExportedFunction(tc.function).Call(testCtx, 5)
			require.Equal(t, expectedErr, err)
		})
	}
}

func testMemOps(t *testing.T, r wazero.Runtime) {
	// Instantiate a module that manages its memory
	memory, err := r.InstantiateModuleFromBinary(testCtx, wat2wasm(`(module $memory
  (func $grow (param $delta i32) (result (;previous_size;) i32) local.get 0 memory.grow)
  (func $size (result (;size;) i32) memory.size)

  (memory 0)

  (export "size" (func $size))
  (export "grow" (func $grow))
  (export "memory" (memory 0))

  (func $store (param $offset i32)
	local.get 0    ;; memory offset
	i64.const 1
	i64.store
  )
  (export "store" (func $store))
)`))
	require.NoError(t, err)
	defer memory.Close(testCtx)

	// Check the export worked
	require.Equal(t, memory.Memory(), memory.ExportedMemory("memory"))

	// Check the size command worked
	results, err := memory.ExportedFunction("size").Call(testCtx)
	require.NoError(t, err)
	require.Zero(t, results[0])
	require.Zero(t, memory.ExportedMemory("memory").Size(testCtx))

	// Any offset should be out of bounds error even when it is less than memory capacity(=memoryCapacityPages).
	_, err = memory.ExportedFunction("store").Call(testCtx, wasm.MemoryPagesToBytesNum(memoryCapacityPages)-8)
	require.Error(t, err) // Out of bounds error.

	// Try to grow the memory by one page
	results, err = memory.ExportedFunction("grow").Call(testCtx, 1)
	require.NoError(t, err)
	require.Zero(t, results[0]) // should succeed and return the old size in pages.

	// Any offset larger than the current size should be out of of bounds error even when it is less than memory capacity.
	_, err = memory.ExportedFunction("store").Call(testCtx, wasm.MemoryPagesToBytesNum(memoryCapacityPages)-8)
	require.Error(t, err) // Out of bounds error.

	// Check the size command works!
	results, err = memory.ExportedFunction("size").Call(testCtx)
	require.NoError(t, err)
	require.Equal(t, uint64(1), results[0])                        // 1 page
	require.Equal(t, uint32(65536), memory.Memory().Size(testCtx)) // 64KB

	// Grow again so that the memory size matches memory capacity.
	results, err = memory.ExportedFunction("grow").Call(testCtx, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), results[0])

	// Verify the size matches cap.
	results, err = memory.ExportedFunction("size").Call(testCtx)
	require.NoError(t, err)
	require.Equal(t, uint64(memoryCapacityPages), results[0])

	// Now the store instruction at the memory capcity bound should succeed.
	_, err = memory.ExportedFunction("store").Call(testCtx, wasm.MemoryPagesToBytesNum(memoryCapacityPages)-8) // i64.store needs 8 bytes from offset.
	require.NoError(t, err)
}

func testMultipleInstantiation(t *testing.T, r wazero.Runtime) {
	compiled, err := r.CompileModule(testCtx, wat2wasm(`(module $test
		(memory 1)
		(func $store
          i32.const 1    ;; memory offset
		  i64.const 1000 ;; expected value
		  i64.store
		)
		(export "store" (func $store))
	  )`), compileConfig)
	require.NoError(t, err)
	defer compiled.Close(testCtx)

	// Instantiate multiple modules with the same source (*CompiledModule).
	for i := 0; i < 100; i++ {
		module, err := r.InstantiateModule(testCtx, compiled, wazero.NewModuleConfig().WithName(strconv.Itoa(i)))
		require.NoError(t, err)
		defer module.Close(testCtx)

		// Ensure that compilation cache doesn't cause race on memory instance.
		before, ok := module.Memory().ReadUint64Le(testCtx, 1)
		require.True(t, ok)
		// Value must be zero as the memory must not be affected by the previously instantiated modules.
		require.Zero(t, before)

		f := module.ExportedFunction("store")
		require.NotNil(t, f)

		_, err = f.Call(testCtx)
		require.NoError(t, err)

		// After the call, the value must be set properly.
		after, ok := module.Memory().ReadUint64Le(testCtx, 1)
		require.True(t, ok)
		require.Equal(t, uint64(1000), after)
	}
}
