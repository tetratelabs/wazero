package interpreter

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/buildoptions"
	"github.com/tetratelabs/wazero/internal/wazeroir"
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

func TestCallEngine_PushFrame(t *testing.T) {
	f1 := &callFrame{}
	f2 := &callFrame{}

	vm := callEngine{}
	require.Empty(t, vm.frames)

	vm.pushFrame(f1)
	require.Equal(t, []*callFrame{f1}, vm.frames)

	vm.pushFrame(f2)
	require.Equal(t, []*callFrame{f1, f2}, vm.frames)
}

func TestCallEngine_PushFrame_StackOverflow(t *testing.T) {
	defer func() { callStackCeiling = buildoptions.CallStackCeiling }()

	callStackCeiling = 3

	f1 := &callFrame{}
	f2 := &callFrame{}
	f3 := &callFrame{}
	f4 := &callFrame{}

	vm := callEngine{}
	vm.pushFrame(f1)
	vm.pushFrame(f2)
	vm.pushFrame(f3)
	require.Panics(t, func() { vm.pushFrame(f4) })
}

func TestEngine_Call(t *testing.T) {
	i64 := wasm.ValueTypeI64
	m := &wasm.Module{
		TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i64}}},
		FunctionSection: []wasm.Index{wasm.Index(0)},
		CodeSection:     []*wasm.Code{{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}}},
		ExportSection:   map[string]*wasm.Export{"fn": {Type: wasm.ExternTypeFunc, Index: 0, Name: "fn"}},
	}

	// Use exported functions to simplify instantiation of a Wasm function
	e := NewEngine()
	store := wasm.NewStore(context.Background(), e, wasm.Features20191205)
	mod, err := store.Instantiate(m, "")
	require.NoError(t, err)

	fn := mod.ExportedFunction("fn")
	require.NotNil(t, fn)

	// ensure base case doesn't fail
	results, err := fn.Call(nil, 3)
	require.NoError(t, err)
	require.Equal(t, uint64(3), results[0])

	t.Run("errs when not enough parameters", func(t *testing.T) {
		_, err := fn.Call(nil)
		require.EqualError(t, err, "expected 1 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		_, err := fn.Call(nil, 1, 2)
		require.EqualError(t, err, "expected 1 params, but passed 2")
	})
}

func TestEngine_Call_HostFn(t *testing.T) {
	memory := &wasm.MemoryInstance{}
	var ctxMemory publicwasm.Memory
	hostFn := reflect.ValueOf(func(ctx publicwasm.Module, v uint64) uint64 {
		ctxMemory = ctx.Memory()
		return v
	})

	e := NewEngine()
	module := &wasm.ModuleInstance{Memory: memory}
	modCtx := wasm.NewModuleContext(context.Background(), e, module)
	f := &wasm.FunctionInstance{
		GoFunc: &hostFn,
		Kind:   wasm.FunctionKindGoModule,
		Type: &wasm.FunctionType{
			Params:  []wasm.ValueType{wasm.ValueTypeI64},
			Results: []wasm.ValueType{wasm.ValueTypeI64},
		},
		Module: module,
	}

	me, err := e.NewModuleEngine(nil, []*wasm.FunctionInstance{f})
	require.NoError(t, err)

	t.Run("defaults to module memory when call stack empty", func(t *testing.T) {
		// When calling a host func directly, there may be no stack. This ensures the module's memory is used.
		results, err := me.Call(modCtx, f, 3)
		require.NoError(t, err)
		require.Equal(t, uint64(3), results[0])
		require.Same(t, memory, ctxMemory)
	})

	t.Run("errs when not enough parameters", func(t *testing.T) {
		_, err := me.Call(modCtx, f)
		require.EqualError(t, err, "expected 1 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		_, err := me.Call(modCtx, f, 1, 2)
		require.EqualError(t, err, "expected 1 params, but passed 2")
	})
}

func TestCallEngine_callNativeFunc_signExtend(t *testing.T) {
	translateToIROperationKind := func(op wasm.Opcode) (kind wazeroir.OperationKind) {
		switch op {
		case wasm.OpcodeI32Extend8S:
			kind = wazeroir.OperationKindSignExtend32From8
		case wasm.OpcodeI32Extend16S:
			kind = wazeroir.OperationKindSignExtend32From16
		case wasm.OpcodeI64Extend8S:
			kind = wazeroir.OperationKindSignExtend64From8
		case wasm.OpcodeI64Extend16S:
			kind = wazeroir.OperationKindSignExtend64From16
		case wasm.OpcodeI64Extend32S:
			kind = wazeroir.OperationKindSignExtend64From32
		}
		return
	}
	t.Run("32bit", func(t *testing.T) {
		for _, tc := range []struct {
			in       int32
			expected int32
			opcode   wasm.Opcode
		}{
			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i32.wast#L270-L276
			{in: 0, expected: 0, opcode: wasm.OpcodeI32Extend8S},
			{in: 0x7f, expected: 127, opcode: wasm.OpcodeI32Extend8S},
			{in: 0x80, expected: -128, opcode: wasm.OpcodeI32Extend8S},
			{in: 0xff, expected: -1, opcode: wasm.OpcodeI32Extend8S},
			{in: 0x012345_00, expected: 0, opcode: wasm.OpcodeI32Extend8S},
			{in: -19088768 /* = 0xfedcba_80 bit pattern */, expected: -0x80, opcode: wasm.OpcodeI32Extend8S},
			{in: -1, expected: -1, opcode: wasm.OpcodeI32Extend8S},

			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i32.wast#L278-L284
			{in: 0, expected: 0, opcode: wasm.OpcodeI32Extend16S},
			{in: 0x7fff, expected: 32767, opcode: wasm.OpcodeI32Extend16S},
			{in: 0x8000, expected: -32768, opcode: wasm.OpcodeI32Extend16S},
			{in: 0xffff, expected: -1, opcode: wasm.OpcodeI32Extend16S},
			{in: 0x0123_0000, expected: 0, opcode: wasm.OpcodeI32Extend16S},
			{in: -19103744 /* = 0xfedc_8000 bit pattern */, expected: -0x8000, opcode: wasm.OpcodeI32Extend16S},
			{in: -1, expected: -1, opcode: wasm.OpcodeI32Extend16S},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%s(i32.const(0x%x))", wasm.InstructionName(tc.opcode), tc.in), func(t *testing.T) {
				ce := &callEngine{}
				f := &compiledFunction{
					moduleEngine: &moduleEngine{},
					funcInstance: &wasm.FunctionInstance{Module: &wasm.ModuleInstance{}},
					body: []*interpreterOp{
						{kind: wazeroir.OperationKindConstI32, us: []uint64{uint64(uint32(tc.in))}},
						{kind: translateToIROperationKind(tc.opcode)},
						{kind: wazeroir.OperationKindBr, us: []uint64{math.MaxUint64}},
					},
				}
				ce.callNativeFunc(&wasm.ModuleContext{}, f)
				require.Equal(t, tc.expected, int32(uint32(ce.pop())))
			})
		}
	})
	t.Run("64bit", func(t *testing.T) {
		for _, tc := range []struct {
			in       int64
			expected int64
			opcode   wasm.Opcode
		}{
			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L271-L277
			{in: 0, expected: 0, opcode: wasm.OpcodeI64Extend8S},
			{in: 0x7f, expected: 127, opcode: wasm.OpcodeI64Extend8S},
			{in: 0x80, expected: -128, opcode: wasm.OpcodeI64Extend8S},
			{in: 0xff, expected: -1, opcode: wasm.OpcodeI64Extend8S},
			{in: 0x01234567_89abcd_00, expected: 0, opcode: wasm.OpcodeI64Extend8S},
			{in: 81985529216486784 /* = 0xfedcba98_765432_80 bit pattern */, expected: -0x80, opcode: wasm.OpcodeI64Extend8S},
			{in: -1, expected: -1, opcode: wasm.OpcodeI64Extend8S},

			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L279-L285
			{in: 0, expected: 0, opcode: wasm.OpcodeI64Extend16S},
			{in: 0x7fff, expected: 32767, opcode: wasm.OpcodeI64Extend16S},
			{in: 0x8000, expected: -32768, opcode: wasm.OpcodeI64Extend16S},
			{in: 0xffff, expected: -1, opcode: wasm.OpcodeI64Extend16S},
			{in: 0x12345678_9abc_0000, expected: 0, opcode: wasm.OpcodeI64Extend16S},
			{in: 81985529216466944 /* = 0xfedcba98_7654_8000 bit pattern */, expected: -0x8000, opcode: wasm.OpcodeI64Extend16S},
			{in: -1, expected: -1, opcode: wasm.OpcodeI64Extend16S},

			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L287-L296
			{in: 0, expected: 0, opcode: wasm.OpcodeI64Extend32S},
			{in: 0x7fff, expected: 32767, opcode: wasm.OpcodeI64Extend32S},
			{in: 0x8000, expected: 32768, opcode: wasm.OpcodeI64Extend32S},
			{in: 0xffff, expected: 65535, opcode: wasm.OpcodeI64Extend32S},
			{in: 0x7fffffff, expected: 0x7fffffff, opcode: wasm.OpcodeI64Extend32S},
			{in: 0x80000000, expected: -0x80000000, opcode: wasm.OpcodeI64Extend32S},
			{in: 0xffffffff, expected: -1, opcode: wasm.OpcodeI64Extend32S},
			{in: 0x01234567_00000000, expected: 0, opcode: wasm.OpcodeI64Extend32S},
			{in: -81985529054232576 /* = 0xfedcba98_80000000 bit pattern */, expected: -0x80000000, opcode: wasm.OpcodeI64Extend32S},
			{in: -1, expected: -1, opcode: wasm.OpcodeI64Extend32S},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%s(i64.const(0x%x))", wasm.InstructionName(tc.opcode), tc.in), func(t *testing.T) {
				ce := &callEngine{}
				f := &compiledFunction{
					moduleEngine: &moduleEngine{},
					funcInstance: &wasm.FunctionInstance{Module: &wasm.ModuleInstance{}},
					body: []*interpreterOp{
						{kind: wazeroir.OperationKindConstI64, us: []uint64{uint64(tc.in)}},
						{kind: translateToIROperationKind(tc.opcode)},
						{kind: wazeroir.OperationKindBr, us: []uint64{math.MaxUint64}},
					},
				}
				ce.callNativeFunc(&wasm.ModuleContext{}, f)
				require.Equal(t, tc.expected, int64(ce.pop()))
			})
		}
	})
}

func TestEngineCompile_Errors(t *testing.T) {
	t.Run("invalid import", func(t *testing.T) {
		e := NewEngine().(*engine)
		_, err := e.NewModuleEngine(
			[]*wasm.FunctionInstance{{Module: &wasm.ModuleInstance{Name: "uncompiled"}, Name: "fn"}},
			nil, // moduleFunctions
		)
		require.EqualError(t, err, "import[0] func[uncompiled.fn]: uncompiled")
	})

	t.Run("release on compilation error", func(t *testing.T) {
		e := NewEngine().(*engine)

		importedFunctions := []*wasm.FunctionInstance{
			{Name: "1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "3", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "4", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
		}

		// initialize the module-engine containing imported functions
		_, err := e.NewModuleEngine(nil, importedFunctions)
		require.NoError(t, err)

		require.Len(t, e.compiledFunctions, len(importedFunctions))

		moduleFunctions := []*wasm.FunctionInstance{
			{Name: "ok1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "ok2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "invalid code", Type: &wasm.FunctionType{}, Body: []byte{
				wasm.OpcodeCall, // Call instruction without immediate for call target index is invalid and should fail to compile.
			}, Module: &wasm.ModuleInstance{}},
		}

		_, err = e.NewModuleEngine(importedFunctions, moduleFunctions)
		require.EqualError(t, err, "function[2/2] failed to lower to wazeroir: handling instruction: apply stack failed for call: reading immediates: EOF")

		// On the compilation failure, all the compiled functions including succeeded ones must be released.
		require.Len(t, e.compiledFunctions, len(importedFunctions))
		for _, f := range moduleFunctions {
			require.NotContains(t, e.compiledFunctions, f)
		}
	})
}

func TestRelease(t *testing.T) {
	newFunctionInstance := func(id int) *wasm.FunctionInstance {
		return &wasm.FunctionInstance{
			Name: strconv.Itoa(id), Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}}
	}

	for _, tc := range []struct {
		name                               string
		importedFunctions, moduleFunctions []*wasm.FunctionInstance
	}{
		{
			name:            "only module-defined",
			moduleFunctions: []*wasm.FunctionInstance{newFunctionInstance(0), newFunctionInstance(1)},
		},
		{
			name:              "only imports",
			importedFunctions: []*wasm.FunctionInstance{newFunctionInstance(0), newFunctionInstance(1)},
		},
		{
			name:              "imports and module-defined",
			importedFunctions: []*wasm.FunctionInstance{newFunctionInstance(0), newFunctionInstance(1)},
			moduleFunctions:   []*wasm.FunctionInstance{newFunctionInstance(100), newFunctionInstance(200), newFunctionInstance(300)},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			e := NewEngine().(*engine)
			if len(tc.importedFunctions) > 0 {
				// initialize the module-engine containing imported functions
				me, err := e.NewModuleEngine(nil, tc.importedFunctions)
				require.NoError(t, err)
				require.Len(t, me.(*moduleEngine).compiledFunctions, len(tc.importedFunctions))
			}

			me, err := e.NewModuleEngine(tc.importedFunctions, tc.moduleFunctions)
			require.NoError(t, err)
			require.Len(t, me.(*moduleEngine).compiledFunctions, len(tc.importedFunctions)+len(tc.moduleFunctions))

			require.Len(t, e.compiledFunctions, len(tc.importedFunctions)+len(tc.moduleFunctions))
			for _, f := range tc.importedFunctions {
				require.Contains(t, e.compiledFunctions, f)
			}
			for _, f := range tc.moduleFunctions {
				require.Contains(t, e.compiledFunctions, f)
			}

			err = me.Release()
			require.NoError(t, err)

			require.Len(t, e.compiledFunctions, len(tc.importedFunctions))
			for _, f := range tc.importedFunctions {
				require.Contains(t, e.compiledFunctions, f)
			}
			for i, f := range tc.moduleFunctions {
				require.NotContains(t, e.compiledFunctions, f, i)
			}
		})
	}
}
