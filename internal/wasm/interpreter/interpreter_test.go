package interpreter

import (
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/buildoptions"
	"github.com/tetratelabs/wazero/internal/testing/enginetest"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestInterpreter_CallEngine_PushFrame(t *testing.T) {
	f1 := &callFrame{}
	f2 := &callFrame{}

	vm := callEngine{}
	require.Empty(t, vm.frames)

	vm.pushFrame(f1)
	require.Equal(t, []*callFrame{f1}, vm.frames)

	vm.pushFrame(f2)
	require.Equal(t, []*callFrame{f1, f2}, vm.frames)
}

func TestInterpreter_CallEngine_PushFrame_StackOverflow(t *testing.T) {
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

// et is used for tests defined in the enginetest package.
var et = &engineTester{}

type engineTester struct {
}

func (e engineTester) NewEngine() wasm.Engine {
	return NewEngine()
}

func (e engineTester) InitTable(me wasm.ModuleEngine, initTableLen uint32, initTableIdxToFnIdx map[wasm.Index]wasm.Index) []interface{} {
	table := make([]interface{}, initTableLen)
	internal := me.(*moduleEngine)
	for idx, fnidx := range initTableIdxToFnIdx {
		table[idx] = internal.compiledFunctions[fnidx]
	}
	return table
}

func TestInterpreter_Engine_NewModuleEngine(t *testing.T) {
	enginetest.RunTestEngine_NewModuleEngine(t, et)
}

func TestInterpreter_Engine_NewModuleEngine_InitTable(t *testing.T) {
	enginetest.RunTestEngine_NewModuleEngine_InitTable(t, et)
}

func TestInterpreter_ModuleEngine_Call(t *testing.T) {
	enginetest.RunTestModuleEngine_Call(t, et)
}

func TestInterpreter_ModuleEngine_Call_HostFn(t *testing.T) {
	enginetest.RunTestModuleEngine_Call_HostFn(t, et)
}

func TestInterpreter_ModuleEngine_Call_Errors(t *testing.T) {
	enginetest.RunTestModuleEngine_Call_Errors(t, et)
}

func TestInterpreter_CallEngine_callNativeFunc_signExtend(t *testing.T) {
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
					source:       &wasm.FunctionInstance{Module: &wasm.ModuleInstance{}},
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
					source:       &wasm.FunctionInstance{Module: &wasm.ModuleInstance{}},
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

func TestInterpreter_EngineCompile_Errors(t *testing.T) {
	t.Run("invalid import", func(t *testing.T) {
		e := et.NewEngine().(*engine)
		_, err := e.NewModuleEngine(t.Name(),
			[]*wasm.FunctionInstance{{Module: &wasm.ModuleInstance{Name: "uncompiled"}, DebugName: "uncompiled.fn"}},
			nil, // moduleFunctions
			nil, // table
			nil, // tableInit
		)
		require.EqualError(t, err, "import[0] func[uncompiled.fn]: uncompiled")
	})

	t.Run("release on compilation error", func(t *testing.T) {
		e := et.NewEngine().(*engine)

		importedFunctions := []*wasm.FunctionInstance{
			{DebugName: "1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "3", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "4", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
		}

		// initialize the module-engine containing imported functions
		_, err := e.NewModuleEngine(t.Name(), nil, importedFunctions, nil, nil)
		require.NoError(t, err)

		require.Len(t, e.compiledFunctions, len(importedFunctions))

		moduleFunctions := []*wasm.FunctionInstance{
			{DebugName: "ok1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "ok2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "invalid code", Type: &wasm.FunctionType{}, Body: []byte{
				wasm.OpcodeCall, // Call instruction without immediate for call target index is invalid and should fail to compile.
			}, Module: &wasm.ModuleInstance{}},
		}

		_, err = e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, nil, nil)
		require.EqualError(t, err, "function[2/2] failed to lower to wazeroir: handling instruction: apply stack failed for call: reading immediates: EOF")

		// On the compilation failure, all the compiled functions including succeeded ones must be released.
		require.Len(t, e.compiledFunctions, len(importedFunctions))
		for _, f := range moduleFunctions {
			require.NotContains(t, e.compiledFunctions, f)
		}
	})
}

func TestInterpreter_Close(t *testing.T) {
	newFunctionInstance := func(id int) *wasm.FunctionInstance {
		return &wasm.FunctionInstance{
			DebugName: strconv.Itoa(id), Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}}
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
			e := et.NewEngine().(*engine)
			if len(tc.importedFunctions) > 0 {
				// initialize the module-engine containing imported functions
				me, err := e.NewModuleEngine(t.Name(), nil, tc.importedFunctions, nil, nil)
				require.NoError(t, err)
				require.Len(t, me.(*moduleEngine).compiledFunctions, len(tc.importedFunctions))
			}

			me, err := e.NewModuleEngine(t.Name(), tc.importedFunctions, tc.moduleFunctions, nil, nil)
			require.NoError(t, err)
			require.Len(t, me.(*moduleEngine).compiledFunctions, len(tc.importedFunctions)+len(tc.moduleFunctions))

			require.Len(t, e.compiledFunctions, len(tc.importedFunctions)+len(tc.moduleFunctions))
			for _, f := range tc.importedFunctions {
				require.Contains(t, e.compiledFunctions, f)
			}
			for _, f := range tc.moduleFunctions {
				require.Contains(t, e.compiledFunctions, f)
			}

			me.Close()

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
