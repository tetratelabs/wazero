package interpreter

import (
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/buildoptions"
	"github.com/tetratelabs/wazero/internal/testing/enginetest"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestInterpreter_CallEngine_PushFrame(t *testing.T) {
	f1 := &callFrame{}
	f2 := &callFrame{}

	ce := callEngine{}
	require.Equal(t, 0, len(ce.frames), "expected no frames")

	ce.pushFrame(f1)
	require.Equal(t, []*callFrame{f1}, ce.frames)

	ce.pushFrame(f2)
	require.Equal(t, []*callFrame{f1, f2}, ce.frames)
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

	captured := require.CapturePanic(func() { vm.pushFrame(f4) })
	require.EqualError(t, captured, "callstack overflow")
}

// et is used for tests defined in the enginetest package.
var et = &engineTester{}

type engineTester struct {
}

func (e engineTester) NewEngine(enabledFeatures wasm.Features) wasm.Engine {
	return NewEngine(enabledFeatures)
}

func (e engineTester) InitTable(me wasm.ModuleEngine, initTableLen uint32, initTableIdxToFnIdx map[wasm.Index]wasm.Index) []interface{} {
	table := make([]interface{}, initTableLen)
	internal := me.(*moduleEngine)
	for idx, fnidx := range initTableIdxToFnIdx {
		table[idx] = internal.functions[fnidx]
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
				f := &function{
					source: &wasm.FunctionInstance{Module: &wasm.ModuleInstance{Engine: &moduleEngine{}}},
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
				f := &function{
					source: &wasm.FunctionInstance{Module: &wasm.ModuleInstance{Engine: &moduleEngine{}}},
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

func TestInterpreter_Compile(t *testing.T) {
	t.Run("uncompiled", func(t *testing.T) {
		e := et.NewEngine(wasm.Features20191205).(*engine)
		_, err := e.NewModuleEngine("foo",
			&wasm.Module{},
			nil, // imports
			nil, // moduleFunctions
			nil, // table
			nil, // tableInit
		)
		require.EqualError(t, err, "source module for foo must be compiled before instantiation")
	})
	t.Run("fail", func(t *testing.T) {
		e := et.NewEngine(wasm.Features20191205).(*engine)

		errModule := &wasm.Module{
			TypeSection:     []*wasm.FunctionType{{}},
			FunctionSection: []wasm.Index{0, 0, 0},
			CodeSection: []*wasm.Code{
				{Body: []byte{wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeCall}}, // Call instruction without immediate for call target index is invalid and should fail to compile.
			},
			ID: wasm.ModuleID{},
		}

		err := e.CompileModule(errModule)
		require.EqualError(t, err, "failed to lower func[2/3] to wazeroir: handling instruction: apply stack failed for call: reading immediates: EOF")

		// On the compilation failure, all the compiled functions including succeeded ones must be released.
		_, ok := e.codes[errModule.ID]
		require.False(t, ok)
	})
	t.Run("ok", func(t *testing.T) {
		e := et.NewEngine(wasm.Features20191205).(*engine)

		okModule := &wasm.Module{
			TypeSection:     []*wasm.FunctionType{{}},
			FunctionSection: []wasm.Index{0, 0, 0, 0},
			CodeSection: []*wasm.Code{
				{Body: []byte{wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeEnd}},
			},
			ID: wasm.ModuleID{},
		}
		err := e.CompileModule(okModule)
		require.NoError(t, err)

		compiled, ok := e.codes[okModule.ID]
		require.True(t, ok)
		require.Equal(t, len(okModule.FunctionSection), len(compiled))

		_, ok = e.codes[okModule.ID]
		require.True(t, ok)
	})
}

func TestEngine_CachedcodesPerModule(t *testing.T) {
	e := et.NewEngine(wasm.Features20191205).(*engine)
	exp := []*code{
		{body: []*interpreterOp{}},
		{body: []*interpreterOp{}},
	}
	m := &wasm.Module{}

	e.addCodes(m, exp)

	actual, ok := e.getCodes(m)
	require.True(t, ok)
	require.Equal(t, len(exp), len(actual))
	for i := range actual {
		require.Equal(t, exp[i], actual[i])
	}

	e.deleteCodes(m)
	_, ok = e.getCodes(m)
	require.False(t, ok)
}
