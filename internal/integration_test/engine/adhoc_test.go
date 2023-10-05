package adhoc

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
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
	"huge stack":                                                       {f: testHugeStack},
	"unreachable":                                                      {f: testUnreachable},
	"recursive entry":                                                  {f: testRecursiveEntry},
	"host func memory":                                                 {f: testHostFuncMemory},
	"host function with context parameter":                             {f: testHostFunctionContextParameter},
	"host function with nested context":                                {f: testNestedGoContext},
	"host function with numeric parameter":                             {f: testHostFunctionNumericParameter},
	"close module with in-flight calls":                                {f: testCloseInFlight},
	"multiple instantiation from same source":                          {f: testMultipleInstantiation},
	"exported function that grows memory":                              {f: testMemOps},
	"import functions with reference type in signature":                {f: testReftypeImports},
	"overflow integer addition":                                        {f: testOverflow},
	"un-signed extend global":                                          {f: testGlobalExtend},
	"user-defined primitive in host func":                              {f: testUserDefinedPrimitiveHostFunc},
	"ensures invocations terminate on module close":                    {f: testEnsureTerminationOnClose},
	"call host function indirectly":                                    {f: callHostFunctionIndirect},
	"lookup function":                                                  {f: testLookupFunction},
	"memory grow in recursive call":                                    {f: testMemoryGrowInRecursiveCall},
	"call":                                                             {f: testCall},
	"module memory":                                                    {f: testModuleMemory},
	"two indirection to host":                                          {f: testTwoIndirection},
	"before listener globals":                                          {f: testBeforeListenerGlobals},
	"before listener stack iterator":                                   {f: testBeforeListenerStackIterator},
	"before listener stack iterator offsets":                           {f: testListenerStackIteratorOffset},
	"many params many results / doubler":                               {f: testManyParamsResultsDoubler},
	"many params many results / doubler / listener":                    {f: testManyParamsResultsDoublerListener},
	"many params many results / call_many_consts":                      {f: testManyParamsResultsCallManyConsts},
	"many params many results / call_many_consts / listener":           {f: testManyParamsResultsCallManyConstsListener},
	"many params many results / swapper":                               {f: testManyParamsResultsSwapper},
	"many params many results / swapper / listener":                    {f: testManyParamsResultsSwapperListener},
	"many params many results / main":                                  {f: testManyParamsResultsMain},
	"many params many results / main / listener":                       {f: testManyParamsResultsMainListener},
	"many params many results / call_many_consts_and_pick_last_vector": {f: testManyParamsResultsCallManyConstsAndPickLastVector},
	"many params many results / call_many_consts_and_pick_last_vector / listener": {f: testManyParamsResultsCallManyConstsAndPickLastVectorListener},
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

const i32, i64, f32, f64, v128 = wasm.ValueTypeI32, wasm.ValueTypeI64, wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeV128

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

func manyParamsResultsMod() (bin []byte, params []uint64) {
	mainType := wasm.FunctionType{}
	swapperType := wasm.FunctionType{}
	doublerType := wasm.FunctionType{}
	manyConstsType := wasm.FunctionType{}
	callManyConstsType := wasm.FunctionType{}
	pickLastVectorType := wasm.FunctionType{Results: []wasm.ValueType{v128}}
	callManyConstsAndPickLastVectorType := wasm.FunctionType{Results: []wasm.ValueType{v128}}
	for i := 0; i < 20; i++ {
		swapperType.Params = append(swapperType.Params, i32, i64, f32, f64, v128)
		swapperType.Results = append(swapperType.Results, v128, f64, f32, i64, i32)
		mainType.Params = append(mainType.Params, i32, i64, f32, f64, v128)
		mainType.Results = append(mainType.Results, v128, f64, f32, i64, i32)
		doublerType.Params = append(doublerType.Results, v128, f64, f32, i64, i32)
		doublerType.Results = append(doublerType.Results, v128, f64, f32, i64, i32)
		manyConstsType.Results = append(manyConstsType.Results, i32, i64, f32, f64, v128)
		callManyConstsType.Results = append(callManyConstsType.Results, i32, i64, f32, f64, v128)
		pickLastVectorType.Params = append(pickLastVectorType.Params, i32, i64, f32, f64, v128)
	}

	var mainBody []byte
	for i := 0; i < 100; i++ {
		mainBody = append(mainBody, wasm.OpcodeLocalGet)
		mainBody = append(mainBody, leb128.EncodeUint32(uint32(i))...)
	}
	mainBody = append(mainBody, wasm.OpcodeCall, 1) // Call swapper.
	mainBody = append(mainBody, wasm.OpcodeCall, 2) // Call doubler.
	mainBody = append(mainBody, wasm.OpcodeEnd)

	var swapperBody []byte
	for i := 0; i < 100; i++ {
		swapperBody = append(swapperBody, wasm.OpcodeLocalGet)
		swapperBody = append(swapperBody, leb128.EncodeUint32(uint32(99-i))...)
	}
	swapperBody = append(swapperBody, wasm.OpcodeEnd)

	var doublerBody []byte
	for i := 0; i < 100; i += 5 {
		// Returns v128 as-is.
		doublerBody = append(doublerBody, wasm.OpcodeLocalGet)
		doublerBody = append(doublerBody, leb128.EncodeUint32(uint32(i))...)
		// Double f64.
		doublerBody = append(doublerBody, wasm.OpcodeLocalGet)
		doublerBody = append(doublerBody, leb128.EncodeUint32(uint32(i+1))...)
		doublerBody = append(doublerBody, wasm.OpcodeLocalGet)
		doublerBody = append(doublerBody, leb128.EncodeUint32(uint32(i+1))...)
		doublerBody = append(doublerBody, wasm.OpcodeF64Add)
		// Double f32.
		doublerBody = append(doublerBody, wasm.OpcodeLocalGet)
		doublerBody = append(doublerBody, leb128.EncodeUint32(uint32(i+2))...)
		doublerBody = append(doublerBody, wasm.OpcodeLocalGet)
		doublerBody = append(doublerBody, leb128.EncodeUint32(uint32(i+2))...)
		doublerBody = append(doublerBody, wasm.OpcodeF32Add)
		// Double i64.
		doublerBody = append(doublerBody, wasm.OpcodeLocalGet)
		doublerBody = append(doublerBody, leb128.EncodeUint32(uint32(i+3))...)
		doublerBody = append(doublerBody, wasm.OpcodeLocalGet)
		doublerBody = append(doublerBody, leb128.EncodeUint32(uint32(i+3))...)
		doublerBody = append(doublerBody, wasm.OpcodeI64Add)
		// Double i32.
		doublerBody = append(doublerBody, wasm.OpcodeLocalGet)
		doublerBody = append(doublerBody, leb128.EncodeUint32(uint32(i+4))...)
		doublerBody = append(doublerBody, wasm.OpcodeLocalGet)
		doublerBody = append(doublerBody, leb128.EncodeUint32(uint32(i+4))...)
		doublerBody = append(doublerBody, wasm.OpcodeI32Add)
	}
	doublerBody = append(doublerBody, wasm.OpcodeEnd)

	var manyConstsBody []byte
	for i := 0; i < 100; i += 5 {
		ib := byte(i)
		manyConstsBody = append(manyConstsBody, wasm.OpcodeI32Const)
		manyConstsBody = append(manyConstsBody, leb128.EncodeInt32(int32(i))...)
		manyConstsBody = append(manyConstsBody, wasm.OpcodeI64Const)
		manyConstsBody = append(manyConstsBody, leb128.EncodeInt64(int64(i))...)
		manyConstsBody = append(manyConstsBody, wasm.OpcodeF32Const)
		manyConstsBody = append(manyConstsBody, ib, ib, ib, ib)
		manyConstsBody = append(manyConstsBody, wasm.OpcodeF64Const)
		manyConstsBody = append(manyConstsBody, ib, ib, ib, ib, ib, ib, ib, ib)
		manyConstsBody = append(manyConstsBody, wasm.OpcodeVecPrefix, wasm.OpcodeVecV128Const)
		manyConstsBody = append(manyConstsBody, ib, ib, ib, ib, ib, ib, ib, ib, ib, ib, ib, ib, ib, ib, ib, ib)
	}
	manyConstsBody = append(manyConstsBody, wasm.OpcodeEnd)

	var callManyConstsBody []byte
	callManyConstsBody = append(callManyConstsBody, wasm.OpcodeCall, 5, wasm.OpcodeEnd)

	var pickLastVector []byte
	pickLastVector = append(pickLastVector, wasm.OpcodeLocalGet, 99, wasm.OpcodeEnd)

	var callManyConstsAndPickLastVector []byte
	callManyConstsAndPickLastVector = append(callManyConstsAndPickLastVector, wasm.OpcodeCall, 5, wasm.OpcodeCall, 6, wasm.OpcodeEnd)

	nameSection := wasm.NameSection{}
	nameSection.FunctionNames = []wasm.NameAssoc{
		{Index: 0, Name: "main"},
		{Index: 1, Name: "swapper"},
		{Index: 2, Name: "doubler"},
		{Index: 3, Name: "call_many_consts"},
		{Index: 4, Name: "call_many_consts_and_pick_last_vector"},
		{Index: 5, Name: "many_consts"},
		{Index: 6, Name: "pick_last_vector"},
	}

	typeSection := []wasm.FunctionType{mainType, swapperType, doublerType, callManyConstsType, callManyConstsAndPickLastVectorType, manyConstsType, pickLastVectorType}

	for i, typ := range typeSection {
		paramNames := wasm.NameMapAssoc{Index: wasm.Index(i)}
		for paramIndex, paramType := range typ.Params {
			name := fmt.Sprintf("[%d:%s]", paramIndex, wasm.ValueTypeName(paramType))
			paramNames.NameMap = append(paramNames.NameMap, wasm.NameAssoc{Index: wasm.Index(paramIndex), Name: name})
		}
		nameSection.LocalNames = append(nameSection.LocalNames, paramNames)
	}

	bin = binaryencoding.EncodeModule(&wasm.Module{
		TypeSection: typeSection,
		ExportSection: []wasm.Export{
			{Name: "main", Type: wasm.ExternTypeFunc, Index: 0},
			{Name: "swapper", Type: wasm.ExternTypeFunc, Index: 1},
			{Name: "doubler", Type: wasm.ExternTypeFunc, Index: 2},
			{Name: "call_many_consts", Type: wasm.ExternTypeFunc, Index: 3},
			{Name: "call_many_consts_and_pick_last_vector", Type: wasm.ExternTypeFunc, Index: 4},
		},
		FunctionSection: []wasm.Index{0, 1, 2, 3, 4, 5, 6},
		CodeSection: []wasm.Code{
			{Body: mainBody},
			{Body: swapperBody},
			{Body: doublerBody},
			{Body: callManyConstsBody},
			{Body: callManyConstsAndPickLastVector},
			{Body: manyConstsBody},
			{Body: pickLastVector},
		},
		NameSection: &nameSection,
	})

	for i := 0; i < 100; i += 5 {
		params = append(params, uint64(i))
		params = append(params, uint64(i+1))
		params = append(params, uint64(i+2))
		params = append(params, uint64(i+3))
		// Vector needs two values.
		params = append(params, uint64(i+3))
		params = append(params, uint64(i+3))
	}
	return
}

func testManyParamsResultsCallManyConsts(t *testing.T, r wazero.Runtime) {
	ctx := context.Background()

	bin, _ := manyParamsResultsMod()
	mod, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	main := mod.ExportedFunction("call_many_consts")
	require.NotNil(t, main)

	results, err := main.Call(ctx)
	require.NoError(t, err)

	exp := []uint64{
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5, 0x5, 0x5050505, 0x505050505050505, 0x505050505050505,
		0x505050505050505, 0xa, 0xa, 0xa0a0a0a, 0xa0a0a0a0a0a0a0a, 0xa0a0a0a0a0a0a0a, 0xa0a0a0a0a0a0a0a,
		0xf, 0xf, 0xf0f0f0f, 0xf0f0f0f0f0f0f0f, 0xf0f0f0f0f0f0f0f, 0xf0f0f0f0f0f0f0f, 0x14, 0x14, 0x14141414,
		0x1414141414141414, 0x1414141414141414, 0x1414141414141414, 0x19, 0x19, 0x19191919, 0x1919191919191919,
		0x1919191919191919, 0x1919191919191919, 0x1e, 0x1e, 0x1e1e1e1e, 0x1e1e1e1e1e1e1e1e, 0x1e1e1e1e1e1e1e1e,
		0x1e1e1e1e1e1e1e1e, 0x23, 0x23, 0x23232323, 0x2323232323232323, 0x2323232323232323, 0x2323232323232323,
		0x28, 0x28, 0x28282828, 0x2828282828282828, 0x2828282828282828, 0x2828282828282828, 0x2d, 0x2d, 0x2d2d2d2d,
		0x2d2d2d2d2d2d2d2d, 0x2d2d2d2d2d2d2d2d, 0x2d2d2d2d2d2d2d2d, 0x32, 0x32, 0x32323232, 0x3232323232323232,
		0x3232323232323232, 0x3232323232323232, 0x37, 0x37, 0x37373737, 0x3737373737373737, 0x3737373737373737,
		0x3737373737373737, 0x3c, 0x3c, 0x3c3c3c3c, 0x3c3c3c3c3c3c3c3c, 0x3c3c3c3c3c3c3c3c, 0x3c3c3c3c3c3c3c3c,
		0x41, 0x41, 0x41414141, 0x4141414141414141, 0x4141414141414141, 0x4141414141414141, 0x46, 0x46, 0x46464646,
		0x4646464646464646, 0x4646464646464646, 0x4646464646464646, 0x4b, 0x4b, 0x4b4b4b4b, 0x4b4b4b4b4b4b4b4b,
		0x4b4b4b4b4b4b4b4b, 0x4b4b4b4b4b4b4b4b, 0x50, 0x50, 0x50505050, 0x5050505050505050, 0x5050505050505050,
		0x5050505050505050, 0x55, 0x55, 0x55555555, 0x5555555555555555, 0x5555555555555555, 0x5555555555555555,
		0x5a, 0x5a, 0x5a5a5a5a, 0x5a5a5a5a5a5a5a5a, 0x5a5a5a5a5a5a5a5a, 0x5a5a5a5a5a5a5a5a, 0x5f, 0x5f, 0x5f5f5f5f,
		0x5f5f5f5f5f5f5f5f, 0x5f5f5f5f5f5f5f5f, 0x5f5f5f5f5f5f5f5f,
	}
	require.Equal(t, exp, results)
}

func testManyParamsResultsCallManyConstsListener(t *testing.T, r wazero.Runtime) {
	var buf bytes.Buffer
	ctx := context.WithValue(context.Background(), experimental.FunctionListenerFactoryKey{}, logging.NewLoggingListenerFactory(&buf))

	bin, _ := manyParamsResultsMod()
	mod, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	main := mod.ExportedFunction("call_many_consts")
	require.NotNil(t, main)

	results, err := main.Call(ctx)
	require.NoError(t, err)

	fmt.Println(buf.String())
	require.Equal(t, `
--> .call_many_consts()
	--> .many_consts()
	<-- (0,0,0,0,00000000000000000000000000000000,0,5,7e-45,4.16077606e-316,05050505050505050505050505050505,84215045,361700864190383365,1.4e-44,5e-323,000000000a0a0a0a0a0a0a0a0a0a0a0a,168430090,723401728380766730,6.6463464e-33,7.4e-323,000000000000000f000000000f0f0f0f,252645135,1085102592571150095,7.0533445e-30,3.815736827118017e-236,00000000000000140000000000000014,20,336860180,7.47605e-27,5.964208835435795e-212,14141414141414140000000000000019,25,25,7.914983e-24,9.01285756841504e-188,19191919191919191919191919191919,421075225,30,4.2e-44,2.496465636e-315,1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e,505290270,2170205185142300190,4.9e-44,1.73e-322,00000000232323232323232323232323,589505315,2531906049332683555,8.843688e-18,2e-322,00000000000000280000000028282828,673720360,2893606913523066920,9.334581e-15,3.0654356309538037e-115,000000000000002d000000000000002d,45,757935405,9.8439425e-12,4.4759381595361623e-91,2d2d2d2d2d2d2d2d0000000000000032,50,50,1.0372377e-08,6.749300603603778e-67,32323232323232323232323232323232,842150450,55,7.7e-44,4.576853666e-315,37373737373737373737373737373737,926365495,3978709506094217015,8.4e-44,2.96e-322,000000003c3c3c3c3c3c3c3c3c3c3c3c,1010580540,4340410370284600380,0.01148897,3.2e-322,00000000000000410000000041414141,1094795585,4702111234474983745,12.078431,2.2616345098039214e+06,00000000000000460000000000000046,70,1179010630,12689.568,3.5295369653413445e+30,4646464646464646000000000000004b,75,75,1.3323083e+07,5.2285141982483265e+54,4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b,1263225675,80,1.12e-43,6.657241696e-315,50505050505050505050505050505050)
<-- (0,0,0,0,00000000000000000000000000000000,0,5,7e-45,4.16077606e-316,05050505050505050505050505050505,84215045,361700864190383365,1.4e-44,5e-323,000000000a0a0a0a0a0a0a0a0a0a0a0a,168430090,723401728380766730,6.6463464e-33,7.4e-323,000000000000000f000000000f0f0f0f,252645135,1085102592571150095,7.0533445e-30,3.815736827118017e-236,00000000000000140000000000000014,20,336860180,7.47605e-27,5.964208835435795e-212,14141414141414140000000000000019,25,25,7.914983e-24,9.01285756841504e-188,19191919191919191919191919191919,421075225,30,4.2e-44,2.496465636e-315,1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e,505290270,2170205185142300190,4.9e-44,1.73e-322,00000000232323232323232323232323,589505315,2531906049332683555,8.843688e-18,2e-322,00000000000000280000000028282828,673720360,2893606913523066920,9.334581e-15,3.0654356309538037e-115,000000000000002d000000000000002d,45,757935405,9.8439425e-12,4.4759381595361623e-91,2d2d2d2d2d2d2d2d0000000000000032,50,50,1.0372377e-08,6.749300603603778e-67,32323232323232323232323232323232,842150450,55,7.7e-44,4.576853666e-315,37373737373737373737373737373737,926365495,3978709506094217015,8.4e-44,2.96e-322,000000003c3c3c3c3c3c3c3c3c3c3c3c,1010580540,4340410370284600380,0.01148897,3.2e-322,00000000000000410000000041414141,1094795585,4702111234474983745,12.078431,2.2616345098039214e+06,00000000000000460000000000000046,70,1179010630,12689.568,3.5295369653413445e+30,4646464646464646000000000000004b,75,75,1.3323083e+07,5.2285141982483265e+54,4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b,1263225675,80,1.12e-43,6.657241696e-315,50505050505050505050505050505050)
`, "\n"+buf.String())

	exp := []uint64{
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5, 0x5, 0x5050505, 0x505050505050505, 0x505050505050505,
		0x505050505050505, 0xa, 0xa, 0xa0a0a0a, 0xa0a0a0a0a0a0a0a, 0xa0a0a0a0a0a0a0a, 0xa0a0a0a0a0a0a0a,
		0xf, 0xf, 0xf0f0f0f, 0xf0f0f0f0f0f0f0f, 0xf0f0f0f0f0f0f0f, 0xf0f0f0f0f0f0f0f, 0x14, 0x14, 0x14141414,
		0x1414141414141414, 0x1414141414141414, 0x1414141414141414, 0x19, 0x19, 0x19191919, 0x1919191919191919,
		0x1919191919191919, 0x1919191919191919, 0x1e, 0x1e, 0x1e1e1e1e, 0x1e1e1e1e1e1e1e1e, 0x1e1e1e1e1e1e1e1e,
		0x1e1e1e1e1e1e1e1e, 0x23, 0x23, 0x23232323, 0x2323232323232323, 0x2323232323232323, 0x2323232323232323,
		0x28, 0x28, 0x28282828, 0x2828282828282828, 0x2828282828282828, 0x2828282828282828, 0x2d, 0x2d, 0x2d2d2d2d,
		0x2d2d2d2d2d2d2d2d, 0x2d2d2d2d2d2d2d2d, 0x2d2d2d2d2d2d2d2d, 0x32, 0x32, 0x32323232, 0x3232323232323232,
		0x3232323232323232, 0x3232323232323232, 0x37, 0x37, 0x37373737, 0x3737373737373737, 0x3737373737373737,
		0x3737373737373737, 0x3c, 0x3c, 0x3c3c3c3c, 0x3c3c3c3c3c3c3c3c, 0x3c3c3c3c3c3c3c3c, 0x3c3c3c3c3c3c3c3c,
		0x41, 0x41, 0x41414141, 0x4141414141414141, 0x4141414141414141, 0x4141414141414141, 0x46, 0x46, 0x46464646,
		0x4646464646464646, 0x4646464646464646, 0x4646464646464646, 0x4b, 0x4b, 0x4b4b4b4b, 0x4b4b4b4b4b4b4b4b,
		0x4b4b4b4b4b4b4b4b, 0x4b4b4b4b4b4b4b4b, 0x50, 0x50, 0x50505050, 0x5050505050505050, 0x5050505050505050,
		0x5050505050505050, 0x55, 0x55, 0x55555555, 0x5555555555555555, 0x5555555555555555, 0x5555555555555555,
		0x5a, 0x5a, 0x5a5a5a5a, 0x5a5a5a5a5a5a5a5a, 0x5a5a5a5a5a5a5a5a, 0x5a5a5a5a5a5a5a5a, 0x5f, 0x5f, 0x5f5f5f5f,
		0x5f5f5f5f5f5f5f5f, 0x5f5f5f5f5f5f5f5f, 0x5f5f5f5f5f5f5f5f,
	}
	require.Equal(t, exp, results)
}

func testManyParamsResultsDoubler(t *testing.T, r wazero.Runtime) {
	ctx := context.Background()

	bin, params := manyParamsResultsMod()
	mod, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	main := mod.ExportedFunction("doubler")
	require.NotNil(t, main)

	results, err := main.Call(ctx, params...)
	require.NoError(t, err)

	exp := []uint64{
		0x0, 0x1, 0x4, 0x6, 0x6, 0x6, 0x5, 0x6, 0xe, 0x10, 0x10, 0x10, 0xa,
		0xb, 0x18, 0x1a, 0x1a, 0x1a, 0xf, 0x10, 0x22, 0x24, 0x24, 0x24, 0x14,
		0x15, 0x2c, 0x2e, 0x2e, 0x2e, 0x19, 0x1a, 0x36, 0x38, 0x38, 0x38, 0x1e,
		0x1f, 0x40, 0x42, 0x42, 0x42, 0x23, 0x24, 0x4a, 0x4c, 0x4c, 0x4c, 0x28,
		0x29, 0x54, 0x56, 0x56, 0x56, 0x2d, 0x2e, 0x5e, 0x60, 0x60, 0x60, 0x32,
		0x33, 0x68, 0x6a, 0x6a, 0x6a, 0x37, 0x38, 0x72, 0x74, 0x74, 0x74, 0x3c,
		0x3d, 0x7c, 0x7e, 0x7e, 0x7e, 0x41, 0x42, 0x86, 0x88, 0x88, 0x88, 0x46,
		0x47, 0x90, 0x92, 0x92, 0x92, 0x4b, 0x4c, 0x9a, 0x9c, 0x9c, 0x9c, 0x50,
		0x51, 0xa4, 0xa6, 0xa6, 0xa6, 0x55, 0x56, 0xae, 0xb0, 0xb0, 0xb0, 0x5a,
		0x5b, 0xb8, 0xba, 0xba, 0xba, 0x5f, 0x60, 0xc2, 0xc4, 0xc4, 0xc4,
	}
	require.Equal(t, exp, results)
}

func testManyParamsResultsDoublerListener(t *testing.T, r wazero.Runtime) {
	var buf bytes.Buffer
	ctx := context.WithValue(context.Background(), experimental.FunctionListenerFactoryKey{}, logging.NewLoggingListenerFactory(&buf))

	bin, params := manyParamsResultsMod()
	mod, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	main := mod.ExportedFunction("doubler")
	require.NotNil(t, main)

	results, err := main.Call(ctx, params...)
	require.NoError(t, err)

	fmt.Println(buf.String())
	require.Equal(t, `
--> .doubler([0:v128]=00000000000000000000000000000001,[1:f64]=5e-324,[2:f32]=3e-45,[3:i64]=3,[4:i32]=3,[5:v128]=00000000000000030000000000000005,[6:f64]=2.5e-323,[7:f32]=8e-45,[8:i64]=7,[9:i32]=8,[10:v128]=00000000000000080000000000000008,[11:f64]=4e-323,[12:f32]=1.4e-44,[13:i64]=11,[14:i32]=12,[15:v128]=000000000000000d000000000000000d,[16:f64]=6.4e-323,[17:f32]=1.8e-44,[18:i64]=15,[19:i32]=16,[20:v128]=00000000000000110000000000000012,[21:f64]=9e-323,[22:f32]=2.5e-44,[23:i64]=18,[24:i32]=20,[25:v128]=00000000000000150000000000000016,[26:f64]=1.1e-322,[27:f32]=3.2e-44,[28:i64]=23,[29:i32]=23,[30:v128]=0000000000000019000000000000001a,[31:f64]=1.3e-322,[32:f32]=3.8e-44,[33:i64]=28,[34:i32]=28,[35:v128]=000000000000001c000000000000001e,[36:f64]=1.5e-322,[37:f32]=4.3e-44,[38:i64]=32,[39:i32]=33,[40:v128]=00000000000000210000000000000021,[41:f64]=1.63e-322,[42:f32]=4.9e-44,[43:i64]=36,[44:i32]=37,[45:v128]=00000000000000260000000000000026,[46:f64]=1.9e-322,[47:f32]=5.3e-44,[48:i64]=40,[49:i32]=41,[50:v128]=000000000000002a000000000000002b,[51:f64]=2.1e-322,[52:f32]=6e-44,[53:i64]=43,[54:i32]=45,[55:v128]=000000000000002e000000000000002f,[56:f64]=2.3e-322,[57:f32]=6.7e-44,[58:i64]=48,[59:i32]=48,[60:v128]=00000000000000320000000000000033,[61:f64]=2.5e-322,[62:f32]=7.3e-44,[63:i64]=53,[64:i32]=53,[65:v128]=00000000000000350000000000000037,[66:f64]=2.7e-322,[67:f32]=7.8e-44,[68:i64]=57,[69:i32]=58,[70:v128]=000000000000003a000000000000003a,[71:f64]=2.87e-322,[72:f32]=8.4e-44,[73:i64]=61,[74:i32]=62,[75:v128]=000000000000003f000000000000003f,[76:f64]=3.1e-322,[77:f32]=8.8e-44,[78:i64]=65,[79:i32]=66,[80:v128]=00000000000000430000000000000044,[81:f64]=3.36e-322,[82:f32]=9.5e-44,[83:i64]=68,[84:i32]=70,[85:v128]=00000000000000470000000000000048,[86:f64]=3.56e-322,[87:f32]=1.02e-43,[88:i64]=73,[89:i32]=73,[90:v128]=000000000000004b000000000000004c,[91:f64]=3.75e-322,[92:f32]=1.08e-43,[93:i64]=78,[94:i32]=78,[95:v128]=000000000000004e0000000000000050,[96:f64]=3.95e-322,[97:f32]=1.14e-43,[98:i64]=82,[99:i32]=83)
<-- (00000000000000000000000000000001,5e-324,6e-45,6,6,00000000000000060000000000000005,2.5e-323,8e-45,14,16,00000000000000100000000000000010,8e-323,1.4e-44,11,24,000000000000001a000000000000001a,1.3e-322,3.6e-44,15,16,00000000000000220000000000000024,1.8e-322,5e-44,36,20,0000000000000015000000000000002c,2.17e-322,6.4e-44,46,46,0000000000000019000000000000001a,1.3e-322,7.6e-44,56,56,0000000000000038000000000000001e,1.5e-322,4.3e-44,64,66,00000000000000420000000000000042,3.26e-322,4.9e-44,36,74,000000000000004c000000000000004c,3.75e-322,1.06e-43,40,41,00000000000000540000000000000056,4.25e-322,1.2e-43,86,45,000000000000002e000000000000005e,4.64e-322,1.35e-43,96,96,00000000000000320000000000000033,2.5e-322,1.46e-43,106,106,000000000000006a0000000000000037,2.7e-322,7.8e-44,114,116,00000000000000740000000000000074,5.73e-322,8.4e-44,61,124,000000000000007e000000000000007e,6.23e-322,1.77e-43,65,66,00000000000000860000000000000088,6.7e-322,1.9e-43,136,70,00000000000000470000000000000090,7.1e-322,2.05e-43,146,146,000000000000004b000000000000004c,3.75e-322,2.16e-43,156,156,000000000000009c0000000000000050,3.95e-322,1.14e-43,164,166)
`, "\n"+buf.String())

	exp := []uint64{
		0x0, 0x1, 0x4, 0x6, 0x6, 0x6, 0x5, 0x6, 0xe, 0x10, 0x10, 0x10, 0xa,
		0xb, 0x18, 0x1a, 0x1a, 0x1a, 0xf, 0x10, 0x22, 0x24, 0x24, 0x24, 0x14,
		0x15, 0x2c, 0x2e, 0x2e, 0x2e, 0x19, 0x1a, 0x36, 0x38, 0x38, 0x38, 0x1e,
		0x1f, 0x40, 0x42, 0x42, 0x42, 0x23, 0x24, 0x4a, 0x4c, 0x4c, 0x4c, 0x28,
		0x29, 0x54, 0x56, 0x56, 0x56, 0x2d, 0x2e, 0x5e, 0x60, 0x60, 0x60, 0x32,
		0x33, 0x68, 0x6a, 0x6a, 0x6a, 0x37, 0x38, 0x72, 0x74, 0x74, 0x74, 0x3c,
		0x3d, 0x7c, 0x7e, 0x7e, 0x7e, 0x41, 0x42, 0x86, 0x88, 0x88, 0x88, 0x46,
		0x47, 0x90, 0x92, 0x92, 0x92, 0x4b, 0x4c, 0x9a, 0x9c, 0x9c, 0x9c, 0x50,
		0x51, 0xa4, 0xa6, 0xa6, 0xa6, 0x55, 0x56, 0xae, 0xb0, 0xb0, 0xb0, 0x5a,
		0x5b, 0xb8, 0xba, 0xba, 0xba, 0x5f, 0x60, 0xc2, 0xc4, 0xc4, 0xc4,
	}
	require.Equal(t, exp, results)
}

func testManyParamsResultsSwapper(t *testing.T, r wazero.Runtime) {
	ctx := context.Background()

	bin, params := manyParamsResultsMod()
	mod, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	main := mod.ExportedFunction("swapper")
	require.NotNil(t, main)

	results, err := main.Call(ctx, params...)
	require.NoError(t, err)

	exp := []uint64{
		0x62, 0x62, 0x62, 0x61, 0x60, 0x5f, 0x5d, 0x5d, 0x5d, 0x5c, 0x5b, 0x5a, 0x58, 0x58, 0x58, 0x57,
		0x56, 0x55, 0x53, 0x53, 0x53, 0x52, 0x51, 0x50, 0x4e, 0x4e, 0x4e, 0x4d, 0x4c, 0x4b, 0x49, 0x49,
		0x49, 0x48, 0x47, 0x46, 0x44, 0x44, 0x44, 0x43, 0x42, 0x41, 0x3f, 0x3f, 0x3f, 0x3e, 0x3d, 0x3c,
		0x3a, 0x3a, 0x3a, 0x39, 0x38, 0x37, 0x35, 0x35, 0x35, 0x34, 0x33, 0x32, 0x30, 0x30, 0x30, 0x2f,
		0x2e, 0x2d, 0x2b, 0x2b, 0x2b, 0x2a, 0x29, 0x28, 0x26, 0x26, 0x26, 0x25, 0x24, 0x23, 0x21, 0x21,
		0x21, 0x20, 0x1f, 0x1e, 0x1c, 0x1c, 0x1c, 0x1b, 0x1a, 0x19, 0x17, 0x17, 0x17, 0x16, 0x15, 0x14,
		0x12, 0x12, 0x12, 0x11, 0x10, 0xf, 0xd, 0xd, 0xd, 0xc, 0xb, 0xa, 0x8, 0x8, 0x8, 0x7, 0x6, 0x5,
		0x3, 0x3, 0x3, 0x2, 0x1, 0x0,
	}
	require.Equal(t, exp, results)
}

func testManyParamsResultsSwapperListener(t *testing.T, r wazero.Runtime) {
	var buf bytes.Buffer
	ctx := context.WithValue(context.Background(), experimental.FunctionListenerFactoryKey{}, logging.NewLoggingListenerFactory(&buf))

	bin, params := manyParamsResultsMod()
	mod, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	main := mod.ExportedFunction("swapper")
	require.NotNil(t, main)

	results, err := main.Call(ctx, params...)
	require.NoError(t, err)

	fmt.Println(buf.String())
	require.Equal(t, `
--> .swapper([0:i32]=0,[1:i64]=1,[2:f32]=3e-45,[3:f64]=1.5e-323,[4:v128]=00000000000000030000000000000003,[5:i32]=3,[6:i64]=5,[7:f32]=8e-45,[8:f64]=3.5e-323,[9:v128]=00000000000000080000000000000008,[10:i32]=8,[11:i64]=8,[12:f32]=1.4e-44,[13:f64]=5.4e-323,[14:v128]=000000000000000c000000000000000d,[15:i32]=13,[16:i64]=13,[17:f32]=1.8e-44,[18:f64]=7.4e-323,[19:v128]=00000000000000100000000000000011,[20:i32]=17,[21:i64]=18,[22:f32]=2.5e-44,[23:f64]=9e-323,[24:v128]=00000000000000140000000000000015,[25:i32]=21,[26:i64]=22,[27:f32]=3.2e-44,[28:f64]=1.14e-322,[29:v128]=00000000000000170000000000000019,[30:i32]=25,[31:i64]=26,[32:f32]=3.8e-44,[33:f64]=1.4e-322,[34:v128]=000000000000001c000000000000001c,[35:i32]=28,[36:i64]=30,[37:f32]=4.3e-44,[38:f64]=1.6e-322,[39:v128]=00000000000000210000000000000021,[40:i32]=33,[41:i64]=33,[42:f32]=4.9e-44,[43:f64]=1.8e-322,[44:v128]=00000000000000250000000000000026,[45:i32]=38,[46:i64]=38,[47:f32]=5.3e-44,[48:f64]=2e-322,[49:v128]=0000000000000029000000000000002a,[50:i32]=42,[51:i64]=43,[52:f32]=6e-44,[53:f64]=2.1e-322,[54:v128]=000000000000002d000000000000002e,[55:i32]=46,[56:i64]=47,[57:f32]=6.7e-44,[58:f64]=2.37e-322,[59:v128]=00000000000000300000000000000032,[60:i32]=50,[61:i64]=51,[62:f32]=7.3e-44,[63:f64]=2.6e-322,[64:v128]=00000000000000350000000000000035,[65:i32]=53,[66:i64]=55,[67:f32]=7.8e-44,[68:f64]=2.8e-322,[69:v128]=000000000000003a000000000000003a,[70:i32]=58,[71:i64]=58,[72:f32]=8.4e-44,[73:f64]=3e-322,[74:v128]=000000000000003e000000000000003f,[75:i32]=63,[76:i64]=63,[77:f32]=8.8e-44,[78:f64]=3.2e-322,[79:v128]=00000000000000420000000000000043,[80:i32]=67,[81:i64]=68,[82:f32]=9.5e-44,[83:f64]=3.36e-322,[84:v128]=00000000000000460000000000000047,[85:i32]=71,[86:i64]=72,[87:f32]=1.02e-43,[88:f64]=3.6e-322,[89:v128]=0000000000000049000000000000004b,[90:i32]=75,[91:i64]=76,[92:f32]=1.08e-43,[93:f64]=3.85e-322,[94:v128]=000000000000004e000000000000004e,[95:i32]=78,[96:i64]=80,[97:f32]=1.14e-43,[98:f64]=4.05e-322,[99:v128]=00000000000000530000000000000053)
<-- (00000000000000620000000000000062,4.84e-322,1.37e-43,97,96,000000000000005f000000000000005d,4.6e-322,1.3e-43,93,92,000000000000005b000000000000005a,4.45e-322,1.23e-43,88,88,00000000000000570000000000000056,4.25e-322,1.19e-43,83,83,00000000000000530000000000000052,4.05e-322,1.14e-43,80,78,000000000000004e000000000000004e,3.85e-322,1.08e-43,76,75,00000000000000490000000000000049,3.6e-322,1.02e-43,72,71,00000000000000460000000000000044,3.36e-322,9.5e-44,68,67,00000000000000420000000000000041,3.2e-322,8.8e-44,63,63,000000000000003e000000000000003d,3e-322,8.4e-44,58,58,000000000000003a0000000000000039,2.8e-322,7.8e-44,55,53,00000000000000350000000000000035,2.6e-322,7.3e-44,51,50,00000000000000300000000000000030,2.37e-322,6.7e-44,47,46,000000000000002d000000000000002b,2.1e-322,6e-44,43,42,00000000000000290000000000000028,2e-322,5.3e-44,38,38,00000000000000250000000000000024,1.8e-322,4.9e-44,33,33,00000000000000210000000000000020,1.6e-322,4.3e-44,30,28,000000000000001c000000000000001c,1.4e-322,3.8e-44,26,25,00000000000000170000000000000017,1.14e-322,3.2e-44,22,21,00000000000000140000000000000012,9e-323,2.5e-44,18,17)
`, "\n"+buf.String())

	exp := []uint64{
		0x62, 0x62, 0x62, 0x61, 0x60, 0x5f, 0x5d, 0x5d, 0x5d, 0x5c, 0x5b, 0x5a, 0x58, 0x58, 0x58, 0x57,
		0x56, 0x55, 0x53, 0x53, 0x53, 0x52, 0x51, 0x50, 0x4e, 0x4e, 0x4e, 0x4d, 0x4c, 0x4b, 0x49, 0x49,
		0x49, 0x48, 0x47, 0x46, 0x44, 0x44, 0x44, 0x43, 0x42, 0x41, 0x3f, 0x3f, 0x3f, 0x3e, 0x3d, 0x3c,
		0x3a, 0x3a, 0x3a, 0x39, 0x38, 0x37, 0x35, 0x35, 0x35, 0x34, 0x33, 0x32, 0x30, 0x30, 0x30, 0x2f,
		0x2e, 0x2d, 0x2b, 0x2b, 0x2b, 0x2a, 0x29, 0x28, 0x26, 0x26, 0x26, 0x25, 0x24, 0x23, 0x21, 0x21,
		0x21, 0x20, 0x1f, 0x1e, 0x1c, 0x1c, 0x1c, 0x1b, 0x1a, 0x19, 0x17, 0x17, 0x17, 0x16, 0x15, 0x14,
		0x12, 0x12, 0x12, 0x11, 0x10, 0xf, 0xd, 0xd, 0xd, 0xc, 0xb, 0xa, 0x8, 0x8, 0x8, 0x7, 0x6, 0x5,
		0x3, 0x3, 0x3, 0x2, 0x1, 0x0,
	}
	require.Equal(t, exp, results)
}

func testManyParamsResultsMain(t *testing.T, r wazero.Runtime) {
	ctx := context.Background()

	bin, params := manyParamsResultsMod()
	mod, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	main := mod.ExportedFunction("main")
	require.NotNil(t, main)

	results, err := main.Call(ctx, params...)
	require.NoError(t, err)

	exp := []uint64{
		98, 98, 196, 194, 192, 190, 93, 93, 186, 184, 182, 180, 88, 88, 176, 174, 172, 170, 83, 83, 166, 164, 162,
		160, 78, 78, 156, 154, 152, 150, 73, 73, 146, 144, 142, 140, 68, 68, 136, 134, 132, 130, 63, 63, 126, 124,
		122, 120, 58, 58, 116, 114, 112, 110, 53, 53, 106, 104, 102, 100, 48, 48, 96, 94, 92, 90, 43, 43, 86, 84,
		82, 80, 38, 38, 76, 74, 72, 70, 33, 33, 66, 64, 62, 60, 28, 28, 56, 54, 52, 50, 23, 23, 46, 44, 42, 40, 18,
		18, 36, 34, 32, 30, 13, 13, 26, 24, 22, 20, 8, 8, 16, 14, 12, 10, 3, 3, 6, 4, 2, 0,
	}
	require.Equal(t, exp, results)
}

func testManyParamsResultsMainListener(t *testing.T, r wazero.Runtime) {
	var buf bytes.Buffer
	ctx := context.WithValue(context.Background(), experimental.FunctionListenerFactoryKey{}, logging.NewLoggingListenerFactory(&buf))

	bin, params := manyParamsResultsMod()
	mod, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	main := mod.ExportedFunction("main")
	require.NotNil(t, main)

	results, err := main.Call(ctx, params...)
	require.NoError(t, err)

	fmt.Println(buf.String())
	require.Equal(t, `
--> .main([0:i32]=0,[1:i64]=1,[2:f32]=3e-45,[3:f64]=1.5e-323,[4:v128]=00000000000000030000000000000003,[5:i32]=3,[6:i64]=5,[7:f32]=8e-45,[8:f64]=3.5e-323,[9:v128]=00000000000000080000000000000008,[10:i32]=8,[11:i64]=8,[12:f32]=1.4e-44,[13:f64]=5.4e-323,[14:v128]=000000000000000c000000000000000d,[15:i32]=13,[16:i64]=13,[17:f32]=1.8e-44,[18:f64]=7.4e-323,[19:v128]=00000000000000100000000000000011,[20:i32]=17,[21:i64]=18,[22:f32]=2.5e-44,[23:f64]=9e-323,[24:v128]=00000000000000140000000000000015,[25:i32]=21,[26:i64]=22,[27:f32]=3.2e-44,[28:f64]=1.14e-322,[29:v128]=00000000000000170000000000000019,[30:i32]=25,[31:i64]=26,[32:f32]=3.8e-44,[33:f64]=1.4e-322,[34:v128]=000000000000001c000000000000001c,[35:i32]=28,[36:i64]=30,[37:f32]=4.3e-44,[38:f64]=1.6e-322,[39:v128]=00000000000000210000000000000021,[40:i32]=33,[41:i64]=33,[42:f32]=4.9e-44,[43:f64]=1.8e-322,[44:v128]=00000000000000250000000000000026,[45:i32]=38,[46:i64]=38,[47:f32]=5.3e-44,[48:f64]=2e-322,[49:v128]=0000000000000029000000000000002a,[50:i32]=42,[51:i64]=43,[52:f32]=6e-44,[53:f64]=2.1e-322,[54:v128]=000000000000002d000000000000002e,[55:i32]=46,[56:i64]=47,[57:f32]=6.7e-44,[58:f64]=2.37e-322,[59:v128]=00000000000000300000000000000032,[60:i32]=50,[61:i64]=51,[62:f32]=7.3e-44,[63:f64]=2.6e-322,[64:v128]=00000000000000350000000000000035,[65:i32]=53,[66:i64]=55,[67:f32]=7.8e-44,[68:f64]=2.8e-322,[69:v128]=000000000000003a000000000000003a,[70:i32]=58,[71:i64]=58,[72:f32]=8.4e-44,[73:f64]=3e-322,[74:v128]=000000000000003e000000000000003f,[75:i32]=63,[76:i64]=63,[77:f32]=8.8e-44,[78:f64]=3.2e-322,[79:v128]=00000000000000420000000000000043,[80:i32]=67,[81:i64]=68,[82:f32]=9.5e-44,[83:f64]=3.36e-322,[84:v128]=00000000000000460000000000000047,[85:i32]=71,[86:i64]=72,[87:f32]=1.02e-43,[88:f64]=3.6e-322,[89:v128]=0000000000000049000000000000004b,[90:i32]=75,[91:i64]=76,[92:f32]=1.08e-43,[93:f64]=3.85e-322,[94:v128]=000000000000004e000000000000004e,[95:i32]=78,[96:i64]=80,[97:f32]=1.14e-43,[98:f64]=4.05e-322,[99:v128]=00000000000000530000000000000053)
	--> .swapper([0:i32]=0,[1:i64]=1,[2:f32]=3e-45,[3:f64]=1.5e-323,[4:v128]=00000000000000030000000000000003,[5:i32]=3,[6:i64]=5,[7:f32]=8e-45,[8:f64]=3.5e-323,[9:v128]=00000000000000080000000000000008,[10:i32]=8,[11:i64]=8,[12:f32]=1.4e-44,[13:f64]=5.4e-323,[14:v128]=000000000000000c000000000000000d,[15:i32]=13,[16:i64]=13,[17:f32]=1.8e-44,[18:f64]=7.4e-323,[19:v128]=00000000000000100000000000000011,[20:i32]=17,[21:i64]=18,[22:f32]=2.5e-44,[23:f64]=9e-323,[24:v128]=00000000000000140000000000000015,[25:i32]=21,[26:i64]=22,[27:f32]=3.2e-44,[28:f64]=1.14e-322,[29:v128]=00000000000000170000000000000019,[30:i32]=25,[31:i64]=26,[32:f32]=3.8e-44,[33:f64]=1.4e-322,[34:v128]=000000000000001c000000000000001c,[35:i32]=28,[36:i64]=30,[37:f32]=4.3e-44,[38:f64]=1.6e-322,[39:v128]=00000000000000210000000000000021,[40:i32]=33,[41:i64]=33,[42:f32]=4.9e-44,[43:f64]=1.8e-322,[44:v128]=00000000000000250000000000000026,[45:i32]=38,[46:i64]=38,[47:f32]=5.3e-44,[48:f64]=2e-322,[49:v128]=0000000000000029000000000000002a,[50:i32]=42,[51:i64]=43,[52:f32]=6e-44,[53:f64]=2.1e-322,[54:v128]=000000000000002d000000000000002e,[55:i32]=46,[56:i64]=47,[57:f32]=6.7e-44,[58:f64]=2.37e-322,[59:v128]=00000000000000300000000000000032,[60:i32]=50,[61:i64]=51,[62:f32]=7.3e-44,[63:f64]=2.6e-322,[64:v128]=00000000000000350000000000000035,[65:i32]=53,[66:i64]=55,[67:f32]=7.8e-44,[68:f64]=2.8e-322,[69:v128]=000000000000003a000000000000003a,[70:i32]=58,[71:i64]=58,[72:f32]=8.4e-44,[73:f64]=3e-322,[74:v128]=000000000000003e000000000000003f,[75:i32]=63,[76:i64]=63,[77:f32]=8.8e-44,[78:f64]=3.2e-322,[79:v128]=00000000000000420000000000000043,[80:i32]=67,[81:i64]=68,[82:f32]=9.5e-44,[83:f64]=3.36e-322,[84:v128]=00000000000000460000000000000047,[85:i32]=71,[86:i64]=72,[87:f32]=1.02e-43,[88:f64]=3.6e-322,[89:v128]=0000000000000049000000000000004b,[90:i32]=75,[91:i64]=76,[92:f32]=1.08e-43,[93:f64]=3.85e-322,[94:v128]=000000000000004e000000000000004e,[95:i32]=78,[96:i64]=80,[97:f32]=1.14e-43,[98:f64]=4.05e-322,[99:v128]=00000000000000530000000000000053)
	<-- (00000000000000620000000000000062,4.84e-322,1.37e-43,97,96,000000000000005f000000000000005d,4.6e-322,1.3e-43,93,92,000000000000005b000000000000005a,4.45e-322,1.23e-43,88,88,00000000000000570000000000000056,4.25e-322,1.19e-43,83,83,00000000000000530000000000000052,4.05e-322,1.14e-43,80,78,000000000000004e000000000000004e,3.85e-322,1.08e-43,76,75,00000000000000490000000000000049,3.6e-322,1.02e-43,72,71,00000000000000460000000000000044,3.36e-322,9.5e-44,68,67,00000000000000420000000000000041,3.2e-322,8.8e-44,63,63,000000000000003e000000000000003d,3e-322,8.4e-44,58,58,000000000000003a0000000000000039,2.8e-322,7.8e-44,55,53,00000000000000350000000000000035,2.6e-322,7.3e-44,51,50,00000000000000300000000000000030,2.37e-322,6.7e-44,47,46,000000000000002d000000000000002b,2.1e-322,6e-44,43,42,00000000000000290000000000000028,2e-322,5.3e-44,38,38,00000000000000250000000000000024,1.8e-322,4.9e-44,33,33,00000000000000210000000000000020,1.6e-322,4.3e-44,30,28,000000000000001c000000000000001c,1.4e-322,3.8e-44,26,25,00000000000000170000000000000017,1.14e-322,3.2e-44,22,21,00000000000000140000000000000012,9e-323,2.5e-44,18,17)
	--> .doubler([0:v128]=00000000000000620000000000000062,[1:f64]=4.84e-322,[2:f32]=1.37e-43,[3:i64]=97,[4:i32]=96,[5:v128]=000000000000005f000000000000005d,[6:f64]=4.6e-322,[7:f32]=1.3e-43,[8:i64]=93,[9:i32]=92,[10:v128]=000000000000005b000000000000005a,[11:f64]=4.45e-322,[12:f32]=1.23e-43,[13:i64]=88,[14:i32]=88,[15:v128]=00000000000000570000000000000056,[16:f64]=4.25e-322,[17:f32]=1.19e-43,[18:i64]=83,[19:i32]=83,[20:v128]=00000000000000530000000000000052,[21:f64]=4.05e-322,[22:f32]=1.14e-43,[23:i64]=80,[24:i32]=78,[25:v128]=000000000000004e000000000000004e,[26:f64]=3.85e-322,[27:f32]=1.08e-43,[28:i64]=76,[29:i32]=75,[30:v128]=00000000000000490000000000000049,[31:f64]=3.6e-322,[32:f32]=1.02e-43,[33:i64]=72,[34:i32]=71,[35:v128]=00000000000000460000000000000044,[36:f64]=3.36e-322,[37:f32]=9.5e-44,[38:i64]=68,[39:i32]=67,[40:v128]=00000000000000420000000000000041,[41:f64]=3.2e-322,[42:f32]=8.8e-44,[43:i64]=63,[44:i32]=63,[45:v128]=000000000000003e000000000000003d,[46:f64]=3e-322,[47:f32]=8.4e-44,[48:i64]=58,[49:i32]=58,[50:v128]=000000000000003a0000000000000039,[51:f64]=2.8e-322,[52:f32]=7.8e-44,[53:i64]=55,[54:i32]=53,[55:v128]=00000000000000350000000000000035,[56:f64]=2.6e-322,[57:f32]=7.3e-44,[58:i64]=51,[59:i32]=50,[60:v128]=00000000000000300000000000000030,[61:f64]=2.37e-322,[62:f32]=6.7e-44,[63:i64]=47,[64:i32]=46,[65:v128]=000000000000002d000000000000002b,[66:f64]=2.1e-322,[67:f32]=6e-44,[68:i64]=43,[69:i32]=42,[70:v128]=00000000000000290000000000000028,[71:f64]=2e-322,[72:f32]=5.3e-44,[73:i64]=38,[74:i32]=38,[75:v128]=00000000000000250000000000000024,[76:f64]=1.8e-322,[77:f32]=4.9e-44,[78:i64]=33,[79:i32]=33,[80:v128]=00000000000000210000000000000020,[81:f64]=1.6e-322,[82:f32]=4.3e-44,[83:i64]=30,[84:i32]=28,[85:v128]=000000000000001c000000000000001c,[86:f64]=1.4e-322,[87:f32]=3.8e-44,[88:i64]=26,[89:i32]=25,[90:v128]=00000000000000170000000000000017,[91:f64]=1.14e-322,[92:f32]=3.2e-44,[93:i64]=22,[94:i32]=21,[95:v128]=00000000000000140000000000000012,[96:f64]=9e-323,[97:f32]=2.5e-44,[98:i64]=18,[99:i32]=17)
	<-- (00000000000000620000000000000062,4.84e-322,2.75e-43,194,192,00000000000000be000000000000005d,4.6e-322,1.3e-43,186,184,00000000000000b600000000000000b4,8.9e-322,1.23e-43,88,176,00000000000000ae00000000000000ac,8.5e-322,2.38e-43,83,83,00000000000000a600000000000000a4,8.1e-322,2.27e-43,160,78,000000000000004e000000000000009c,7.7e-322,2.16e-43,152,150,00000000000000490000000000000049,3.6e-322,2.05e-43,144,142,000000000000008c0000000000000044,3.36e-322,9.5e-44,136,134,00000000000000840000000000000082,6.4e-322,8.8e-44,63,126,000000000000007c000000000000007a,6.03e-322,1.68e-43,58,58,00000000000000740000000000000072,5.63e-322,1.57e-43,110,53,0000000000000035000000000000006a,5.24e-322,1.46e-43,102,100,00000000000000300000000000000030,2.37e-322,1.35e-43,94,92,000000000000005a000000000000002b,2.1e-322,6e-44,86,84,00000000000000520000000000000050,3.95e-322,5.3e-44,38,76,000000000000004a0000000000000048,3.56e-322,9.8e-44,33,33,00000000000000420000000000000040,3.16e-322,8.7e-44,60,28,000000000000001c0000000000000038,2.77e-322,7.6e-44,52,50,00000000000000170000000000000017,1.14e-322,6.4e-44,44,42,00000000000000280000000000000012,9e-323,2.5e-44,36,34)
<-- (00000000000000620000000000000062,4.84e-322,2.75e-43,194,192,00000000000000be000000000000005d,4.6e-322,1.3e-43,186,184,00000000000000b600000000000000b4,8.9e-322,1.23e-43,88,176,00000000000000ae00000000000000ac,8.5e-322,2.38e-43,83,83,00000000000000a600000000000000a4,8.1e-322,2.27e-43,160,78,000000000000004e000000000000009c,7.7e-322,2.16e-43,152,150,00000000000000490000000000000049,3.6e-322,2.05e-43,144,142,000000000000008c0000000000000044,3.36e-322,9.5e-44,136,134,00000000000000840000000000000082,6.4e-322,8.8e-44,63,126,000000000000007c000000000000007a,6.03e-322,1.68e-43,58,58,00000000000000740000000000000072,5.63e-322,1.57e-43,110,53,0000000000000035000000000000006a,5.24e-322,1.46e-43,102,100,00000000000000300000000000000030,2.37e-322,1.35e-43,94,92,000000000000005a000000000000002b,2.1e-322,6e-44,86,84,00000000000000520000000000000050,3.95e-322,5.3e-44,38,76,000000000000004a0000000000000048,3.56e-322,9.8e-44,33,33,00000000000000420000000000000040,3.16e-322,8.7e-44,60,28,000000000000001c0000000000000038,2.77e-322,7.6e-44,52,50,00000000000000170000000000000017,1.14e-322,6.4e-44,44,42,00000000000000280000000000000012,9e-323,2.5e-44,36,34)
`, "\n"+buf.String())

	exp := []uint64{
		98, 98, 196, 194, 192, 190, 93, 93, 186, 184, 182, 180, 88, 88, 176, 174, 172, 170, 83, 83, 166, 164, 162,
		160, 78, 78, 156, 154, 152, 150, 73, 73, 146, 144, 142, 140, 68, 68, 136, 134, 132, 130, 63, 63, 126, 124,
		122, 120, 58, 58, 116, 114, 112, 110, 53, 53, 106, 104, 102, 100, 48, 48, 96, 94, 92, 90, 43, 43, 86, 84,
		82, 80, 38, 38, 76, 74, 72, 70, 33, 33, 66, 64, 62, 60, 28, 28, 56, 54, 52, 50, 23, 23, 46, 44, 42, 40, 18,
		18, 36, 34, 32, 30, 13, 13, 26, 24, 22, 20, 8, 8, 16, 14, 12, 10, 3, 3, 6, 4, 2, 0,
	}
	require.Equal(t, exp, results)
}

func testManyParamsResultsCallManyConstsAndPickLastVector(t *testing.T, r wazero.Runtime) {
	ctx := context.Background()

	bin, _ := manyParamsResultsMod()
	mod, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	main := mod.ExportedFunction("call_many_consts_and_pick_last_vector")
	require.NotNil(t, main)

	results, err := main.Call(ctx)
	require.NoError(t, err)
	exp := []uint64{0x5f5f5f5f5f5f5f5f, 0x5f5f5f5f5f5f5f5f}
	require.Equal(t, exp, results)
}

func testManyParamsResultsCallManyConstsAndPickLastVectorListener(t *testing.T, r wazero.Runtime) {
	var buf bytes.Buffer
	ctx := context.WithValue(context.Background(), experimental.FunctionListenerFactoryKey{}, logging.NewLoggingListenerFactory(&buf))

	bin, _ := manyParamsResultsMod()
	mod, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	main := mod.ExportedFunction("call_many_consts_and_pick_last_vector")
	require.NotNil(t, main)

	results, err := main.Call(ctx)
	require.NoError(t, err)
	exp := []uint64{0x5f5f5f5f5f5f5f5f, 0x5f5f5f5f5f5f5f5f}
	require.Equal(t, exp, results)

	fmt.Println(buf.String())
	require.Equal(t, `
--> .call_many_consts_and_pick_last_vector()
	--> .many_consts()
	<-- (0,0,0,0,00000000000000000000000000000000,0,5,7e-45,4.16077606e-316,05050505050505050505050505050505,84215045,361700864190383365,1.4e-44,5e-323,000000000a0a0a0a0a0a0a0a0a0a0a0a,168430090,723401728380766730,6.6463464e-33,7.4e-323,000000000000000f000000000f0f0f0f,252645135,1085102592571150095,7.0533445e-30,3.815736827118017e-236,00000000000000140000000000000014,20,336860180,7.47605e-27,5.964208835435795e-212,14141414141414140000000000000019,25,25,7.914983e-24,9.01285756841504e-188,19191919191919191919191919191919,421075225,30,4.2e-44,2.496465636e-315,1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e,505290270,2170205185142300190,4.9e-44,1.73e-322,00000000232323232323232323232323,589505315,2531906049332683555,8.843688e-18,2e-322,00000000000000280000000028282828,673720360,2893606913523066920,9.334581e-15,3.0654356309538037e-115,000000000000002d000000000000002d,45,757935405,9.8439425e-12,4.4759381595361623e-91,2d2d2d2d2d2d2d2d0000000000000032,50,50,1.0372377e-08,6.749300603603778e-67,32323232323232323232323232323232,842150450,55,7.7e-44,4.576853666e-315,37373737373737373737373737373737,926365495,3978709506094217015,8.4e-44,2.96e-322,000000003c3c3c3c3c3c3c3c3c3c3c3c,1010580540,4340410370284600380,0.01148897,3.2e-322,00000000000000410000000041414141,1094795585,4702111234474983745,12.078431,2.2616345098039214e+06,00000000000000460000000000000046,70,1179010630,12689.568,3.5295369653413445e+30,4646464646464646000000000000004b,75,75,1.3323083e+07,5.2285141982483265e+54,4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b,1263225675,80,1.12e-43,6.657241696e-315,50505050505050505050505050505050)
	--> .pick_last_vector([0:i32]=0,[1:i64]=0,[2:f32]=0,[3:f64]=0,[4:v128]=00000000000000000000000000000000,[5:i32]=0,[6:i64]=5,[7:f32]=7e-45,[8:f64]=4.16077606e-316,[9:v128]=05050505050505050505050505050505,[10:i32]=84215045,[11:i64]=361700864190383365,[12:f32]=1.4e-44,[13:f64]=5e-323,[14:v128]=000000000a0a0a0a0a0a0a0a0a0a0a0a,[15:i32]=168430090,[16:i64]=723401728380766730,[17:f32]=6.6463464e-33,[18:f64]=7.4e-323,[19:v128]=000000000000000f000000000f0f0f0f,[20:i32]=252645135,[21:i64]=1085102592571150095,[22:f32]=7.0533445e-30,[23:f64]=3.815736827118017e-236,[24:v128]=00000000000000140000000000000014,[25:i32]=20,[26:i64]=336860180,[27:f32]=7.47605e-27,[28:f64]=5.964208835435795e-212,[29:v128]=14141414141414140000000000000019,[30:i32]=25,[31:i64]=25,[32:f32]=7.914983e-24,[33:f64]=9.01285756841504e-188,[34:v128]=19191919191919191919191919191919,[35:i32]=421075225,[36:i64]=30,[37:f32]=4.2e-44,[38:f64]=2.496465636e-315,[39:v128]=1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e,[40:i32]=505290270,[41:i64]=2170205185142300190,[42:f32]=4.9e-44,[43:f64]=1.73e-322,[44:v128]=00000000232323232323232323232323,[45:i32]=589505315,[46:i64]=2531906049332683555,[47:f32]=8.843688e-18,[48:f64]=2e-322,[49:v128]=00000000000000280000000028282828,[50:i32]=673720360,[51:i64]=2893606913523066920,[52:f32]=9.334581e-15,[53:f64]=3.0654356309538037e-115,[54:v128]=000000000000002d000000000000002d,[55:i32]=45,[56:i64]=757935405,[57:f32]=9.8439425e-12,[58:f64]=4.4759381595361623e-91,[59:v128]=2d2d2d2d2d2d2d2d0000000000000032,[60:i32]=50,[61:i64]=50,[62:f32]=1.0372377e-08,[63:f64]=6.749300603603778e-67,[64:v128]=32323232323232323232323232323232,[65:i32]=842150450,[66:i64]=55,[67:f32]=7.7e-44,[68:f64]=4.576853666e-315,[69:v128]=37373737373737373737373737373737,[70:i32]=926365495,[71:i64]=3978709506094217015,[72:f32]=8.4e-44,[73:f64]=2.96e-322,[74:v128]=000000003c3c3c3c3c3c3c3c3c3c3c3c,[75:i32]=1010580540,[76:i64]=4340410370284600380,[77:f32]=0.01148897,[78:f64]=3.2e-322,[79:v128]=00000000000000410000000041414141,[80:i32]=1094795585,[81:i64]=4702111234474983745,[82:f32]=12.078431,[83:f64]=2.2616345098039214e+06,[84:v128]=00000000000000460000000000000046,[85:i32]=70,[86:i64]=1179010630,[87:f32]=12689.568,[88:f64]=3.5295369653413445e+30,[89:v128]=4646464646464646000000000000004b,[90:i32]=75,[91:i64]=75,[92:f32]=1.3323083e+07,[93:f64]=5.2285141982483265e+54,[94:v128]=4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b,[95:i32]=1263225675,[96:i64]=80,[97:f32]=1.12e-43,[98:f64]=6.657241696e-315,[99:v128]=50505050505050505050505050505050)
	<-- 5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f
<-- 5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f
`, "\n"+buf.String())
}
