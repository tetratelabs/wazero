package interpreter

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/tetratelabs/wazero/internal/buildoptions"
	"github.com/tetratelabs/wazero/internal/testing/enginetest"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

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

// engineTester implements enginetest.EngineTester.
type engineTester struct{}

// NewEngine implements enginetest.EngineTester NewEngine.
func (e engineTester) NewEngine(enabledFeatures wasm.Features) wasm.Engine {
	return NewEngine(enabledFeatures)
}

// InitTables implements enginetest.EngineTester InitTables.
func (e engineTester) InitTables(me wasm.ModuleEngine, tableIndexToLen map[wasm.Index]int, initTableIdxToFnIdx wasm.TableInitMap) [][]wasm.Reference {
	references := make([][]wasm.Reference, len(tableIndexToLen))
	for tableIndex, l := range tableIndexToLen {
		references[tableIndex] = make([]interface{}, l)
	}
	internal := me.(*moduleEngine)

	for tableIndex, init := range initTableIdxToFnIdx {
		referencesPerTable := references[tableIndex]
		for idx, fnidx := range init {
			referencesPerTable[idx] = internal.functions[fnidx]
		}
	}
	return references
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

func TestInterpreter_ModuleEngine_Memory(t *testing.T) {
	enginetest.RunTestModuleEngine_Memory(t, et)
}

func TestInterpreter_NonTrappingFloatToIntConversion(t *testing.T) {
	_0x80000000 := uint32(0x80000000)
	_0xffffffff := uint32(0xffffffff)
	_0x8000000000000000 := uint64(0x8000000000000000)
	_0xffffffffffffffff := uint64(0xffffffffffffffff)

	for _, tc := range []struct {
		op            wasm.OpcodeMisc
		inputType     wazeroir.Float
		outputType    wazeroir.SignedInt
		input32bit    []float32
		input64bit    []float64
		expected32bit []int32
		expected64bit []int64
	}{
		{
			// https://github.com/WebAssembly/spec/blob/c8fd933fa51eb0b511bce027b573aef7ee373726/test/core/conversions.wast#L261-L282
			op:         wasm.OpcodeMiscI32TruncSatF32S,
			inputType:  wazeroir.Float32,
			outputType: wazeroir.SignedInt32,
			input32bit: []float32{
				0.0, 0.0, 0x1p-149, -0x1p-149, 1.0, 0x1.19999ap+0, 1.5, -1.0, -0x1.19999ap+0,
				-1.5, -1.9, -2.0, 2147483520.0, -2147483648.0, 2147483648.0, -2147483904.0,
				float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()), float32(math.NaN()),
				float32(math.NaN()), float32(math.NaN()),
			},
			expected32bit: []int32{
				0, 0, 0, 0, 1, 1, 1, -1, -1, -1, -1, -2, 2147483520, -2147483648, 0x7fffffff,
				int32(_0x80000000), 0x7fffffff, int32(_0x80000000), 0, 0, 0, 0,
			},
		},
		{
			// https://github.com/WebAssembly/spec/blob/c8fd933fa51eb0b511bce027b573aef7ee373726/test/core/conversions.wast#L284-L304
			op:         wasm.OpcodeMiscI32TruncSatF32U,
			inputType:  wazeroir.Float32,
			outputType: wazeroir.SignedUint32,
			input32bit: []float32{
				0.0, 0.0, 0x1p-149, -0x1p-149, 1.0, 0x1.19999ap+0, 1.5, 1.9, 2.0, 2147483648, 4294967040.0,
				-0x1.ccccccp-1, -0x1.fffffep-1, 4294967296.0, -1.0, float32(math.Inf(1)), float32(math.Inf(-1)),
				float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN()),
			},
			expected32bit: []int32{
				0, 0, 0, 0, 1, 1, 1, 1, 2, -2147483648, -256, 0, 0, int32(_0xffffffff), 0x00000000,
				int32(_0xffffffff), 0x00000000, 0, 0, 0, 0,
			},
		},
		{
			// https://github.com/WebAssembly/spec/blob/c8fd933fa51eb0b511bce027b573aef7ee373726/test/core/conversions.wast#L355-L378
			op:         wasm.OpcodeMiscI64TruncSatF32S,
			inputType:  wazeroir.Float32,
			outputType: wazeroir.SignedInt64,
			input32bit: []float32{
				0.0, 0.0, 0x1p-149, -0x1p-149, 1.0, 0x1.19999ap+0, 1.5, -1.0, -0x1.19999ap+0, -1.5, -1.9, -2.0, 4294967296,
				-4294967296, 9223371487098961920.0, -9223372036854775808.0, 9223372036854775808.0, -9223373136366403584.0,
				float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()), float32(math.NaN()), float32(math.NaN()),
				float32(math.NaN()),
			},
			expected64bit: []int64{
				0, 0, 0, 0, 1, 1, 1, -1, -1, -1, -1, -2, 4294967296, -4294967296, 9223371487098961920, -9223372036854775808,
				0x7fffffffffffffff, int64(_0x8000000000000000), 0x7fffffffffffffff, int64(_0x8000000000000000), 0, 0, 0, 0,
			},
		},
		{
			// https://github.com/WebAssembly/spec/blob/c8fd933fa51eb0b511bce027b573aef7ee373726/test/core/conversions.wast#L380-L398
			op:         wasm.OpcodeMiscI64TruncSatF32U,
			inputType:  wazeroir.Float32,
			outputType: wazeroir.SignedUint64,
			input32bit: []float32{
				0.0, 0.0, 0x1p-149, -0x1p-149, 1.0, 0x1.19999ap+0, 1.5, 4294967296,
				18446742974197923840.0, -0x1.ccccccp-1, -0x1.fffffep-1, 18446744073709551616.0, -1.0,
				float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()), float32(math.NaN()),
				float32(math.NaN()), float32(math.NaN()),
			},
			expected64bit: []int64{
				0, 0, 0, 0, 1, 1, 1,
				4294967296, -1099511627776, 0, 0, int64(_0xffffffffffffffff), 0x0000000000000000,
				int64(_0xffffffffffffffff), 0x0000000000000000, 0, 0, 0, 0,
			},
		},
		{
			// https://github.com/WebAssembly/spec/blob/c8fd933fa51eb0b511bce027b573aef7ee373726/test/core/conversions.wast#L306-L327
			op:         wasm.OpcodeMiscI32TruncSatF64S,
			inputType:  wazeroir.Float64,
			outputType: wazeroir.SignedInt32,
			input64bit: []float64{
				0.0, 0.0, 0x0.0000000000001p-1022, -0x0.0000000000001p-1022, 1.0, 0x1.199999999999ap+0, 1.5, -1.0,
				-0x1.199999999999ap+0, -1.5, -1.9, -2.0, 2147483647.0, -2147483648.0, 2147483648.0,
				-2147483649.0, math.Inf(1), math.Inf(-1), math.NaN(), math.NaN(), math.NaN(), math.NaN(),
			},
			expected32bit: []int32{
				0, 0, 0, 0, 1, 1, 1, -1, -1, -1, -1, -2,
				2147483647, -2147483648, 0x7fffffff, int32(_0x80000000), 0x7fffffff, int32(_0x80000000), 0,
				0, 0, 0,
			},
		},
		{
			// https://github.com/WebAssembly/spec/blob/c8fd933fa51eb0b511bce027b573aef7ee373726/test/core/conversions.wast#L329-L353
			op:         wasm.OpcodeMiscI32TruncSatF64U,
			inputType:  wazeroir.Float64,
			outputType: wazeroir.SignedUint32,
			input64bit: []float64{
				0.0, 0.0, 0x0.0000000000001p-1022, -0x0.0000000000001p-1022, 1.0, 0x1.199999999999ap+0, 1.5, 1.9, 2.0,
				2147483648, 4294967295.0, -0x1.ccccccccccccdp-1, -0x1.fffffffffffffp-1, 1e8, 4294967296.0, -1.0, 1e16, 1e30,
				9223372036854775808, math.Inf(1), math.Inf(-1), math.NaN(), math.NaN(), math.NaN(), math.NaN(),
			},
			expected32bit: []int32{
				0, 0, 0, 0, 1, 1, 1, 1, 2, -2147483648, -1,
				0, 0, 100000000, int32(_0xffffffff), 0x00000000, int32(_0xffffffff), int32(_0xffffffff), int32(_0xffffffff),
				int32(_0xffffffff), 0x00000000, 0, 0, 0, 0,
			},
		},
		{
			// https://github.com/WebAssembly/spec/blob/c8fd933fa51eb0b511bce027b573aef7ee373726/test/core/conversions.wast#L400-L423
			op:         wasm.OpcodeMiscI64TruncSatF64S,
			inputType:  wazeroir.Float64,
			outputType: wazeroir.SignedInt64,
			input64bit: []float64{
				0.0, 0.0, 0x0.0000000000001p-1022, -0x0.0000000000001p-1022, 1.0, 0x1.199999999999ap+0, 1.5, -1.0,
				-0x1.199999999999ap+0, -1.5, -1.9, -2.0, 4294967296, -4294967296, 9223372036854774784.0, -9223372036854775808.0,
				9223372036854775808.0, -9223372036854777856.0, math.Inf(1), math.Inf(-1), math.NaN(), math.NaN(), math.NaN(),
				math.NaN(),
			},
			expected64bit: []int64{
				0, 0, 0, 0, 1, 1, 1, -1, -1, -1, -1, -2,
				4294967296, -4294967296, 9223372036854774784, -9223372036854775808, 0x7fffffffffffffff,
				int64(_0x8000000000000000), 0x7fffffffffffffff, int64(_0x8000000000000000), 0, 0, 0, 0,
			},
		},
		{
			// https://github.com/WebAssembly/spec/blob/c8fd933fa51eb0b511bce027b573aef7ee373726/test/core/conversions.wast#L425-L447
			op:         wasm.OpcodeMiscI64TruncSatF64U,
			inputType:  wazeroir.Float64,
			outputType: wazeroir.SignedUint64,
			input64bit: []float64{
				0.0, 0.0, 0x0.0000000000001p-1022, -0x0.0000000000001p-1022, 1.0, 0x1.199999999999ap+0, 1.5, 4294967295, 4294967296,
				18446744073709549568.0, -0x1.ccccccccccccdp-1, -0x1.fffffffffffffp-1, 1e8, 1e16, 9223372036854775808,
				18446744073709551616.0, -1.0, math.Inf(1), math.Inf(-1), math.NaN(), math.NaN(), math.NaN(), math.NaN(),
			},
			expected64bit: []int64{
				0, 0, 0, 0, 1, 1, 1, 0xffffffff, 0x100000000, -2048, 0, 0, 100000000, 10000000000000000,
				-9223372036854775808, int64(_0xffffffffffffffff), 0x0000000000000000, int64(_0xffffffffffffffff),
				0x0000000000000000, 0, 0, 0, 0,
			},
		},
	} {
		tc := tc
		t.Run(wasm.MiscInstructionName(tc.op), func(t *testing.T) {
			in32bit := len(tc.input32bit) > 0
			casenum := len(tc.input32bit)
			if !in32bit {
				casenum = len(tc.input64bit)
			}
			for i := 0; i < casenum; i++ {
				i := i
				t.Run(strconv.Itoa(i), func(t *testing.T) {
					var body []*interpreterOp
					if in32bit {
						body = append(body, &interpreterOp{kind: wazeroir.OperationKindConstF32,
							us: []uint64{uint64(math.Float32bits(tc.input32bit[i]))}})
					} else {
						body = append(body, &interpreterOp{kind: wazeroir.OperationKindConstF64,
							us: []uint64{uint64(math.Float64bits(tc.input64bit[i]))}})
					}

					body = append(body, &interpreterOp{
						kind: wazeroir.OperationKindITruncFromF,
						b1:   byte(tc.inputType),
						b2:   byte(tc.outputType),
						b3:   true, // NonTrapping = true.
					})

					// Return from function.
					body = append(body,
						&interpreterOp{kind: wazeroir.OperationKindBr, us: []uint64{math.MaxUint64}},
					)

					ce := &callEngine{}
					f := &function{
						source: &wasm.FunctionInstance{Module: &wasm.ModuleInstance{Engine: &moduleEngine{}}},
						body:   body,
					}
					ce.callNativeFunc(testCtx, &wasm.CallContext{}, f)

					if len(tc.expected32bit) > 0 {
						require.Equal(t, tc.expected32bit[i], int32(uint32(ce.popValue())))
					} else {
						require.Equal(t, tc.expected64bit[i], int64((ce.popValue())))
					}
				})
			}
		})

	}
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
				ce.callNativeFunc(testCtx, &wasm.CallContext{}, f)
				require.Equal(t, tc.expected, int32(uint32(ce.popValue())))
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
				ce.callNativeFunc(testCtx, &wasm.CallContext{}, f)
				require.Equal(t, tc.expected, int64(ce.popValue()))
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

		err := e.CompileModule(testCtx, errModule)
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
		err := e.CompileModule(testCtx, okModule)
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
