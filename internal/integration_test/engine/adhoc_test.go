package adhoc

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"math"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/experimental/table"
	"github.com/tetratelabs/wazero/internal/engine/wazevo"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/proxy"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasmdebug"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
	"github.com/tetratelabs/wazero/sys"
)

type testCase struct {
	f          func(t *testing.T, r wazero.Runtime)
	wazevoSkip bool
}

var tests = map[string]testCase{
	"huge stack":                                        {f: testHugeStack, wazevoSkip: true},
	"unreachable":                                       {f: testUnreachable},
	"recursive entry":                                   {f: testRecursiveEntry},
	"host func memory":                                  {f: testHostFuncMemory},
	"host function with context parameter":              {f: testHostFunctionContextParameter},
	"host function with nested context":                 {f: testNestedGoContext},
	"host function with numeric parameter":              {f: testHostFunctionNumericParameter},
	"close module with in-flight calls":                 {f: testCloseInFlight},
	"multiple instantiation from same source":           {f: testMultipleInstantiation},
	"exported function that grows memory":               {f: testMemOps},
	"import functions with reference type in signature": {f: testReftypeImports, wazevoSkip: true},
	"overflow integer addition":                         {f: testOverflow},
	"un-signed extend global":                           {f: testGlobalExtend},
	"user-defined primitive in host func":               {f: testUserDefinedPrimitiveHostFunc},
	"ensures invocations terminate on module close":     {f: testEnsureTerminationOnClose},
	"call host function indirectly":                     {f: callHostFunctionIndirect},
	"lookup function":                                   {f: testLookupFunction},
	"memory grow in recursive call":                     {f: testMemoryGrowInRecursiveCall},
	"call":                                              {f: testCall},
	"module memory":                                     {f: testModuleMemory, wazevoSkip: true},
	"two indirection to host":                           {f: testTwoIndirection},
	"before listener globals":                           {f: testBeforeListenerGlobals},
	"before listener stack iterator":                    {f: testBeforeListenerStackIterator},
	"before listener stack iterator offsets":            {f: testListenerStackIteratorOffset, wazevoSkip: true},
}

func TestEngineCompiler(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	runAllTests(t, tests, wazero.NewRuntimeConfigCompiler().WithCloseOnContextDone(true), false)
}

func TestEngineInterpreter(t *testing.T) {
	runAllTests(t, tests, wazero.NewRuntimeConfigInterpreter().WithCloseOnContextDone(true), false)
}

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

const i32, i64 = wasm.ValueTypeI32, wasm.ValueTypeI64

var memoryCapacityPages = uint32(2)

func TestEngineWazevo(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip()
	}
	config := wazero.NewRuntimeConfigInterpreter()
	wazevo.ConfigureWazevo(config)
	runAllTests(t, tests, config.WithCloseOnContextDone(true), true)
}

func runAllTests(t *testing.T, tests map[string]testCase, config wazero.RuntimeConfig, isWazevo bool) {
	for name, tc := range tests {
		name := name
		tc := tc
		if isWazevo && tc.wazevoSkip {
			t.Logf("skipping %s because it is not supported by wazevo", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tc.f(t, wazero.NewRuntimeWithConfig(testCtx, config))
		})
	}
}

var (
	//go:embed testdata/unreachable.wasm
	unreachableWasm []byte
	//go:embed testdata/recursive.wasm
	recursiveWasm []byte
	//go:embed testdata/host_memory.wasm
	hostMemoryWasm []byte
	//go:embed testdata/hugestack.wasm
	hugestackWasm []byte
	//go:embed testdata/memory.wasm
	memoryWasm []byte
	//go:embed testdata/reftype_imports.wasm
	reftypeImportsWasm []byte
	//go:embed testdata/overflow.wasm
	overflowWasm []byte
	//go:embed testdata/global_extend.wasm
	globalExtendWasm []byte
	//go:embed testdata/infinite_loop.wasm
	infiniteLoopWasm []byte
)

func testEnsureTerminationOnClose(t *testing.T, r wazero.Runtime) {
	compiled, err := r.CompileModule(context.Background(), infiniteLoopWasm)
	require.NoError(t, err)

	newInfiniteLoopFn := func(t *testing.T) (m api.Module, infinite api.Function) {
		var err error
		m, err = r.InstantiateModule(context.Background(), compiled, wazero.NewModuleConfig().WithName(t.Name()))
		require.NoError(t, err)
		infinite = m.ExportedFunction("infinite_loop")
		require.NotNil(t, infinite)
		return
	}

	t.Run("context cancel", func(t *testing.T) {
		_, infinite := newInfiniteLoopFn(t)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(time.Second)
			cancel()
		}()
		_, err = infinite.Call(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "module closed with context canceled")
	})

	t.Run("context cancel in advance", func(t *testing.T) {
		_, infinite := newInfiniteLoopFn(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = infinite.Call(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "module closed with context canceled")
	})

	t.Run("context timeout", func(t *testing.T) {
		_, infinite := newInfiniteLoopFn(t)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, err = infinite.Call(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "module closed with context deadline exceeded")
	})

	t.Run("explicit close of module", func(t *testing.T) {
		module, infinite := newInfiniteLoopFn(t)
		go func() {
			time.Sleep(time.Second)
			require.NoError(t, module.CloseWithExitCode(context.Background(), 2))
		}()
		_, err = infinite.Call(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "module closed with exit_code(2)")
	})
}

func testUserDefinedPrimitiveHostFunc(t *testing.T, r wazero.Runtime) {
	type u32 uint32
	type u64 uint64
	type f32 float32
	type f64 float64

	const fn = "fn"
	hostCompiled, err := r.NewHostModuleBuilder("host").NewFunctionBuilder().
		WithFunc(func(u1 u32, u2 u64, f1 f32, f2 f64) u64 {
			return u64(u1) + u2 + u64(math.Float32bits(float32(f1))) + u64(math.Float64bits(float64(f2)))
		}).Export(fn).Compile(testCtx)
	require.NoError(t, err)

	_, err = r.InstantiateModule(testCtx, hostCompiled, wazero.NewModuleConfig())
	require.NoError(t, err)

	proxyBin := proxy.NewModuleBinary("host", hostCompiled)

	mod, err := r.Instantiate(testCtx, proxyBin)
	require.NoError(t, err)

	f := mod.ExportedFunction(fn)
	require.NotNil(t, f)

	const u1, u2, f1, f2 = 1, 2, float32(1234.123), 5431.123
	res, err := f.Call(context.Background(), uint64(u1), uint64(u2), uint64(math.Float32bits(f1)), math.Float64bits(f2))
	require.NoError(t, err)
	require.Equal(t, res[0], uint64(u1)+uint64(u2)+uint64(math.Float32bits(f1))+math.Float64bits(f2))
}

func testReftypeImports(t *testing.T, r wazero.Runtime) {
	type dog struct {
		name string
	}

	hostObj := &dog{name: "hello"}
	host, err := r.NewHostModuleBuilder("host").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, externrefFromRefNull uintptr) uintptr {
			require.Zero(t, externrefFromRefNull)
			return uintptr(unsafe.Pointer(hostObj))
		}).
		Export("externref").
		Instantiate(testCtx)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, host.Close(testCtx))
	}()

	module, err := r.Instantiate(testCtx, reftypeImportsWasm)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, module.Close(testCtx))
	}()

	actual, err := module.ExportedFunction("get_externref_by_host").Call(testCtx)
	require.NoError(t, err)

	// Verifies that the returned raw uintptr is the same as the one for the host object.
	require.Equal(t, uintptr(unsafe.Pointer(hostObj)), uintptr(actual[0]))
}

func testHugeStack(t *testing.T, r wazero.Runtime) {
	module, err := r.Instantiate(testCtx, hugestackWasm)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, module.Close(testCtx))
	}()

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
	module, err := r.Instantiate(testCtx, overflowWasm)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, module.Close(testCtx))
	}()

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
	module, err := r.Instantiate(testCtx, globalExtendWasm)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, module.Close(testCtx))
	}()

	extend := module.ExportedFunction("extend")
	require.NotNil(t, extend)

	res, err := extend.Call(testCtx)
	require.NoError(t, err)

	require.Equal(t, uint64(0xffff_ffff), res[0])
}

func testUnreachable(t *testing.T, r wazero.Runtime) {
	callUnreachable := func() {
		panic("panic in host function")
	}

	_, err := r.NewHostModuleBuilder("host").
		NewFunctionBuilder().WithFunc(callUnreachable).Export("cause_unreachable").
		Instantiate(testCtx)
	require.NoError(t, err)

	module, err := r.Instantiate(testCtx, unreachableWasm)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, module.Close(testCtx))
	}()

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
	hostfunc := func(ctx context.Context, mod api.Module) {
		_, err := mod.ExportedFunction("called_by_host_func").Call(testCtx)
		require.NoError(t, err)
	}

	_, err := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().WithFunc(hostfunc).Export("host_func").
		Instantiate(testCtx)
	require.NoError(t, err)

	module, err := r.Instantiate(testCtx, recursiveWasm)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, module.Close(testCtx))
	}()

	_, err = module.ExportedFunction("main").Call(testCtx, 1)
	require.NoError(t, err)
}

// testHostFuncMemory ensures that host functions can see the callers' memory
func testHostFuncMemory(t *testing.T, r wazero.Runtime) {
	var memory *wasm.MemoryInstance
	storeInt := func(ctx context.Context, m api.Module, offset uint32, val uint64) uint32 {
		if !m.Memory().WriteUint64Le(offset, val) {
			return 1
		}
		// sneak a reference to the memory, so we can check it later
		memory = m.Memory().(*wasm.MemoryInstance)
		return 0
	}

	host, err := r.NewHostModuleBuilder("host").
		NewFunctionBuilder().WithFunc(storeInt).Export("store_int").
		Instantiate(testCtx)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, host.Close(testCtx))
	}()

	module, err := r.Instantiate(testCtx, hostMemoryWasm)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, module.Close(testCtx))
	}()

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
	nestedCtx := context.WithValue(context.Background(), struct{}{}, "nested")

	importedName := t.Name() + "-imported"
	importingName := t.Name() + "-importing"

	var importing api.Module

	imported, err := r.NewHostModuleBuilder(importedName).
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, p uint32) uint32 {
			// We expect the initial context, testCtx, to be overwritten by "outer" when it called this.
			require.Equal(t, nestedCtx, ctx)
			return p + 1
		}).
		Export("inner").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, p uint32) uint32 {
			require.Equal(t, testCtx, ctx)
			results, err := module.ExportedFunction("inner").Call(nestedCtx, uint64(p))
			require.NoError(t, err)
			return uint32(results[0]) + 1
		}).
		Export("outer").
		Instantiate(testCtx)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, imported.Close(testCtx))
	}()

	// Instantiate a module that uses Wasm code to call the host function.
	importing, err = r.Instantiate(testCtx, callOuterInnerWasm(t, importedName, importingName))
	require.NoError(t, err)
	defer func() {
		require.NoError(t, importing.Close(testCtx))
	}()

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
		"ctx": func(ctx context.Context, p uint32) uint32 {
			require.Equal(t, testCtx, ctx)
			return p + 1
		},
		"ctx mod": func(ctx context.Context, module api.Module, p uint32) uint32 {
			require.Equal(t, importing, module)
			return p + 1
		},
	}

	for test := range fns {
		t.Run(test, func(t *testing.T) {
			imported, err := r.NewHostModuleBuilder(importedName).
				NewFunctionBuilder().WithFunc(fns[test]).Export("return_input").
				Instantiate(testCtx)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, imported.Close(testCtx))
			}()

			// Instantiate a module that uses Wasm code to call the host function.
			importing, err = r.Instantiate(testCtx,
				callReturnImportWasm(t, importedName, importingName, i32))
			require.NoError(t, err)
			defer func() {
				require.NoError(t, importing.Close(testCtx))
			}()

			results, err := importing.ExportedFunction("call_return_input").Call(testCtx, math.MaxUint32-1)
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
		"i32": func(ctx context.Context, p uint32) uint32 {
			return p + 1
		},
		"i64": func(ctx context.Context, p uint64) uint64 {
			return p + 1
		},
		"f32": func(ctx context.Context, p float32) float32 {
			return p + 1
		},
		"f64": func(ctx context.Context, p float64) float64 {
			return p + 1
		},
	}

	for _, test := range []struct {
		name            string
		vt              wasm.ValueType
		input, expected uint64
	}{
		{
			name:     "i32",
			vt:       i32,
			input:    math.MaxUint32 - 1,
			expected: math.MaxUint32,
		},
		{
			name:     "i64",
			vt:       i64,
			input:    math.MaxUint64 - 1,
			expected: math.MaxUint64,
		},
		{
			name:     "f32",
			vt:       wasm.ValueTypeF32,
			input:    api.EncodeF32(math.MaxFloat32 - 1),
			expected: api.EncodeF32(math.MaxFloat32),
		},
		{
			name:     "f64",
			vt:       wasm.ValueTypeF64,
			input:    api.EncodeF64(math.MaxFloat64 - 1),
			expected: api.EncodeF64(math.MaxFloat64),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			imported, err := r.NewHostModuleBuilder(importedName).
				NewFunctionBuilder().WithFunc(fns[test.name]).Export("return_input").
				Instantiate(testCtx)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, imported.Close(testCtx))
			}()

			// Instantiate a module that uses Wasm code to call the host function.
			importing, err := r.Instantiate(testCtx,
				callReturnImportWasm(t, importedName, importingName, test.vt))
			require.NoError(t, err)
			defer func() {
				require.NoError(t, importing.Close(testCtx))
			}()

			results, err := importing.ExportedFunction("call_return_input").Call(testCtx, test.input)
			require.NoError(t, err)
			require.Equal(t, test.expected, results[0])
		})
	}
}

func callHostFunctionIndirect(t *testing.T, r wazero.Runtime) {
	// With the following call graph,
	//  originWasmModule -- call --> importingWasmModule -- call --> hostModule
	// this ensures that hostModule's hostFn only has access importingWasmModule, not originWasmModule.

	const hostModule, importingWasmModule, originWasmModule = "host", "importing", "origin"
	const hostFn, importingWasmModuleFn, originModuleFn = "host_fn", "call_host_func", "origin"
	importingModule := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{}, Results: []wasm.ValueType{}}},
		ImportSection:   []wasm.Import{{Module: hostModule, Name: hostFn, Type: wasm.ExternTypeFunc, DescFunc: 0}},
		FunctionSection: []wasm.Index{0},
		ExportSection:   []wasm.Export{{Name: importingWasmModuleFn, Type: wasm.ExternTypeFunc, Index: 1}},
		CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeCall, 0, wasm.OpcodeEnd}}},
		NameSection:     &wasm.NameSection{ModuleName: importingWasmModule},
	}

	originModule := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{}, Results: []wasm.ValueType{}}},
		ImportSection:   []wasm.Import{{Module: importingWasmModule, Name: importingWasmModuleFn, Type: wasm.ExternTypeFunc, DescFunc: 0}},
		FunctionSection: []wasm.Index{0},
		ExportSection:   []wasm.Export{{Name: "origin", Type: wasm.ExternTypeFunc, Index: 1}},
		CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeCall, 0, wasm.OpcodeEnd}}},
		NameSection:     &wasm.NameSection{ModuleName: originWasmModule},
	}

	require.NoError(t, importingModule.Validate(api.CoreFeaturesV2))
	require.NoError(t, originModule.Validate(api.CoreFeaturesV2))
	importingModuleBytes := binaryencoding.EncodeModule(importingModule)
	originModuleBytes := binaryencoding.EncodeModule(originModule)

	var originInst, importingInst api.Module
	_, err := r.NewHostModuleBuilder(hostModule).
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module) {
			// Module must be the caller (importing module), not the origin.
			require.Equal(t, mod, importingInst)
			require.NotEqual(t, mod, originInst)
			// Name must be the caller, not origin.
			require.Equal(t, importingWasmModule, mod.Name())
		}).
		Export(hostFn).
		Instantiate(testCtx)
	require.NoError(t, err)

	importingInst, err = r.Instantiate(testCtx, importingModuleBytes)
	require.NoError(t, err)
	originInst, err = r.Instantiate(testCtx, originModuleBytes)
	require.NoError(t, err)

	originFn := originInst.ExportedFunction(originModuleFn)
	require.NotNil(t, originFn)

	_, err = originFn.Call(testCtx)
	require.NoError(t, err)
}

func callReturnImportWasm(t *testing.T, importedModule, importingModule string, vt wasm.ValueType) []byte {
	// test an imported function by re-exporting it
	module := &wasm.Module{
		TypeSection: []wasm.FunctionType{{Params: []wasm.ValueType{vt}, Results: []wasm.ValueType{vt}}},
		// (import "%[2]s" "return_input" (func $return_input (param i32) (result i32)))
		ImportSection: []wasm.Import{
			{Module: importedModule, Name: "return_input", Type: wasm.ExternTypeFunc, DescFunc: 0},
		},
		FunctionSection: []wasm.Index{0},
		ExportSection: []wasm.Export{
			// (export "return_input" (func $return_input))
			{Name: "return_input", Type: wasm.ExternTypeFunc, Index: 0},
			// (export "call_return_input" (func $call_return_input))
			{Name: "call_return_input", Type: wasm.ExternTypeFunc, Index: 1},
		},
		// (func $call_return_input (param i32) (result i32) local.get 0 call $return_input)
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, 0, wasm.OpcodeEnd}},
		},
		NameSection: &wasm.NameSection{
			ModuleName: importingModule,
			FunctionNames: wasm.NameMap{
				{Index: 0, Name: "return_input"},
				{Index: 1, Name: "call_return_input"},
			},
		},
	}
	require.NoError(t, module.Validate(api.CoreFeaturesV2))
	return binaryencoding.EncodeModule(module)
}

func callOuterInnerWasm(t *testing.T, importedModule, importingModule string) []byte {
	module := &wasm.Module{
		TypeSection: []wasm.FunctionType{{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}}},
		// (import "%[2]s" "outer" (func $outer (param i32) (result i32)))
		// (import "%[2]s" "inner" (func $inner (param i32) (result i32)))
		ImportSection: []wasm.Import{
			{Module: importedModule, Name: "outer", Type: wasm.ExternTypeFunc, DescFunc: 0},
			{Module: importedModule, Name: "inner", Type: wasm.ExternTypeFunc, DescFunc: 0},
		},
		FunctionSection: []wasm.Index{0, 0},
		ExportSection: []wasm.Export{
			// (export "call->outer" (func $call_outer))
			{Name: "call->outer", Type: wasm.ExternTypeFunc, Index: 2},
			// 	(export "inner" (func $call_inner))
			{Name: "inner", Type: wasm.ExternTypeFunc, Index: 3},
		},
		CodeSection: []wasm.Code{
			// (func $call_outer (param i32) (result i32) local.get 0 call $outer)
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, 0, wasm.OpcodeEnd}},
			// (func $call_inner (param i32) (result i32) local.get 0 call $inner)
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, 1, wasm.OpcodeEnd}},
		},
		NameSection: &wasm.NameSection{
			ModuleName: importingModule,
			FunctionNames: wasm.NameMap{
				{Index: 0, Name: "outer"},
				{Index: 1, Name: "inner"},
				{Index: 2, Name: "call_outer"},
				{Index: 3, Name: "call_inner"},
			},
		},
	}
	require.NoError(t, module.Validate(api.CoreFeaturesV2))
	return binaryencoding.EncodeModule(module)
}

func testCloseInFlight(t *testing.T, r wazero.Runtime) {
	tests := []struct {
		name, function                        string
		closeImporting, closeImported         uint32
		closeImportingCode, closeImportedCode bool
	}{
		{ // e.g. WASI proc_exit or AssemblyScript abort handler.
			name:           "importing",
			function:       "call_return_input",
			closeImporting: 1,
		},
		// TODO: A module that re-exports a function (ex "return_input") can call it after it is closed!
		{ // e.g. A function that stops the runtime.
			name:           "both",
			function:       "call_return_input",
			closeImporting: 1,
			closeImported:  2,
		},
		{ // e.g. WASI proc_exit or AssemblyScript abort handler.
			name:              "importing",
			function:          "call_return_input",
			closeImporting:    1,
			closeImportedCode: true,
		},
		{ // e.g. WASI proc_exit or AssemblyScript abort handler.
			name:               "importing",
			function:           "call_return_input",
			closeImporting:     1,
			closeImportedCode:  true,
			closeImportingCode: true,
		},
		// TODO: A module that re-exports a function (ex "return_input") can call it after it is closed!
		{ // e.g. A function that stops the runtime.
			name:               "both",
			function:           "call_return_input",
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
					err = importedCode.Close(testCtx)
					require.NoError(t, err)
				}
				if tc.closeImportingCode {
					err = importingCode.Close(testCtx)
					require.NoError(t, err)
				}
				return x
			}

			// Create the host module, which exports the function that closes the importing module.
			importedCode, err = r.NewHostModuleBuilder(t.Name() + "-imported").
				NewFunctionBuilder().WithFunc(closeAndReturn).Export("return_input").
				Compile(testCtx)
			require.NoError(t, err)

			imported, err = r.InstantiateModule(testCtx, importedCode, wazero.NewModuleConfig())
			require.NoError(t, err)
			defer func() {
				require.NoError(t, imported.Close(testCtx))
			}()

			// Import that module.
			bin := callReturnImportWasm(t, imported.Name(), t.Name()+"-importing", i32)
			importingCode, err = r.CompileModule(testCtx, bin)
			require.NoError(t, err)

			importing, err = r.InstantiateModule(testCtx, importingCode, wazero.NewModuleConfig())
			require.NoError(t, err)
			defer func() {
				require.NoError(t, importing.Close(testCtx))
			}()

			var expectedErr error
			if tc.closeImported != 0 && tc.closeImporting != 0 {
				// When both modules are closed, importing is the better one to choose in the error message.
				expectedErr = sys.NewExitError(tc.closeImporting)
			} else if tc.closeImported != 0 {
				expectedErr = sys.NewExitError(tc.closeImported)
			} else if tc.closeImporting != 0 {
				expectedErr = sys.NewExitError(tc.closeImporting)
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
	mod, err := r.Instantiate(testCtx, memoryWasm)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, mod.Close(testCtx))
	}()

	// Check the export worked
	require.Equal(t, mod.Memory(), mod.ExportedMemory("memory"))
	memory := mod.Memory()

	sizeFn, storeFn, growFn := mod.ExportedFunction("size"), mod.ExportedFunction("store"), mod.ExportedFunction("grow")

	// Check the size command worked
	results, err := sizeFn.Call(testCtx)
	require.NoError(t, err)
	require.Zero(t, results[0])
	require.Zero(t, memory.Size())

	// Any offset should be out of bounds error even when it is less than memory capacity(=memoryCapacityPages).
	_, err = storeFn.Call(testCtx, wasm.MemoryPagesToBytesNum(memoryCapacityPages)-8)
	require.Error(t, err) // Out of bounds error.

	// Try to grow the memory by one page
	results, err = growFn.Call(testCtx, 1)
	require.NoError(t, err)
	require.Zero(t, results[0]) // should succeed and return the old size in pages.

	// Any offset larger than the current size should be out of bounds error even when it is less than memory capacity.
	_, err = storeFn.Call(testCtx, wasm.MemoryPagesToBytesNum(memoryCapacityPages)-8)
	require.Error(t, err) // Out of bounds error.

	// Check the size command works!
	results, err = sizeFn.Call(testCtx)
	require.NoError(t, err)
	require.Equal(t, uint64(1), results[0])        // 1 page
	require.Equal(t, uint32(65536), memory.Size()) // 64KB

	// Grow again so that the memory size matches memory capacity.
	results, err = growFn.Call(testCtx, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), results[0])

	// Verify the size matches cap.
	results, err = sizeFn.Call(testCtx)
	require.NoError(t, err)
	require.Equal(t, uint64(memoryCapacityPages), results[0])

	// Now the store instruction at the memory capcity bound should succeed.
	_, err = storeFn.Call(testCtx, wasm.MemoryPagesToBytesNum(memoryCapacityPages)-8) // i64.store needs 8 bytes from offset.
	require.NoError(t, err)
}

func testMultipleInstantiation(t *testing.T, r wazero.Runtime) {
	bin := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		FunctionSection: []wasm.Index{0},
		MemorySection:   &wasm.Memory{Min: 1, Cap: 1, Max: 1, IsMaxEncoded: true},
		CodeSection: []wasm.Code{{
			Body: []byte{
				wasm.OpcodeI32Const, 1, // i32.const 1    ;; memory offset
				wasm.OpcodeI64Const, 0xe8, 0x7, // i64.const 1000 ;; expected value
				wasm.OpcodeI64Store, 0x3, 0x0, // i64.store
				wasm.OpcodeEnd,
			},
		}},
		ExportSection: []wasm.Export{{Name: "store"}},
	})
	compiled, err := r.CompileModule(testCtx, bin)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, compiled.Close(testCtx))
	}()

	// Instantiate multiple modules with the same source (*CompiledModule).
	for i := 0; i < 100; i++ {
		module, err := r.InstantiateModule(testCtx, compiled, wazero.NewModuleConfig().WithName(strconv.Itoa(i)))
		require.NoError(t, err)

		// Ensure that compilation cache doesn't cause race on memory instance.
		before, ok := module.Memory().ReadUint64Le(1)
		require.True(t, ok)
		// Value must be zero as the memory must not be affected by the previously instantiated modules.
		require.Zero(t, before)

		f := module.ExportedFunction("store")
		require.NotNil(t, f)

		_, err = f.Call(testCtx)
		require.NoError(t, err)

		// After the call, the value must be set properly.
		after, ok := module.Memory().ReadUint64Le(1)
		require.True(t, ok)
		require.Equal(t, uint64(1000), after)

		require.NoError(t, module.Close(testCtx))
	}
}

func testLookupFunction(t *testing.T, r wazero.Runtime) {
	bin := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{{Results: []wasm.ValueType{i32}}},
		FunctionSection: []wasm.Index{0, 0, 0},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeI32Const, 1, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeI32Const, 2, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeI32Const, 3, wasm.OpcodeEnd}},
		},
		TableSection: []wasm.Table{{Min: 10, Type: wasm.RefTypeFuncref}},
		ElementSection: []wasm.ElementSegment{
			{
				OffsetExpr: wasm.ConstantExpression{
					Opcode: wasm.OpcodeI32Const,
					Data:   []byte{0},
				},
				TableIndex: 0,
				Init:       []wasm.Index{2, 0},
			},
		},
	})

	inst, err := r.Instantiate(testCtx, bin)
	require.NoError(t, err)

	t.Run("null reference", func(t *testing.T) {
		err = require.CapturePanic(func() {
			table.LookupFunction(inst, 0, 3, nil, []wasm.ValueType{i32})
		})
		require.Equal(t, wasmruntime.ErrRuntimeInvalidTableAccess, err)
	})

	t.Run("out of range", func(t *testing.T) {
		err = require.CapturePanic(func() {
			table.LookupFunction(inst, 0, 1000, nil, []wasm.ValueType{i32})
		})
		require.Equal(t, wasmruntime.ErrRuntimeInvalidTableAccess, err)
	})

	t.Run("type mismatch", func(t *testing.T) {
		err = require.CapturePanic(func() {
			table.LookupFunction(inst, 0, 0, []wasm.ValueType{i32}, nil)
		})
		require.Equal(t, wasmruntime.ErrRuntimeIndirectCallTypeMismatch, err)
	})
	t.Run("ok", func(t *testing.T) {
		f2 := table.LookupFunction(inst, 0, 0, nil, []wasm.ValueType{i32})
		res, err := f2.Call(testCtx)
		require.NoError(t, err)
		require.Equal(t, uint64(3), res[0])

		f0 := table.LookupFunction(inst, 0, 1, nil, []wasm.ValueType{i32})
		res, err = f0.Call(testCtx)
		require.NoError(t, err)
		require.Equal(t, uint64(1), res[0])
	})
}

func testMemoryGrowInRecursiveCall(t *testing.T, r wazero.Runtime) {
	const hostModuleName = "env"
	const hostFnName = "grow_memory"
	var growFn api.Function
	hostCompiled, err := r.NewHostModuleBuilder(hostModuleName).NewFunctionBuilder().
		WithFunc(func() {
			// Does the recursive call into Wasm, which grows memory.
			_, err := growFn.Call(testCtx)
			require.NoError(t, err)
		}).Export(hostFnName).Compile(testCtx)
	require.NoError(t, err)

	_, err = r.InstantiateModule(testCtx, hostCompiled, wazero.NewModuleConfig())
	require.NoError(t, err)

	bin := binaryencoding.EncodeModule(&wasm.Module{
		ImportFunctionCount: 1,
		TypeSection:         []wasm.FunctionType{{Params: []wasm.ValueType{}, Results: []wasm.ValueType{}}},
		FunctionSection:     []wasm.Index{0, 0},
		CodeSection: []wasm.Code{
			{
				Body: []byte{
					// Calls the imported host function, which in turn calls the next in-Wasm function recursively.
					wasm.OpcodeCall, 0,
					// Access the memory and this should succeed as we already had memory grown at this point.
					wasm.OpcodeI32Const, 0,
					wasm.OpcodeI32Load, 0x2, 0x0,
					wasm.OpcodeDrop,
					wasm.OpcodeEnd,
				},
			},
			{
				// Grows memory by 1 page.
				Body: []byte{wasm.OpcodeI32Const, 1, wasm.OpcodeMemoryGrow, 0, wasm.OpcodeDrop, wasm.OpcodeEnd},
			},
		},
		MemorySection:   &wasm.Memory{Max: 1000},
		ImportSection:   []wasm.Import{{Module: hostModuleName, Name: hostFnName, DescFunc: 0}},
		ImportPerModule: map[string][]*wasm.Import{hostModuleName: {{Module: hostModuleName, Name: hostFnName, DescFunc: 0}}},
		ExportSection: []wasm.Export{
			{Name: "main", Type: wasm.ExternTypeFunc, Index: 1},
			{Name: "grow_memory", Type: wasm.ExternTypeFunc, Index: 2},
		},
	})

	inst, err := r.Instantiate(testCtx, bin)
	require.NoError(t, err)

	growFn = inst.ExportedFunction("grow_memory")
	require.NotNil(t, growFn)
	main := inst.ExportedFunction("main")
	require.NotNil(t, main)

	_, err = main.Call(testCtx)
	require.NoError(t, err)
}

func testCall(t *testing.T, r wazero.Runtime) {
	// Define a basic function which defines two parameters and two results.
	// This is used to test results when incorrect arity is used.
	bin := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection: []wasm.FunctionType{
			{
				Params:            []wasm.ValueType{i64, i64},
				Results:           []wasm.ValueType{i64, i64},
				ParamNumInUint64:  2,
				ResultNumInUint64: 2,
			},
		},
		FunctionSection: []wasm.Index{0},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeEnd}},
		},
		ExportSection: []wasm.Export{{Name: "func", Type: wasm.ExternTypeFunc, Index: 0}},
	})

	inst, err := r.Instantiate(testCtx, bin)
	require.NoError(t, err)

	// Ensure the base case doesn't fail: A single parameter should work as that matches the function signature.
	f := inst.ExportedFunction("func")
	require.NotNil(t, f)

	t.Run("call with stack", func(t *testing.T) {
		stack := []uint64{1, 2}
		err = f.CallWithStack(testCtx, stack)
		require.NoError(t, err)
		require.Equal(t, []uint64{1, 2}, stack)

		t.Run("errs when not enough parameters", func(t *testing.T) {
			err = f.CallWithStack(testCtx, nil)
			require.EqualError(t, err, "need 2 params, but stack size is 0")
		})
	})

	t.Run("errs when not enough parameters", func(t *testing.T) {
		_, err = f.Call(testCtx)
		require.EqualError(t, err, "expected 2 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		_, err = f.Call(testCtx, 1, 2, 3)
		require.EqualError(t, err, "expected 2 params, but passed 3")
	})
}

// RunTestModuleEngineMemory shows that the byte slice returned from api.Memory Read is not a copy, rather a re-slice
// of the underlying memory. This allows both host and Wasm to see each other's writes, unless one side changes the
// capacity of the slice.
//
// Known cases that change the slice capacity:
// * Host code calls append on a byte slice returned by api.Memory Read
// * Wasm code calls wasm.OpcodeMemoryGrowName and this changes the capacity (by default, it will).
func testModuleMemory(t *testing.T, r wazero.Runtime) {
	wasmPhrase := "Well, that'll be the day when you say goodbye."
	wasmPhraseSize := uint32(len(wasmPhrase))

	one := uint32(1)

	bin := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{{Params: []api.ValueType{api.ValueTypeI32}, ParamNumInUint64: 1}, {}},
		FunctionSection: []wasm.Index{0, 1},
		MemorySection:   &wasm.Memory{Min: 1, Cap: 1, Max: 2},
		DataSection: []wasm.DataSegment{
			{
				Passive: true,
				Init:    []byte(wasmPhrase),
			},
		},
		DataCountSection: &one,
		CodeSection: []wasm.Code{
			{Body: []byte{ // "grow"
				wasm.OpcodeLocalGet, 0, // how many pages to grow (param)
				wasm.OpcodeMemoryGrow, 0, // memory index zero
				wasm.OpcodeDrop, // drop the previous page count (or -1 if grow failed)
				wasm.OpcodeEnd,
			}},
			{Body: []byte{ // "init"
				wasm.OpcodeI32Const, 0, // target offset
				wasm.OpcodeI32Const, 0, // source offset
				wasm.OpcodeI32Const, byte(wasmPhraseSize), // len
				wasm.OpcodeMiscPrefix, wasm.OpcodeMiscMemoryInit, 0, 0, // segment 0, memory 0
				wasm.OpcodeEnd,
			}},
		},
		ExportSection: []wasm.Export{
			{Name: "grow", Type: wasm.ExternTypeFunc, Index: 0},
			{Name: "init", Type: wasm.ExternTypeFunc, Index: 1},
		},
	})

	inst, err := r.Instantiate(testCtx, bin)
	require.NoError(t, err)

	memory := inst.Memory()

	buf, ok := memory.Read(0, wasmPhraseSize)
	require.True(t, ok)
	require.Equal(t, make([]byte, wasmPhraseSize), buf)

	// Initialize the memory using Wasm. This copies the test phrase.
	initCallEngine := inst.ExportedFunction("init")
	_, err = initCallEngine.Call(testCtx)
	require.NoError(t, err)

	// We expect the same []byte read earlier to now include the phrase in wasm.
	require.Equal(t, wasmPhrase, string(buf))

	hostPhrase := "Goodbye, cruel world. I'm off to join the circus." // Intentionally slightly longer.
	hostPhraseSize := uint32(len(hostPhrase))

	// Copy over the buffer, which should stop at the current length.
	copy(buf, hostPhrase)
	require.Equal(t, "Goodbye, cruel world. I'm off to join the circ", string(buf))

	// The underlying memory should be updated. This proves that Memory.Read returns a re-slice, not a copy, and that
	// programs can rely on this (for example, to update shared state in Wasm and view that in Go and visa versa).
	buf2, ok := memory.Read(0, wasmPhraseSize)
	require.True(t, ok)
	require.Equal(t, buf, buf2)

	// Now, append to the buffer we got from Wasm. As this changes capacity, it should result in a new byte slice.
	buf = append(buf, 'u', 's', '.')
	require.Equal(t, hostPhrase, string(buf))

	// To prove the above, we re-read the memory and should not see the appended bytes (rather zeros instead).
	buf2, ok = memory.Read(0, hostPhraseSize)
	require.True(t, ok)
	hostPhraseTruncated := "Goodbye, cruel world. I'm off to join the circ" + string([]byte{0, 0, 0})
	require.Equal(t, hostPhraseTruncated, string(buf2))

	// Now, we need to prove the other direction, that when Wasm changes the capacity, the host's buffer is unaffected.
	growCallEngine := inst.ExportedFunction("grow")
	_, err = growCallEngine.Call(testCtx, 1)
	require.NoError(t, err)

	// The host buffer should still contain the same bytes as before grow
	require.Equal(t, hostPhraseTruncated, string(buf2))

	// Re-initialize the memory in wasm, which overwrites the region.
	initCallEngine2 := inst.ExportedFunction("init")
	_, err = initCallEngine2.Call(testCtx)
	require.NoError(t, err)

	// The host was not affected because it is a different slice due to "memory.grow" affecting the underlying memory.
	require.Equal(t, hostPhraseTruncated, string(buf2))
}

func testTwoIndirection(t *testing.T, r wazero.Runtime) {
	var buf bytes.Buffer
	ctx := context.WithValue(testCtx, experimental.FunctionListenerFactoryKey{}, logging.NewLoggingListenerFactory(&buf))
	_, err := r.NewHostModuleBuilder("host").NewFunctionBuilder().WithFunc(func(d uint32) uint32 {
		if d == math.MaxUint32 {
			panic(errors.New("host-function panic"))
		}
		return 1 / d // panics if d ==0.
	}).Export("div").Instantiate(ctx)
	require.NoError(t, err)

	ft := wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}}
	hostImporter := binaryencoding.EncodeModule(&wasm.Module{
		ImportSection:   []wasm.Import{{Module: "host", Name: "div", DescFunc: 0}},
		TypeSection:     []wasm.FunctionType{ft},
		FunctionSection: []wasm.Index{0},
		CodeSection: []wasm.Code{
			// Calling imported host function ^.
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, 0, wasm.OpcodeEnd}},
		},
		ExportSection: []wasm.Export{{Name: "call_host_div", Type: wasm.ExternTypeFunc, Index: 1}},
		NameSection: &wasm.NameSection{
			ModuleName:    "host_importer",
			FunctionNames: wasm.NameMap{{Index: wasm.Index(1), Name: "call_host_div"}},
		},
	})

	_, err = r.Instantiate(ctx, hostImporter)
	require.NoError(t, err)

	main := binaryencoding.EncodeModule(&wasm.Module{
		ImportFunctionCount: 1,
		TypeSection:         []wasm.FunctionType{ft},
		ImportSection:       []wasm.Import{{Module: "host_importer", Name: "call_host_div", DescFunc: 0}},
		FunctionSection:     []wasm.Index{0},
		CodeSection:         []wasm.Code{{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, 0, wasm.OpcodeEnd}}},
		ExportSection:       []wasm.Export{{Name: "main", Type: wasm.ExternTypeFunc, Index: 1}},
		NameSection:         &wasm.NameSection{ModuleName: "main", FunctionNames: wasm.NameMap{{Index: wasm.Index(1), Name: "main"}}},
	})

	inst, err := r.Instantiate(ctx, main)
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {
		mainFn := inst.ExportedFunction("main")
		require.NotNil(t, mainFn)

		result1, err := mainFn.Call(testCtx, 1)
		require.NoError(t, err)

		result2, err := mainFn.Call(testCtx, 2)
		require.NoError(t, err)

		require.Equal(t, uint64(1), result1[0])
		require.Equal(t, uint64(0), result2[0])
	})

	t.Run("errors", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			input  uint64
			expErr string
		}{
			{name: "host panic", input: math.MaxUint32, expErr: `host-function panic (recovered by wazero)
wasm stack trace:
	host.div(i32) i32
	host_importer.call_host_div(i32) i32
	main.main(i32) i32`},
			{name: "go runtime panic", input: 0, expErr: `runtime error: integer divide by zero (recovered by wazero)
wasm stack trace:
	host.div(i32) i32
	host_importer.call_host_div(i32) i32
	main.main(i32) i32`},
		} {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				mainFn := inst.ExportedFunction("main")
				require.NotNil(t, mainFn)

				_, err := mainFn.Call(testCtx, tc.input)
				require.Error(t, err)
				errStr := err.Error()
				// If this faces a Go runtime error, the error includes the Go stack trace which makes the test unstable,
				// so we trim them here.
				if index := strings.Index(errStr, wasmdebug.GoRuntimeErrorTracePrefix); index > -1 {
					errStr = strings.TrimSpace(errStr[:index])
				}
				require.Equal(t, errStr, tc.expErr)
			})
		}
	})

	require.Equal(t, `
--> main.main(1)
	--> host_importer.call_host_div(1)
		==> host.div(1)
		<== 1
	<-- 1
<-- 1
--> main.main(2)
	--> host_importer.call_host_div(2)
		==> host.div(2)
		<== 0
	<-- 0
<-- 0
--> main.main(-1)
	--> host_importer.call_host_div(-1)
		==> host.div(-1)
--> main.main(0)
	--> host_importer.call_host_div(0)
		==> host.div(0)
`, "\n"+buf.String())
}

func testBeforeListenerGlobals(t *testing.T, r wazero.Runtime) {
	type globals struct {
		values []uint64
		types  []api.ValueType
	}

	expectedGlobals := []globals{
		{values: []uint64{100, 200}, types: []api.ValueType{api.ValueTypeI32, api.ValueTypeI32}},
		{values: []uint64{42, 11}, types: []api.ValueType{api.ValueTypeI32, api.ValueTypeI32}},
	}

	fnListener := &fnListener{
		beforeFn: func(ctx context.Context, mod api.Module, def api.FunctionDefinition, params []uint64, si experimental.StackIterator) {
			require.True(t, len(expectedGlobals) > 0)

			imod := mod.(experimental.InternalModule)
			expected := expectedGlobals[0]

			require.Equal(t, len(expected.values), imod.NumGlobal())
			for i := 0; i < imod.NumGlobal(); i++ {
				global := imod.Global(i)
				require.Equal(t, expected.types[i], global.Type())
				require.Equal(t, expected.values[i], global.Get())
			}

			expectedGlobals = expectedGlobals[1:]
		},
	}

	buf := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		FunctionSection: []wasm.Index{0, 0},
		GlobalSection: []wasm.Global{
			{
				Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true},
				Init: wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(100)},
			},
			{
				Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true},
				Init: wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(200)},
			},
		},
		CodeSection: []wasm.Code{
			{
				Body: []byte{
					wasm.OpcodeI32Const, 42,
					wasm.OpcodeGlobalSet, 0, // store 42 in global 0
					wasm.OpcodeI32Const, 11,
					wasm.OpcodeGlobalSet, 1, // store 11 in global 1
					wasm.OpcodeCall, 1, // call f2
					wasm.OpcodeEnd,
				},
			},
			{Body: []byte{wasm.OpcodeEnd}},
		},
		ExportSection: []wasm.Export{{Name: "f", Type: wasm.ExternTypeFunc, Index: 0}},
	})

	ctx := context.WithValue(testCtx, experimental.FunctionListenerFactoryKey{}, fnListener)
	inst, err := r.Instantiate(ctx, buf)
	require.NoError(t, err)

	f := inst.ExportedFunction("f")
	require.NotNil(t, f)

	_, err = f.Call(ctx)
	require.NoError(t, err)
	require.True(t, len(expectedGlobals) == 0)
}

// testBeforeListenerStackIterator tests that the StackIterator provided by the Engine to the Before hook
// of the listener is properly able to walk the stack.
func testBeforeListenerStackIterator(t *testing.T, r wazero.Runtime) {
	type stackEntry struct {
		debugName string
	}

	expectedCallstacks := [][]stackEntry{
		{ // when calling f1
			{debugName: "whatever.f1"},
		},
		{ // when calling f2
			{debugName: "whatever.f2"},
			{debugName: "whatever.f1"},
		},
		{ // when calling
			{debugName: "whatever.f3"},
			{debugName: "whatever.f2"},
			{debugName: "whatever.f1"},
		},
		{ // when calling f4
			{debugName: "host.f4"},
			{debugName: "whatever.f3"},
			{debugName: "whatever.f2"},
			{debugName: "whatever.f1"},
		},
	}

	fnListener := &fnListener{
		beforeFn: func(ctx context.Context, mod api.Module, def api.FunctionDefinition, params []uint64, si experimental.StackIterator) {
			require.True(t, len(expectedCallstacks) > 0)
			expectedCallstack := expectedCallstacks[0]
			for si.Next() {
				require.True(t, len(expectedCallstack) > 0)
				require.Equal(t, expectedCallstack[0].debugName, si.Function().Definition().DebugName())
				expectedCallstack = expectedCallstack[1:]
			}
			require.Equal(t, 0, len(expectedCallstack))
			expectedCallstacks = expectedCallstacks[1:]
		},
	}

	ctx := context.WithValue(testCtx, experimental.FunctionListenerFactoryKey{}, fnListener)
	_, err := r.NewHostModuleBuilder("host").NewFunctionBuilder().WithFunc(func(x int32) int32 {
		return x + 100
	}).Export("f4").Instantiate(ctx)
	require.NoError(t, err)

	m := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection: []wasm.FunctionType{
			// f1 type
			{
				Params:  []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32},
				Results: []api.ValueType{},
			},
			// f2 type
			{
				Params:  []api.ValueType{},
				Results: []api.ValueType{api.ValueTypeI32},
			},
			// f3 type
			{
				Params:  []api.ValueType{api.ValueTypeI32},
				Results: []api.ValueType{api.ValueTypeI32},
			},
			// f4 type
			{
				Params:  []api.ValueType{api.ValueTypeI32},
				Results: []api.ValueType{api.ValueTypeI32},
			},
		},
		ImportFunctionCount: 1,
		ImportSection:       []wasm.Import{{Name: "f4", Module: "host", DescFunc: 3}},
		FunctionSection:     []wasm.Index{0, 1, 2},
		NameSection: &wasm.NameSection{
			ModuleName: "whatever",
			FunctionNames: wasm.NameMap{
				{Index: wasm.Index(1), Name: "f1"},
				{Index: wasm.Index(2), Name: "f2"},
				{Index: wasm.Index(3), Name: "f3"},
				{Index: wasm.Index(0), Name: "f4"},
			},
		},
		CodeSection: []wasm.Code{
			{ // f1
				Body: []byte{
					wasm.OpcodeCall,
					2, // call f2
					wasm.OpcodeDrop,
					wasm.OpcodeEnd,
				},
			},
			{ // f2
				LocalTypes: []wasm.ValueType{wasm.ValueTypeI32},
				Body: []byte{
					wasm.OpcodeI32Const, 42, // local for f2
					wasm.OpcodeLocalSet, 0,
					wasm.OpcodeI32Const, 5, // argument of f3
					wasm.OpcodeCall,
					3, // call f3
					wasm.OpcodeEnd,
				},
			},
			{ // f3
				Body: []byte{
					wasm.OpcodeI32Const, 6,
					wasm.OpcodeCall,
					0, // call host function
					wasm.OpcodeEnd,
				},
			},
		},
		ExportSection: []wasm.Export{{Name: "f1", Type: wasm.ExternTypeFunc, Index: 1}},
	})

	inst, err := r.Instantiate(ctx, m)
	require.NoError(t, err)

	f1 := inst.ExportedFunction("f1")
	require.NotNil(t, f1)

	_, err = f1.Call(ctx, 2, 3, 4)
	require.NoError(t, err)
	require.Equal(t, 0, len(expectedCallstacks))
}

func testListenerStackIteratorOffset(t *testing.T, r wazero.Runtime) {
	type frame struct {
		function api.FunctionDefinition
		offset   uint64
	}

	var tape [][]frame
	fnListener := &fnListener{
		beforeFn: func(ctx context.Context, mod api.Module, def api.FunctionDefinition, params []uint64, si experimental.StackIterator) {
			var stack []frame
			for si.Next() {
				fn := si.Function()
				pc := si.ProgramCounter()
				stack = append(stack, frame{fn.Definition(), fn.SourceOffsetForPC(pc)})
			}
			tape = append(tape, stack)
		},
	}
	ctx := context.WithValue(testCtx, experimental.FunctionListenerFactoryKey{}, fnListener)

	// Minimal DWARF info section to make debug/dwarf.New() happy.
	// Necessary to make the compiler emit source offset maps.
	minimalDWARFInfo := []byte{
		0x7, 0x0, 0x0, 0x0, // length (len(info) - 4)
		0x3, 0x0, // version (between 3 and 5 makes it easier)
		0x0, 0x0, 0x0, 0x0, // abbrev offset
		0x0, // asize
	}

	encoded := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection: []wasm.FunctionType{
			// f1 type
			{Params: []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32}},
			// f2 type
			{Results: []api.ValueType{api.ValueTypeI32}},
			// f3 type
			{Params: []api.ValueType{api.ValueTypeI32}, Results: []api.ValueType{api.ValueTypeI32}},
		},
		FunctionSection: []wasm.Index{0, 1, 2},
		NameSection: &wasm.NameSection{
			ModuleName: "whatever",
			FunctionNames: wasm.NameMap{
				{Index: wasm.Index(0), Name: "f1"},
				{Index: wasm.Index(1), Name: "f2"},
				{Index: wasm.Index(2), Name: "f3"},
			},
		},
		CodeSection: []wasm.Code{
			{ // f1
				Body: []byte{
					wasm.OpcodeI32Const, 42,
					wasm.OpcodeLocalSet, 0,
					wasm.OpcodeI32Const, 11,
					wasm.OpcodeLocalSet, 1,
					wasm.OpcodeCall, 1, // call f2
					wasm.OpcodeDrop,
					wasm.OpcodeEnd,
				},
			},
			{
				Body: []byte{
					wasm.OpcodeI32Const, 6,
					wasm.OpcodeCall, 2, // call f3
					wasm.OpcodeEnd,
				},
			},
			{Body: []byte{wasm.OpcodeI32Const, 15, wasm.OpcodeEnd}},
		},
		ExportSection: []wasm.Export{
			{Name: "f1", Type: wasm.ExternTypeFunc, Index: 0},
			{Name: "f2", Type: wasm.ExternTypeFunc, Index: 1},
			{Name: "f3", Type: wasm.ExternTypeFunc, Index: 2},
		},
		CustomSections: []*wasm.CustomSection{{Name: ".debug_info", Data: minimalDWARFInfo}},
	})
	decoded, err := binary.DecodeModule(encoded, api.CoreFeaturesV2, 0, false, true, true)
	require.NoError(t, err)

	f1offset := decoded.CodeSection[0].BodyOffsetInCodeSection
	f2offset := decoded.CodeSection[1].BodyOffsetInCodeSection
	f3offset := decoded.CodeSection[2].BodyOffsetInCodeSection

	inst, err := r.Instantiate(ctx, encoded)
	require.NoError(t, err)

	f1Fn := inst.ExportedFunction("f1")
	require.NotNil(t, f1Fn)

	_, err = f1Fn.Call(ctx, 2, 3, 4)
	require.NoError(t, err)

	module, ok := inst.(*wasm.ModuleInstance)
	require.True(t, ok)

	defs := module.ExportedFunctionDefinitions()
	f1 := defs["f1"]
	f2 := defs["f2"]
	f3 := defs["f3"]
	t.Logf("f1 offset: %#x", f1offset)
	t.Logf("f2 offset: %#x", f2offset)
	t.Logf("f3 offset: %#x", f3offset)

	expectedStacks := [][]frame{
		{
			{f1, f1offset + 0},
		},
		{
			{f2, f2offset + 0},
			{f1, f1offset + 8}, // index of call opcode in f1's code
		},
		{
			{f3, f3offset},     // host functions don't have a wasm code offset
			{f2, f2offset + 2}, // index of call opcode in f2's code
			{f1, f1offset + 8}, // index of call opcode in f1's code
		},
	}

	for si, stack := range tape {
		t.Log("Recorded stack", si, ":")
		require.True(t, len(expectedStacks) > 0, "more recorded stacks than expected stacks")
		expectedStack := expectedStacks[0]
		expectedStacks = expectedStacks[1:]
		for fi, frame := range stack {
			t.Logf("\t%d -> %s :: %#x", fi, frame.function.Name(), frame.offset)
			require.True(t, len(expectedStack) > 0, "more frames in stack than expected")
			expectedFrame := expectedStack[0]
			expectedStack = expectedStack[1:]
			require.Equal(t, expectedFrame, frame)
		}
		require.Zero(t, len(expectedStack), "expected more frames in stack")
	}
	require.Zero(t, len(expectedStacks), "expected more stacks")
}

// fnListener implements both experimental.FunctionListenerFactory and experimental.FunctionListener for testing.
type fnListener struct {
	beforeFn func(context.Context, api.Module, api.FunctionDefinition, []uint64, experimental.StackIterator)
	afterFn  func(context.Context, api.Module, api.FunctionDefinition, []uint64)
	abortFn  func(context.Context, api.Module, api.FunctionDefinition, any)
}

// NewFunctionListener implements experimental.FunctionListenerFactory.
func (f *fnListener) NewFunctionListener(api.FunctionDefinition) experimental.FunctionListener {
	return f
}

// Before implements experimental.FunctionListener.
func (f *fnListener) Before(ctx context.Context, mod api.Module, def api.FunctionDefinition, params []uint64, stackIterator experimental.StackIterator) {
	if f.beforeFn != nil {
		f.beforeFn(ctx, mod, def, params, stackIterator)
	}
}

// After implements experimental.FunctionListener.
func (f *fnListener) After(ctx context.Context, mod api.Module, def api.FunctionDefinition, results []uint64) {
	if f.afterFn != nil {
		f.afterFn(ctx, mod, def, results)
	}
}

// Abort implements experimental.FunctionListener.
func (f *fnListener) Abort(ctx context.Context, mod api.Module, def api.FunctionDefinition, err error) {
	if f.abortFn != nil {
		f.abortFn(ctx, mod, def, err)
	}
}
