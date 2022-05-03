package wasm

import (
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestModule_ValidateFunction_validateFunctionWithMaxStackValues(t *testing.T) {
	const max = 100
	const valuesNum = max + 1

	// Build a function which has max+1 const instructions.
	var body []byte
	for i := 0; i < valuesNum; i++ {
		body = append(body, OpcodeI32Const, 1)
	}

	// Drop all the consts so that if the max is higher, this function body would be sound.
	for i := 0; i < valuesNum; i++ {
		body = append(body, OpcodeDrop)
	}

	// Plus all functions must end with End opcode.
	body = append(body, OpcodeEnd)

	m := &Module{
		TypeSection:     []*FunctionType{v_v},
		FunctionSection: []Index{0},
		CodeSection:     []*Code{{Body: body}},
	}

	t.Run("not exceed", func(t *testing.T) {
		err := m.validateFunctionWithMaxStackValues(Features20191205, 0, []Index{0}, nil, nil, nil, max+1)
		require.NoError(t, err)
	})
	t.Run("exceed", func(t *testing.T) {
		err := m.validateFunctionWithMaxStackValues(Features20191205, 0, []Index{0}, nil, nil, nil, max)
		require.Error(t, err)
		expMsg := fmt.Sprintf("function may have %d stack values, which exceeds limit %d", valuesNum, max)
		require.Equal(t, expMsg, err.Error())
	})
}

func TestModule_ValidateFunction_SignExtensionOps(t *testing.T) {
	tests := []struct {
		input                Opcode
		expectedErrOnDisable string
	}{
		{
			input:                OpcodeI32Extend8S,
			expectedErrOnDisable: "i32.extend8_s invalid as feature \"sign-extension-ops\" is disabled",
		},
		{
			input:                OpcodeI32Extend16S,
			expectedErrOnDisable: "i32.extend16_s invalid as feature \"sign-extension-ops\" is disabled",
		},
		{
			input:                OpcodeI64Extend8S,
			expectedErrOnDisable: "i64.extend8_s invalid as feature \"sign-extension-ops\" is disabled",
		},
		{
			input:                OpcodeI64Extend16S,
			expectedErrOnDisable: "i64.extend16_s invalid as feature \"sign-extension-ops\" is disabled",
		},
		{
			input:                OpcodeI64Extend32S,
			expectedErrOnDisable: "i64.extend32_s invalid as feature \"sign-extension-ops\" is disabled",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(InstructionName(tc.input), func(t *testing.T) {
			t.Run("disabled", func(t *testing.T) {
				m := &Module{
					TypeSection:     []*FunctionType{v_v},
					FunctionSection: []Index{0},
					CodeSection:     []*Code{{Body: []byte{tc.input}}},
				}
				err := m.validateFunction(Features20191205, 0, []Index{0}, nil, nil, nil)
				require.EqualError(t, err, tc.expectedErrOnDisable)
			})
			t.Run("enabled", func(t *testing.T) {
				is32bit := tc.input == OpcodeI32Extend8S || tc.input == OpcodeI32Extend16S
				var body []byte
				if is32bit {
					body = append(body, OpcodeI32Const)
				} else {
					body = append(body, OpcodeI64Const)
				}
				body = append(body, tc.input, 123, OpcodeDrop, OpcodeEnd)
				m := &Module{
					TypeSection:     []*FunctionType{v_v},
					FunctionSection: []Index{0},
					CodeSection:     []*Code{{Body: body}},
				}
				err := m.validateFunction(FeatureSignExtensionOps, 0, []Index{0}, nil, nil, nil)
				require.NoError(t, err)
			})
		})
	}
}

func TestModule_ValidateFunction_NonTrappingFloatToIntConversion(t *testing.T) {
	tests := []struct {
		input                Opcode
		expectedErrOnDisable string
	}{
		{
			input:                OpcodeMiscI32TruncSatF32S,
			expectedErrOnDisable: "i32.trunc_sat_f32_s invalid as feature \"nontrapping-float-to-int-conversion\" is disabled",
		},
		{
			input:                OpcodeMiscI32TruncSatF32U,
			expectedErrOnDisable: "i32.trunc_sat_f32_u invalid as feature \"nontrapping-float-to-int-conversion\" is disabled",
		},
		{
			input:                OpcodeMiscI32TruncSatF64S,
			expectedErrOnDisable: "i32.trunc_sat_f64_s invalid as feature \"nontrapping-float-to-int-conversion\" is disabled",
		},
		{
			input:                OpcodeMiscI32TruncSatF64U,
			expectedErrOnDisable: "i32.trunc_sat_f64_u invalid as feature \"nontrapping-float-to-int-conversion\" is disabled",
		},
		{
			input:                OpcodeMiscI64TruncSatF32S,
			expectedErrOnDisable: "i64.trunc_sat_f32_s invalid as feature \"nontrapping-float-to-int-conversion\" is disabled",
		},
		{
			input:                OpcodeMiscI64TruncSatF32U,
			expectedErrOnDisable: "i64.trunc_sat_f32_u invalid as feature \"nontrapping-float-to-int-conversion\" is disabled",
		},
		{
			input:                OpcodeMiscI64TruncSatF64S,
			expectedErrOnDisable: "i64.trunc_sat_f64_s invalid as feature \"nontrapping-float-to-int-conversion\" is disabled",
		},
		{
			input:                OpcodeMiscI64TruncSatF64U,
			expectedErrOnDisable: "i64.trunc_sat_f64_u invalid as feature \"nontrapping-float-to-int-conversion\" is disabled",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(InstructionName(tc.input), func(t *testing.T) {
			t.Run("disabled", func(t *testing.T) {
				m := &Module{
					TypeSection:     []*FunctionType{v_v},
					FunctionSection: []Index{0},
					CodeSection:     []*Code{{Body: []byte{OpcodeMiscPrefix, tc.input}}},
				}
				err := m.validateFunction(Features20191205, 0, []Index{0}, nil, nil, nil)
				require.EqualError(t, err, tc.expectedErrOnDisable)
			})
			t.Run("enabled", func(t *testing.T) {
				var body []byte
				switch tc.input {
				case OpcodeMiscI32TruncSatF32S, OpcodeMiscI32TruncSatF32U, OpcodeMiscI64TruncSatF32S, OpcodeMiscI64TruncSatF32U:
					body = []byte{OpcodeF32Const, 1, 2, 3, 4}
				case OpcodeMiscI32TruncSatF64S, OpcodeMiscI32TruncSatF64U, OpcodeMiscI64TruncSatF64S, OpcodeMiscI64TruncSatF64U:
					body = []byte{OpcodeF64Const, 1, 2, 3, 4, 5, 6, 7, 8}
				}
				body = append(body, OpcodeMiscPrefix, tc.input, OpcodeDrop, OpcodeEnd)

				m := &Module{
					TypeSection:     []*FunctionType{v_v},
					FunctionSection: []Index{0},
					CodeSection:     []*Code{{Body: body}},
				}
				err := m.validateFunction(FeatureNonTrappingFloatToIntConversion, 0, []Index{0}, nil, nil, nil)
				require.NoError(t, err)
			})
		})
	}
}

// TestModule_ValidateFunction_MultiValue only tests what can't yet be detected during compilation. These examples are
// from test/core/if.wast from the commit that added "multi-value" support.
//
// See https://github.com/WebAssembly/spec/commit/484180ba3d9d7638ba1cb400b699ffede796927c
func TestModule_ValidateFunction_MultiValue(t *testing.T) {
	tests := []struct {
		name                 string
		module               *Module
		expectedErrOnDisable string
	}{
		{
			name: "block with function type",
			module: &Module{
				TypeSection:     []*FunctionType{v_f64f64},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0, // (block (result f64 f64)
					OpcodeF64Const, 0, 0, 0, 0, 0, 0, 0x10, 0x40, // (f64.const 4)
					OpcodeF64Const, 0, 0, 0, 0, 0, 0, 0x14, 0x40, // (f64.const 5)
					OpcodeBr, 0,
					OpcodeF64Add,
					OpcodeF64Const, 0, 0, 0, 0, 0, 0, 0x18, 0x40, // (f64.const 6)
					OpcodeEnd,
					OpcodeEnd,
				}}},
			},
			expectedErrOnDisable: "read block: block with function type return invalid as feature \"multi-value\" is disabled",
		},
		{
			name: "if with function type", // a.k.a. "param"
			module: &Module{
				TypeSection:     []*FunctionType{i32_i32}, // (func (param i32) (result i32)
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, // (i32.const 1)
					OpcodeLocalGet, 0, OpcodeIf, 0, // (if (param i32) (result i32) (local.get 0)
					OpcodeI32Const, 2, OpcodeI32Add, // (then (i32.const 2) (i32.add))
					OpcodeElse, OpcodeI32Const, 0x7e, OpcodeI32Add, // (else (i32.const -2) (i32.add))
					OpcodeEnd, // )
					OpcodeEnd, // )
				}}},
			},
			expectedErrOnDisable: "read block: block with function type return invalid as feature \"multi-value\" is disabled",
		},
		{
			name: "if with function type - br", // a.k.a. "params-break"
			module: &Module{
				TypeSection: []*FunctionType{
					i32_i32,    // (func (param i32) (result i32)
					i32i32_i32, // (if (param i32 i32) (result i32)
				},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, // (i32.const 1)
					OpcodeI32Const, 2, // (i32.const 2)
					OpcodeLocalGet, 0, OpcodeIf, 1, // (if (param i32) (result i32) (local.get 0)
					OpcodeI32Add, OpcodeBr, 0, // (then (i32.add) (br 0))
					OpcodeElse, OpcodeI32Sub, OpcodeBr, 0, // (else (i32.sub) (br 0))
					OpcodeEnd, // )
					OpcodeEnd, // )
				}}},
			},
			expectedErrOnDisable: "read block: block with function type return invalid as feature \"multi-value\" is disabled",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Run("disabled", func(t *testing.T) {
				err := tc.module.validateFunction(Features20191205, 0, []Index{0}, nil, nil, nil)
				require.EqualError(t, err, tc.expectedErrOnDisable)
			})
			t.Run("enabled", func(t *testing.T) {
				err := tc.module.validateFunction(FeatureMultiValue, 0, []Index{0}, nil, nil, nil)
				require.NoError(t, err)
			})
		})
	}
}

func TestModule_ValidateFunction_BulkMemoryOperations(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		for _, op := range []OpcodeMisc{
			OpcodeMiscMemoryInit, OpcodeMiscDataDrop, OpcodeMiscMemoryCopy,
			OpcodeMiscMemoryFill, OpcodeMiscTableInit, OpcodeMiscElemDrop, OpcodeMiscTableCopy,
		} {
			t.Run(MiscInstructionName(op), func(t *testing.T) {
				var body []byte
				if op != OpcodeMiscDataDrop && op != OpcodeMiscElemDrop {
					body = append(body, OpcodeI32Const, 1, OpcodeI32Const, 2, OpcodeI32Const, 3)
				}

				body = append(body, OpcodeMiscPrefix, op)
				if op != OpcodeMiscDataDrop && op != OpcodeMiscMemoryFill && op != OpcodeMiscElemDrop {
					body = append(body, 0, 0)
				} else {
					body = append(body, 0)
				}

				body = append(body, OpcodeEnd)

				c := uint32(0)
				m := &Module{
					TypeSection:      []*FunctionType{v_v},
					FunctionSection:  []Index{0},
					CodeSection:      []*Code{{Body: body}},
					DataSection:      []*DataSegment{{}},
					ElementSection:   []*ElementSegment{{}},
					DataCountSection: &c,
				}
				err := m.validateFunction(FeatureBulkMemoryOperations, 0, []Index{0}, nil, &Memory{}, []*Table{{}, {}})
				require.NoError(t, err)
			})
		}
	})
	t.Run("errors", func(t *testing.T) {
		for _, tc := range []struct {
			body                []byte
			dataSection         []*DataSegment
			elementSection      []*ElementSegment
			dataCountSectionNil bool
			memory              *Memory
			tables              []*Table
			flag                Features
			expectedErr         string
		}{
			// memory.init
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit},
				flag:        FeatureBulkMemoryOperations,
				memory:      nil,
				expectedErr: "memory must exist for memory.init",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit},
				flag:        Features20191205,
				expectedErr: `memory.init invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:                []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit},
				flag:                FeatureBulkMemoryOperations,
				dataCountSectionNil: true,
				expectedErr:         `memory must exist for memory.init`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "failed to read data segment index for memory.init: EOF",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit, 100 /* data section out of range */},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []*DataSegment{{}},
				expectedErr: "index 100 out of range of data section(len=1)",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit, 0},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []*DataSegment{{}},
				expectedErr: "failed to read memory index for memory.init: EOF",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit, 0, 1},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []*DataSegment{{}},
				expectedErr: "memory.init reserved byte must be zero encoded with 1 byte",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit, 0, 0},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []*DataSegment{{}},
				expectedErr: "cannot pop the operand for memory.init: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscMemoryInit, 0, 0},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []*DataSegment{{}},
				expectedErr: "cannot pop the operand for memory.init: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscMemoryInit, 0, 0},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []*DataSegment{{}},
				expectedErr: "cannot pop the operand for memory.init: i32 missing",
			},
			// data.drop
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscDataDrop},
				flag:        Features20191205,
				expectedErr: `data.drop invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:                []byte{OpcodeMiscPrefix, OpcodeMiscDataDrop},
				dataCountSectionNil: true,
				memory:              &Memory{},
				flag:                FeatureBulkMemoryOperations,
				expectedErr:         `data.drop requires data count section`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscDataDrop},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "failed to read data segment index for data.drop: EOF",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscDataDrop, 100 /* data section out of range */},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []*DataSegment{{}},
				expectedErr: "index 100 out of range of data section(len=1)",
			},
			// memory.copy
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryCopy},
				flag:        FeatureBulkMemoryOperations,
				memory:      nil,
				expectedErr: "memory must exist for memory.copy",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryCopy},
				flag:        Features20191205,
				expectedErr: `memory.copy invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryCopy},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: `failed to read memory index for memory.copy: EOF`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryCopy, 0},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "failed to read memory index for memory.copy: EOF",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryCopy, 0, 1},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "memory.copy reserved byte must be zero encoded with 1 byte",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryCopy, 0, 0},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "cannot pop the operand for memory.copy: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscMemoryCopy, 0, 0},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "cannot pop the operand for memory.copy: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscMemoryCopy, 0, 0},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "cannot pop the operand for memory.copy: i32 missing",
			},
			// memory.fill
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryFill},
				flag:        FeatureBulkMemoryOperations,
				memory:      nil,
				expectedErr: "memory must exist for memory.fill",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryFill},
				flag:        Features20191205,
				expectedErr: `memory.fill invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryFill},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: `failed to read memory index for memory.fill: EOF`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryFill, 1},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: `memory.fill reserved byte must be zero encoded with 1 byte`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryFill, 0},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "cannot pop the operand for memory.fill: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscMemoryFill, 0},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "cannot pop the operand for memory.fill: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscMemoryFill, 0},
				flag:        FeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "cannot pop the operand for memory.fill: i32 missing",
			},
			// table.init
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableInit},
				flag:        Features20191205,
				tables:      []*Table{{}},
				expectedErr: `table.init invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableInit},
				flag:        FeatureBulkMemoryOperations,
				tables:      []*Table{{}},
				expectedErr: "failed to read element segment index for table.init: EOF",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscTableInit, 100 /* data section out of range */},
				flag:           FeatureBulkMemoryOperations,
				tables:         []*Table{{}},
				elementSection: []*ElementSegment{{}},
				expectedErr:    "index 100 out of range of element section(len=1)",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscTableInit, 0},
				flag:           FeatureBulkMemoryOperations,
				tables:         []*Table{{}},
				elementSection: []*ElementSegment{{}},
				expectedErr:    "failed to read source table index for table.init: EOF",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscTableInit, 0, 10},
				flag:           FeatureBulkMemoryOperations,
				tables:         []*Table{{}},
				elementSection: []*ElementSegment{{}},
				expectedErr:    "source table index must be zero for table.init as feature \"reference-types\" is disabled",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscTableInit, 0, 10},
				flag:           FeatureBulkMemoryOperations | FeatureReferenceTypes,
				tables:         []*Table{{}},
				elementSection: []*ElementSegment{{}},
				expectedErr:    "table of index 10 not found",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscTableInit, 0, 1},
				flag:           FeatureBulkMemoryOperations | FeatureReferenceTypes,
				tables:         []*Table{{}, {Type: RefTypeExternref}},
				elementSection: []*ElementSegment{{Type: RefTypeFuncref}},
				expectedErr:    "type mismatch for table.init: element type funcref does not match table type externref",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscTableInit, 0, 0},
				flag:           FeatureBulkMemoryOperations,
				tables:         []*Table{{}},
				elementSection: []*ElementSegment{{}},
				expectedErr:    "cannot pop the operand for table.init: i32 missing",
			},
			{
				body:           []byte{OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscTableInit, 0, 0},
				flag:           FeatureBulkMemoryOperations,
				tables:         []*Table{{}},
				elementSection: []*ElementSegment{{}},
				expectedErr:    "cannot pop the operand for table.init: i32 missing",
			},
			{
				body:           []byte{OpcodeI32Const, 0, OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscTableInit, 0, 0},
				flag:           FeatureBulkMemoryOperations,
				tables:         []*Table{{}},
				elementSection: []*ElementSegment{{}},
				expectedErr:    "cannot pop the operand for table.init: i32 missing",
			},
			// elem.drop
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscElemDrop},
				flag:        Features20191205,
				tables:      []*Table{{}},
				expectedErr: `elem.drop invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscElemDrop},
				flag:        FeatureBulkMemoryOperations,
				tables:      []*Table{{}},
				expectedErr: "failed to read element segment index for elem.drop: EOF",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscElemDrop, 100 /* element section out of range */},
				flag:           FeatureBulkMemoryOperations,
				tables:         []*Table{{}},
				elementSection: []*ElementSegment{{}},
				expectedErr:    "index 100 out of range of element section(len=1)",
			},
			// table.copy
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy},
				flag:        Features20191205,
				tables:      []*Table{{}},
				expectedErr: `table.copy invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy},
				flag:        FeatureBulkMemoryOperations,
				tables:      []*Table{{}},
				expectedErr: `failed to read destination table index for table.copy: EOF`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy, 10},
				flag:        FeatureBulkMemoryOperations,
				tables:      []*Table{{}},
				expectedErr: "destination table index must be zero for table.copy as feature \"reference-types\" is disabled",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy, 3},
				flag:        FeatureBulkMemoryOperations | FeatureReferenceTypes,
				tables:      []*Table{{}, {}},
				expectedErr: "table of index 3 not found",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy, 3},
				flag:        FeatureBulkMemoryOperations | FeatureReferenceTypes,
				tables:      []*Table{{}, {}, {}, {}},
				expectedErr: "failed to read source table index for table.copy: EOF",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy, 0, 3},
				flag:        FeatureBulkMemoryOperations,
				tables:      []*Table{{}, {}, {}, {}},
				expectedErr: "source table index must be zero for table.copy as feature \"reference-types\" is disabled",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy, 3, 1},
				flag:        FeatureBulkMemoryOperations | FeatureReferenceTypes,
				tables:      []*Table{{}, {Type: RefTypeFuncref}, {}, {Type: RefTypeExternref}},
				expectedErr: "table type mismatch for table.copy: funcref (src) != externref (dst)",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy, 0, 0},
				flag:        FeatureBulkMemoryOperations,
				tables:      []*Table{{}},
				expectedErr: "cannot pop the operand for table.copy: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscTableCopy, 0, 0},
				flag:        FeatureBulkMemoryOperations,
				tables:      []*Table{{}},
				expectedErr: "cannot pop the operand for table.copy: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscTableCopy, 0, 0},
				flag:        FeatureBulkMemoryOperations,
				tables:      []*Table{{}},
				expectedErr: "cannot pop the operand for table.copy: i32 missing",
			},
		} {
			t.Run(tc.expectedErr, func(t *testing.T) {
				m := &Module{
					TypeSection:     []*FunctionType{v_v},
					FunctionSection: []Index{0},
					CodeSection:     []*Code{{Body: tc.body}},
					ElementSection:  tc.elementSection,
					DataSection:     tc.dataSection,
				}
				if !tc.dataCountSectionNil {
					c := uint32(0)
					m.DataCountSection = &c
				}
				err := m.validateFunction(tc.flag, 0, []Index{0}, nil, tc.memory, tc.tables)
				require.EqualError(t, err, tc.expectedErr)
			})
		}
	})
}

var (
	f32, f64, i32, i64 = ValueTypeF32, ValueTypeF64, ValueTypeI32, ValueTypeI64
	f32i32_v           = &FunctionType{Params: []ValueType{f32, i32}}
	i32_i32            = &FunctionType{Params: []ValueType{i32}, Results: []ValueType{i32}}
	i32f64_v           = &FunctionType{Params: []ValueType{i32, f64}}
	i32i32_i32         = &FunctionType{Params: []ValueType{i32, i32}, Results: []ValueType{i32}}
	i32_v              = &FunctionType{Params: []ValueType{i32}}
	v_v                = &FunctionType{}
	v_f32              = &FunctionType{Results: []ValueType{f32}}
	v_f32f32           = &FunctionType{Results: []ValueType{f32, f32}}
	v_f64i32           = &FunctionType{Results: []ValueType{f64, i32}}
	v_f64f64           = &FunctionType{Results: []ValueType{f64, f64}}
	v_i32              = &FunctionType{Results: []ValueType{i32}}
	v_i32i32           = &FunctionType{Results: []ValueType{i32, i32}}
	v_i32i64           = &FunctionType{Results: []ValueType{i32, i64}}
	v_i64i64           = &FunctionType{Results: []ValueType{i64, i64}}
)

// TestModule_ValidateFunction_TypeMismatchSpecTests are "type mismatch" tests when "multi-value" was merged.
//
// See https://github.com/WebAssembly/spec/commit/484180ba3d9d7638ba1cb400b699ffede796927c
func TestModule_ValidateFunction_MultiValue_TypeMismatch(t *testing.T) {
	tests := []struct {
		name            string
		module          *Module
		expectedErr     string
		enabledFeatures Features
	}{
		// test/core/func.wast

		{
			name: `func.wast - type-empty-f64-i32`,
			module: &Module{
				TypeSection:     []*FunctionType{v_f64i32},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{{Body: []byte{OpcodeEnd}}},
			},
			expectedErr: `not enough results
	have ()
	want (f64, i32)`,
		},
		{
			name: `func.wast - type-value-void-vs-nums`,
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{{Body: []byte{OpcodeNop, OpcodeEnd}}},
			},
			expectedErr: `not enough results
	have ()
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-value-nums-vs-void`,
			module: &Module{
				TypeSection:     []*FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{{Body: []byte{OpcodeI32Const, 0, OpcodeI64Const, 0, OpcodeEnd}}},
			},
			expectedErr: `too many results
	have (i32, i64)
	want ()`,
		},
		{
			name: `func.wast - type-value-num-vs-nums - v_f32f32 -> f32`,
			module: &Module{
				TypeSection:     []*FunctionType{v_f32f32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeF32Const, 0, 0, 0, 0, // (f32.const 0)
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results
	have (f32)
	want (f32, f32)`,
		},
		{
			name: `func.wast - type-value-num-vs-nums - v_f32 -> f32f32`,
			module: &Module{
				TypeSection:     []*FunctionType{v_f32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeF32Const, 0, 0, 0, 0, OpcodeF32Const, 0, 0, 0, 0, // (f32.const 0) (f32.const 0)
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `too many results
	have (f32, f32)
	want (f32)`,
		},
		{
			name: `func.wast - type-return-last-empty-vs-nums`,
			module: &Module{
				TypeSection:     []*FunctionType{v_f32f32},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{{Body: []byte{OpcodeReturn, OpcodeEnd}}},
			},
			expectedErr: `not enough results
	have ()
	want (f32, f32)`,
		},
		{
			name: `func.wast - type-return-last-void-vs-nums`,
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i64},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{{Body: []byte{OpcodeNop, OpcodeReturn, OpcodeEnd}}}, // (return (nop))
			},
			expectedErr: `not enough results
	have ()
	want (i32, i64)`,
		},
		{
			name: `func.wast - type-return-last-num-vs-nums`,
			module: &Module{
				TypeSection:     []*FunctionType{v_i64i64},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI64Const, 0, OpcodeReturn, // (return (i64.const 0))
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results
	have (i64)
	want (i64, i64)`,
		},
		{
			name: `func.wast - type-return-empty-vs-nums`,
			// This should err because (return) precedes the values expected in the signature (i32i32):
			//	(module (func $type-return-empty-vs-nums (result i32 i32)
			//	  (return) (i32.const 1) (i32.const 2)
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeReturn, OpcodeI32Const, 1, OpcodeI32Const, 2,
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results
	have ()
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-return-partial-vs-nums`,
			// This should err because (return) precedes one of the values expected in the signature (i32i32):
			//	(module (func $type-return-partial-vs-nums (result i32 i32)
			//	  (i32.const 1) (return) (i32.const 2)
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeReturn, OpcodeI32Const, 2,
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-return-void-vs-nums`,
			// This should err because (return) is empty due to nop, but the signature requires i32i32:
			//	(module (func $type-return-void-vs-nums (result i32 i32)
			//	  (return (nop)) (i32.const 1)
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeNop, OpcodeReturn, // (return (nop))
					OpcodeI32Const, 1, // (i32.const 1)
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results
	have ()
	want (i32, i32)`,
		},

		{
			name: `func.wast - type-return-num-vs-nums`,
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI64Const, 1, OpcodeReturn, // (return (i64.const 1))
					OpcodeI32Const, 1, OpcodeI32Const, 2, // (i32.const 1) (i32.const 2)
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results
	have (i64)
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-return-first-num-vs-nums`,
			// This should err because the return block doesn't return enough values.
			//	(module (func $type-return-first-num-vs-nums (result i32 i32)
			//	  (return (i32.const 1)) (return (i32.const 1) (i32.const 2))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI64Const, 1, OpcodeReturn, // (return (i64.const 1))
					OpcodeI32Const, 1, OpcodeI32Const, 2, OpcodeReturn, // (return (i32.const 1) (i32.const 2))
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results
	have (i64)
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-break-last-num-vs-nums`,
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 0, OpcodeBr, 0, // (br 0 (i32.const 0))
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-break-void-vs-nums`,
			// This should err because (br 0) returns no values, but its enclosing function requires two:
			//	(module (func $type-break-void-vs-nums (result i32 i32)
			//	  (br 0) (i32.const 1) (i32.const 2)
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBr, 0, // (br 0)
					OpcodeI32Const, 1, OpcodeI32Const, 2, // (i32.const 1) (i32.const 2)
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have ()
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-break-num-vs-nums`,
			// This should err because (br 0) returns one value, but its enclosing function requires two:
			//	(module (func $type-break-num-vs-nums (result i32 i32)
			//	  (br 0 (i32.const 1)) (i32.const 1) (i32.const 2)
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeBr, 0, // (br 0 (i32.const 1))
					OpcodeI32Const, 1, OpcodeI32Const, 2, // (i32.const 1) (i32.const 2)
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-break-nested-empty-vs-nums`,
			// This should err because (br 1) doesn't return values, but its enclosing function does:
			//	(module (func $type-break-nested-empty-vs-nums (result i32 i32)
			//	  (block (br 1)) (br 0 (i32.const 1) (i32.const 2))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x40, OpcodeBr, 0x01, OpcodeEnd, // (block (br 1))
					OpcodeI32Const, 1, OpcodeI32Const, 2, OpcodeBr, 0, // (br 0 (i32.const 1) (i32.const 2))
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have ()
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-break-nested-void-vs-nums`,
			// This should err because nop returns the empty type, but the enclosing function returns i32i32:
			//	(module (func $type-break-nested-void-vs-nums (result i32 i32)
			//	  (block (br 1 (nop))) (br 0 (i32.const 1) (i32.const 2))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x40, OpcodeNop, OpcodeBr, 0x01, OpcodeEnd, // (block (br 1 (nop)))
					OpcodeI32Const, 1, OpcodeI32Const, 2, OpcodeBr, 0, // (br 0 (i32.const 1) (i32.const 2))
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have ()
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-break-nested-num-vs-nums`,
			// This should err because the block signature is v_i32, but the enclosing function is v_i32i32:
			//	(module (func $type-break-nested-num-vs-nums (result i32 i32)
			//	  (block (result i32) (br 1 (i32.const 1))) (br 0 (i32.const 1) (i32.const 2))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x7f, OpcodeI32Const, 1, OpcodeBr, 1, OpcodeEnd, // (block (result i32) (br 1 (i32.const 1)))
					OpcodeI32Const, 1, OpcodeI32Const, 2, OpcodeBr, 0, // (br 0 (i32.const 1) (i32.const 2))
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have (i32)
	want (i32, i32)`,
		},

		// test/core/if.wast
		{
			name: `if.wast - wrong signature for if type use`,
			// This should err because (br 0) returns no values, but its enclosing function requires two:
			//  (module
			//    (type $sig (func))
			//    (func (i32.const 1) (if (type $sig) (i32.const 0) (then)))
			//  )
			module: &Module{
				TypeSection:     []*FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, // (i32.const 1)
					OpcodeI32Const, 0, OpcodeIf, 0, // (if (type $sig) (i32.const 0)
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `too many results
	have (i32)
	want ()`,
		},
		{
			name: `if.wast - type-then-value-nums-vs-void`,
			// This should err because (if) without a type use returns no values, but its (then) returns two:
			//	(module (func $type-then-value-nums-vs-void
			//	  (if (i32.const 1) (then (i32.const 1) (i32.const 2)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x40, // (if (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 2, // (then (i32.const 1) (i32.const 2))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `too many results in if block
	have (i32, i32)
	want ()`,
		},
		{
			name: `if.wast - type-then-value-nums-vs-void-else`,
			// This should err because (if) without a type use returns no values, but its (then) returns two:
			//	(module (func $type-then-value-nums-vs-void-else
			//	  (if (i32.const 1) (then (i32.const 1) (i32.const 2)) (else))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x40, // (if (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 2, // (then (i32.const 1) (i32.const 2))
					OpcodeElse, // (else)
					OpcodeEnd,  // if
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: `too many results in if block
	have (i32, i32)
	want ()`,
		},
		{
			name: `if.wast - type-else-value-nums-vs-void`,
			// This should err because (if) without a type use returns no values, but its (else) returns two:
			//	(module (func $type-else-value-nums-vs-void
			//	  (if (i32.const 1) (then) (else (i32.const 1) (i32.const 2)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x40, // (if (i32.const 1) (then)
					OpcodeElse, OpcodeI32Const, 1, OpcodeI32Const, 2, // (else (i32.const 1) (i32.const 2))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `too many results in else block
	have (i32, i32)
	want ()`,
		},
		{
			name: `if.wast - type-both-value-nums-vs-void`,
			// This should err because (if) without a type use returns no values, each branch returns two:
			//	(module (func $type-both-value-nums-vs-void
			//	  (if (i32.const 1) (then (i32.const 1) (i32.const 2)) (else (i32.const 2) (i32.const 1)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x40, // (if (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 2, // (then (i32.const 1) (i32.const 2))
					OpcodeElse, OpcodeI32Const, 2, OpcodeI32Const, 1, // (else (i32.const 2) (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `too many results in if block
	have (i32, i32)
	want ()`,
		},
		{
			name: `if.wast - type-then-value-empty-vs-nums`,
			// This should err because the if branch is empty, but its type use requires two i32s:
			//	(module (func $type-then-value-empty-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1) (then) (else (i32.const 0) (i32.const 2)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x01, // (if (result i32 i32) (i32.const 1) (then)
					OpcodeElse, OpcodeI32Const, 0, OpcodeI32Const, 2, // (else (i32.const 0) (i32.const 2)))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in if block
	have ()
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-else-value-empty-vs-nums`,
			// This should err because the else branch is empty, but its type use requires two i32s:
			//	(module (func $type-else-value-empty-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1) (then (i32.const 0) (i32.const 1)) (else))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x01, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 0, OpcodeI32Const, 2, // (then (i32.const 0) (i32.const 1))
					OpcodeElse, // (else)
					OpcodeEnd,  // if
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: `not enough results in else block
	have ()
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-both-value-empty-vs-nums`,
			// This should err because the both branches are empty, but the if type use requires two i32s:
			//	(module (func $type-both-value-empty-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1) (then) (else))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x01, // (if (result i32 i32) (i32.const 1) (then)
					OpcodeElse, // (else)
					OpcodeEnd,  // if
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: `not enough results in if block
	have ()
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-no-else-vs-nums`,
			// This should err because the else branch is missing, but its type use requires two i32s:
			//	(module (func $type-no-else-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1) (then (i32.const 1) (i32.const 1)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x01, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 1, // (then (i32.const 1) (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in else block
	have ()
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-then-value-void-vs-nums`,
			// This should err because the then branch evaluates to empty, but its type use requires two i32s:
			//	(module (func $type-then-value-void-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1) (then (nop)) (else (i32.const 0) (i32.const 0)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x01, // (if (result i32 i32) (i32.const 1)
					OpcodeNop,                                        // (then (nop))
					OpcodeElse, OpcodeI32Const, 1, OpcodeI32Const, 1, // (else (i32.const 1) (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in if block
	have ()
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-then-value-void-vs-nums`,
			// This should err because the else branch evaluates to empty, but its type use requires two i32s:
			//	(module (func $type-else-value-void-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1) (then (i32.const 0) (i32.const 0)) (else (nop)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 0, OpcodeI32Const, 0, // (then (i32.const 0) (i32.const 0))
					OpcodeElse, OpcodeNop, // (else (nop))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in else block
	have ()
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-both-value-void-vs-nums`,
			// This should err because the if branch evaluates to empty, but its type use requires two i32s:
			//	(module (func $type-both-value-void-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1) (then (nop)) (else (nop)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeNop,             // (then (nop))
					OpcodeElse, OpcodeNop, // (else (nop))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in if block
	have ()
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-then-value-num-vs-nums`,
			// This should err because the if branch returns one value, but its type use requires two:
			//	(module (func $type-then-value-num-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1) (then (i32.const 1)) (else (i32.const 1) (i32.const 1)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 1, // (then (i32.const 1))
					OpcodeElse, OpcodeI32Const, 1, OpcodeI32Const, 1, // (else (i32.const 1) (i32.const 1)))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in if block
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-else-value-num-vs-nums`,
			// This should err because the else branch returns one value, but its type use requires two:
			//	(module (func $type-else-value-num-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1) (then (i32.const 1) (i32.const 1)) (else (i32.const 1)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 1, // (then (i32.const 1) (i32.const 1))
					OpcodeElse, OpcodeI32Const, 1, // (else (i32.const 1)))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in else block
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-both-value-num-vs-nums`,
			// This should err because the if branch returns one value, but its type use requires two:
			//	(module (func $type-both-value-num-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1) (then (i32.const 1)) (else (i32.const 1)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 1, // (then (i32.const 1))
					OpcodeElse, OpcodeI32Const, 1, // (else (i32.const 1)))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in if block
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-then-value-partial-vs-nums`,
			// This should err because the if branch returns one value, but its type use requires two:
			//	(module (func $type-then-value-partial-vs-nums (result i32 i32)
			//	  (i32.const 0)
			//	  (if (result i32 i32) (i32.const 1) (then (i32.const 1)) (else (i32.const 1) (i32.const 1)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 0, // (i32.const 0) - NOTE: this is outside the (if)
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 1, // (then (i32.const 1))
					OpcodeElse, OpcodeI32Const, 1, OpcodeI32Const, 1, // (else (i32.const 1) (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in if block
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-else-value-partial-vs-nums`,
			// This should err because the else branch returns one value, but its type use requires two:
			//	(module (func $type-else-value-partial-vs-nums (result i32 i32)
			//	  (i32.const 0)
			//	  (if (result i32 i32) (i32.const 1) (then (i32.const 1) (i32.const 1)) (else (i32.const 1)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 0, // (i32.const 0) - NOTE: this is outside the (if)
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 1, // (then (i32.const 1) (i32.const 1))
					OpcodeElse, OpcodeI32Const, 1, // (else (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in else block
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-both-value-partial-vs-nums`,
			// This should err because the if branch returns one value, but its type use requires two:
			//  (module (func $type-both-value-partial-vs-nums (result i32 i32)
			//    (i32.const 0)
			//    (if (result i32 i32) (i32.const 1) (then (i32.const 1)) (else (i32.const 1)))
			//  ))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 0, // (i32.const 0) - NOTE: this is outside the (if)
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 1, // (then (i32.const 1))
					OpcodeElse, OpcodeI32Const, 1, // (else (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in if block
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-then-value-nums-vs-num`,
			// This should err because the if branch returns two values, but its type use requires one:
			//	(module (func $type-then-value-nums-vs-num (result i32)
			//	  (if (result i32) (i32.const 1) (then (i32.const 1) (i32.const 1)) (else (i32.const 1)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32) (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 1, // (then (i32.const 1) (i32.const 1))
					OpcodeElse, OpcodeI32Const, 1, // (else (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `too many results in if block
	have (i32, i32)
	want (i32)`,
		},
		{
			name: `if.wast - type-else-value-nums-vs-num`,
			// This should err because the else branch returns two values, but its type use requires one:
			//	(module (func $type-else-value-nums-vs-num (result i32)
			//	  (if (result i32) (i32.const 1) (then (i32.const 1)) (else (i32.const 1) (i32.const 1)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32) (i32.const 1)
					OpcodeI32Const, 1, // (then (i32.const 1))
					OpcodeElse, OpcodeI32Const, 1, OpcodeI32Const, 1, // (else (i32.const 1) (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `too many results in else block
	have (i32, i32)
	want (i32)`,
		},
		{
			name: `if.wast - type-both-value-nums-vs-num`,
			// This should err because the if branch returns two values, but its type use requires one:
			//	(module (func $type-both-value-nums-vs-num (result i32)
			//	  (if (result i32) (i32.const 1) (then (i32.const 1) (i32.const 1)) (else (i32.const 1) (i32.const 1)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32) (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 1, // (then (i32.const 1) (i32.const 1))
					OpcodeElse, OpcodeI32Const, 1, OpcodeI32Const, 1, // (else (i32.const 1) (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `too many results in if block
	have (i32, i32)
	want (i32)`,
		},
		{
			name: `if.wast - type-both-different-value-nums-vs-nums`,
			// This should err because the if branch returns three values, but its type use requires two:
			//	(module (func $type-both-different-value-nums-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1) (then (i32.const 1) (i32.const 1) (i32.const 1)) (else (i32.const 1)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 1, OpcodeI32Const, 1, // (then (i32.const 1) (i32.const 1) (i32.const 1))
					OpcodeElse, OpcodeI32Const, 1, // (else (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `too many results in if block
	have (i32, i32, i32)
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-then-break-last-void-vs-nums`,
			// This should err because the branch in the if returns no values, but its type use requires two:
			//	(module (func $type-then-break-last-void-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1) (then (br 0)) (else (i32.const 1) (i32.const 1)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeBr, 0, // (then (br 0))
					OpcodeElse, OpcodeI32Const, 1, // (else (i32.const 1) (i32.const 1)))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have ()
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-else-break-last-void-vs-nums`,
			// This should err because the branch in the else returns no values, but its type use requires two:
			//	(module (func $type-else-break-last-void-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1) (then (i32.const 1) (i32.const 1)) (else (br 0)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 1, // (then (i32.const 1) (i32.const 1))
					OpcodeElse, OpcodeBr, 0, // (else (br 0))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have ()
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-then-break-empty-vs-nums`,
			// This should err because the branch in the if returns no values, but its type use requires two:
			//	(module (func $type-then-break-empty-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1)
			//	    (then (br 0) (i32.const 1) (i32.const 1))
			//	    (else (i32.const 1) (i32.const 1))
			//	  )
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeBr, 0, OpcodeI32Const, 1, OpcodeI32Const, 1, // (then (br 0) (i32.const 1) (i32.const 1))
					// ^^ NOTE: consts are outside the br block
					OpcodeElse, OpcodeI32Const, 1, OpcodeI32Const, 1, // (else (i32.const 1) (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have ()
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-else-break-empty-vs-nums`,
			// This should err because the branch in the else returns no values, but its type use requires two:
			//	(module (func $type-else-break-empty-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1)
			//	    (then (i32.const 1) (i32.const 1))
			//	    (else (br 0) (i32.const 1) (i32.const 1))
			//	  )
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 1, // (then (i32.const 1) (i32.const 1))
					OpcodeElse, OpcodeBr, 0, OpcodeI32Const, 1, OpcodeI32Const, 1, // (else (br 0) (i32.const 1) (i32.const 1))
					// ^^ NOTE: consts are outside the br block
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have ()
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-then-break-void-vs-nums`,
			// This should err because the branch in the if evaluates to no values, but its type use requires two:
			//	(module (func $type-then-break-void-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1)
			//	    (then (br 0 (nop)) (i32.const 1) (i32.const 1))
			//	    (else (i32.const 1) (i32.const 1))
			//	  )
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeNop, OpcodeBr, 0, OpcodeI32Const, 1, OpcodeI32Const, 1, // (then (br 0 (nop)) (i32.const 1) (i32.const 1))
					// ^^ NOTE: consts are outside the br block
					OpcodeElse, OpcodeI32Const, 1, OpcodeI32Const, 1, // (else (i32.const 1) (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have ()
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-else-break-void-vs-nums`,
			// This should err because the branch in the else evaluates to no values, but its type use requires two:
			//	(module (func $type-else-break-void-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1)
			//	    (then (i32.const 1) (i32.const 1))
			//	    (else (br 0 (nop)) (i32.const 1) (i32.const 1))
			//	  )
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 1, // (then (i32.const 1) (i32.const 1))
					OpcodeElse, OpcodeNop, OpcodeBr, 0, OpcodeI32Const, 1, OpcodeI32Const, 1, // (else (br 0 (nop)) (i32.const 1) (i32.const 1))
					// ^^ NOTE: consts are outside the br block
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have ()
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-then-break-num-vs-nums`,
			// This should err because the branch in the if evaluates to one value, but its type use requires two:
			//	(module (func $type-then-break-num-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1)
			//	    (then (br 0 (i64.const 1)) (i32.const 1) (i32.const 1))
			//	    (else (i32.const 1) (i32.const 1))
			//	  )
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI64Const, 1, OpcodeBr, 0, OpcodeI32Const, 1, OpcodeI32Const, 1, // (then (br 0 (i64.const 1)) (i32.const 1) (i32.const 1))
					// ^^ NOTE: only one (incorrect) const is inside the br block
					OpcodeElse, OpcodeI32Const, 1, OpcodeI32Const, 1, // (else (i32.const 1) (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have (i64)
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-else-break-num-vs-nums`,
			// This should err because the branch in the else evaluates to one value, but its type use requires two:
			//	(module (func $type-else-break-num-vs-nums (result i32 i32)
			//	  (if (result i32 i32) (i32.const 1)
			//	    (then (i32.const 1) (i32.const 1))
			//	    (else (br 0 (i64.const 1)) (i32.const 1) (i32.const 1))
			//	  )
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 1, // (then (i32.const 1) (i32.const 1))
					OpcodeElse, OpcodeI64Const, 1, OpcodeBr, 0, OpcodeI32Const, 1, OpcodeI32Const, 1, // (else (br 0 (i64.const 1)) (i32.const 1) (i32.const 1))
					// ^^ NOTE: only one (incorrect) const is inside the br block
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have (i64)
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-then-break-partial-vs-nums`,
			// This should err because the branch in the if evaluates to one value, but its type use requires two:
			//	(module (func $type-then-break-partial-vs-nums (result i32 i32)
			//	  (i32.const 1)
			//	  (if (result i32 i32) (i32.const 1)
			//	    (then (br 0 (i64.const 1)) (i32.const 1))
			//	    (else (i32.const 1))
			//	  )
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, // (i32.const 1)
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI64Const, 1, OpcodeBr, 0, OpcodeI32Const, 1, // (then (br 0 (i64.const 1)) (i32.const 1))
					// ^^ NOTE: only one (incorrect) const is inside the br block
					OpcodeElse, OpcodeI32Const, 1, // (else (i32.const 1))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in br block
	have (i64)
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-else-break-partial-vs-nums`,
			// This should err because the branch in the if evaluates to one value, but its type use requires two:
			//	(module (func $type-else-break-partial-vs-nums (result i32 i32)
			//	  (i32.const 1)
			//	  (if (result i32 i32) (i32.const 1)
			//	    (then (i32.const 1))
			//	    (else (br 0 (i64.const 1)) (i32.const 1))
			//	  )
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, // (i32.const 1)
					OpcodeI32Const, 1, OpcodeIf, 0x00, // (if (result i32 i32) (i32.const 1)
					OpcodeI32Const, 1, // (then (i32.const 1))
					OpcodeElse, OpcodeI64Const, 1, OpcodeBr, 0, OpcodeI32Const, 1, // (else (br 0 (i64.const 1)) (i32.const 1))
					// ^^ NOTE: only one (incorrect) const is inside the br block
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in if block
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `if.wast - type-param-void-vs-num`,
			// This should err because the stack has no values, but the if type use requires two:
			//	(module (func $type-param-void-vs-num
			//	  (if (param i32) (i32.const 1) (then (drop)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x01, // (if (param i32) (i32.const 1)
					OpcodeDrop, // (then (drop)))
					OpcodeEnd,  // if
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: `not enough params for if block
	have ()
	want (i32)`,
		},
		{
			name: `if.wast - type-param-void-vs-nums`,
			// This should err because the stack has no values, but the if type use requires two:
			//	(module (func $type-param-void-vs-nums
			//	  (if (param i32 f64) (i32.const 1) (then (drop) (drop)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, i32f64_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x01, // (if (param i32 f64) (i32.const 1)
					OpcodeI32Const, 1, OpcodeDrop, // (then (drop) (drop))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough params for if block
	have ()
	want (i32, f64)`,
		},
		{
			name: `if.wast - type-param-num-vs-num`,
			// This should err because the stack has a different value that what the if type use requires:
			//	(module (func $type-param-num-vs-num
			//	  (f32.const 0) (if (param i32) (i32.const 1) (then (drop)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeF32Const, 0, 0, 0, 0, // (f32.const 0)
					OpcodeI32Const, 1, OpcodeIf, 0x01, // (if (param i32) (i32.const 1)
					OpcodeDrop, // (then (drop))
					OpcodeEnd,  // if
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: "cannot use f32 in if block as param[0] type i32",
		},
		{
			name: `if.wast - type-param-num-vs-nums`,
			// This should err because the stack has one value, but the if type use requires two:
			//	(module (func $type-param-num-vs-nums
			//	  (f32.const 0) (if (param f32 i32) (i32.const 1) (then (drop) (drop)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, f32i32_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeF32Const, 0, 0, 0, 0, // (f32.const 0)
					OpcodeI32Const, 1, OpcodeIf, 0x01, // (if (param f32 i32) (i32.const 1)
					OpcodeDrop, OpcodeDrop, // (then (drop) (drop))
					OpcodeEnd, // if
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough params for if block
	have (f32)
	want (f32, i32)`,
		},
		{
			name: `if.wast - type-param-nested-void-vs-num`,
			// This should err because the stack has no values, but the if type use requires one:
			//	(module (func $type-param-nested-void-vs-num
			//	  (block (if (param i32) (i32.const 1) (then (drop))))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x40, // (block
					OpcodeI32Const, 1, OpcodeIf, 0x01, // (if (param i32) (i32.const 1)
					OpcodeDrop, // (then (drop))
					OpcodeEnd,  // block
					OpcodeEnd,  // if
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: `not enough params for if block
	have ()
	want (i32)`,
		},
		{
			name: `if.wast - type-param-void-vs-nums`,
			// This should err because the stack has no values, but the if type use requires two:
			//	(module (func $type-param-void-vs-nums
			//	  (block (if (param i32 f64) (i32.const 1) (then (drop) (drop))))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, i32f64_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x40, // (block
					OpcodeI32Const, 1, OpcodeIf, 0x01, // (if (param i32 f64) (i32.const 1)
					OpcodeDrop, // (then (drop) (drop))
					OpcodeEnd,  // block
					OpcodeEnd,  // if
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: `not enough params for if block
	have ()
	want (i32, f64)`,
		},
		{
			name: `if.wast - type-param-num-vs-num`,
			// This should err because the stack has a different values than required by the if type use:
			//	(module (func $type-param-num-vs-num
			//	  (block (f32.const 0) (if (param i32) (i32.const 1) (then (drop))))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x40, // (block
					OpcodeF32Const, 0, 0, 0, 0, // (f32.const 0)
					OpcodeI32Const, 1, OpcodeIf, 0x01, // (if (param i32) (i32.const 1)
					OpcodeDrop, // (then (drop))
					OpcodeEnd,  // block
					OpcodeEnd,  // if
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: "cannot use f32 in if block as param[0] type i32",
		},
		{
			name: `if.wast - type-param-num-vs-nums`,
			// This should err because the stack has one value, but the if type use requires two:
			//	(module (func $type-param-num-vs-nums
			//	  (block (f32.const 0) (if (param f32 i32) (i32.const 1) (then (drop) (drop))))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, f32i32_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x40, // (block
					OpcodeF32Const, 0, 0, 0, 0, // (f32.const 0)
					OpcodeI32Const, 1, OpcodeIf, 0x01, // (if (param f32 i32) (i32.const 1)
					OpcodeDrop, // (then (drop) (drop))
					OpcodeEnd,  // block
					OpcodeEnd,  // if
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: `not enough params for if block
	have (f32)
	want (f32, i32)`,
		},

		// test/core/loop.wast
		{
			name: `loop.wast - wrong signature for loop type use`,
			// This should err because the loop type use returns no values, but its block returns one:
			//  (module
			//    (type $sig (func))
			//    (func (loop (type $sig) (i32.const 0)))
			//  )
			module: &Module{
				TypeSection:     []*FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeLoop, 0, OpcodeI32Const, 0, // (loop (type $sig) (i32.const 0))
					OpcodeEnd, // loop
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `too many results in loop block
	have (i32)
	want ()`,
		},
		{
			name: `loop.wast - type-value-nums-vs-void`,
			// This should err because the empty block type requires no values, but the loop returns two:
			//  (module (func $type-value-nums-vs-void
			//    (loop (i32.const 1) (i32.const 2))
			//  ))
			module: &Module{
				TypeSection:     []*FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeLoop, 0x40, OpcodeI32Const, 1, OpcodeI32Const, 2, // (loop (i32.const 1) (i32.const 2))
					OpcodeEnd, // loop
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `too many results in loop block
	have (i32, i32)
	want ()`,
		},
		{
			name: `loop.wast - type-value-empty-vs-nums`,
			// This should err because the loop type use returns two values, but the block returns none:
			//	(module (func $type-value-empty-vs-nums (result i32 i32)
			//	  (loop (result i32 i32))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeLoop, 0x0, // (loop (result i32 i32)) - matches existing func type
					OpcodeEnd, // loop
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in loop block
	have ()
	want (i32, i32)`,
		},
		{
			name: `loop.wast - type-value-void-vs-nums`,
			// This should err because the loop type use returns two values, but the block returns none:
			//	(module (func $type-value-void-vs-nums (result i32 i32)
			//	  (loop (result i32 i32) (nop))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeLoop, 0x0, // (loop (result i32 i32) - matches existing func type
					OpcodeNop, // (nop)
					OpcodeEnd, // loop
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in loop block
	have ()
	want (i32, i32)`,
		},
		{
			name: `loop.wast - type-value-num-vs-nums`,
			// This should err because the loop type use returns two values, but the block returns one:
			//	(module (func $type-value-num-vs-nums (result i32 i32)
			//	  (loop (result i32 i32) (i32.const 0))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeLoop, 0x0, // (loop (result i32 i32) - matches existing func type
					OpcodeI32Const, 0, // (i32.const 0)
					OpcodeEnd, // loop
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in loop block
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `loop.wast - type-value-partial-vs-nums`,
			// This should err because the loop type use returns two values, but the block returns one:
			//	(module (func $type-value-partial-vs-nums (result i32 i32)
			//	  (i32.const 1) (loop (result i32 i32) (i32.const 2))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, // (i32.const 1) - NOTE: outside the loop!
					OpcodeLoop, 0x0, // (loop (result i32 i32) - matches existing func type
					OpcodeI32Const, 2, // (i32.const 2)
					OpcodeEnd, // loop
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough results in loop block
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `loop.wast - type-value-nums-vs-num`,
			// This should err because the loop type use returns one value, but the block returns two:
			//	(module (func $type-value-nums-vs-num (result i32)
			//	  (loop (result i32) (i32.const 1) (i32.const 2))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeLoop, 0x0, // (loop (result i32) - matches existing func type
					OpcodeI32Const, 1, OpcodeI32Const, 2, // (i32.const 1) (i32.const 2))
					OpcodeEnd, // loop
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `too many results in loop block
	have (i32, i32)
	want (i32)`,
		},
		{
			name: `loop.wast - type-param-void-vs-num`,
			// This should err because the loop type use requires one param, but the stack has none:
			//	(module (func $type-param-void-vs-num
			//	  (loop (param i32) (drop))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeLoop, 0x1, // (loop (param i32)
					OpcodeDrop, // (drop)
					OpcodeEnd,  // loop
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: `not enough params for loop block
	have ()
	want (i32)`,
		},
		{
			name: `loop.wast - type-param-void-vs-nums`,
			// This should err because the loop type use requires two params, but the stack has none:
			//	(module (func $type-param-void-vs-nums
			//	  (loop (param i32 f64) (drop) (drop))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, i32f64_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeLoop, 0x1, // (loop (param i32 f64)
					OpcodeDrop, // (drop)
					OpcodeDrop, // (drop)
					OpcodeEnd,  // loop
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: `not enough params for loop block
	have ()
	want (i32, f64)`,
		},
		{
			name: `loop.wast - type-param-num-vs-num`,
			// This should err because the loop type use requires a different param type than what's on the stack:
			//	(module (func $type-param-num-vs-num
			//	  (f32.const 0) (loop (param i32) (drop))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeF32Const, 0, 0, 0, 0, // (f32.const 0)
					OpcodeLoop, 0x1, // (loop (param i32)
					OpcodeDrop, // (drop)
					OpcodeEnd,  // loop
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: "cannot use f32 in loop block as param[0] type i32",
		},
		{
			name: `loop.wast - type-param-num-vs-num`,
			// This should err because the loop type use requires a more parameters than what's on the stack:
			//	(module (func $type-param-num-vs-nums
			//	  (f32.const 0) (loop (param f32 i32) (drop) (drop))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, f32i32_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeF32Const, 0, 0, 0, 0, // (f32.const 0)
					OpcodeLoop, 0x1, // (loop (param f32 i32)
					OpcodeDrop, OpcodeDrop, // (drop) (drop)
					OpcodeEnd, // loop
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough params for loop block
	have (f32)
	want (f32, i32)`,
		},
		{
			name: `loop.wast - type-param-nested-void-vs-num`,
			// This should err because the loop type use requires a more parameters than what's on the stack:
			//	(module (func $type-param-nested-void-vs-num
			//	  (block (loop (param i32) (drop)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x40, // (block
					OpcodeLoop, 0x1, // (loop (param i32)
					OpcodeDrop, // (drop)
					OpcodeEnd,  // loop
					OpcodeEnd,  // block
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: `not enough params for loop block
	have ()
	want (i32)`,
		},
		{
			name: `loop.wast - type-param-void-vs-nums`,
			// This should err because the loop type use requires a more parameters than what's on the stack:
			//	(module (func $type-param-void-vs-nums
			//	  (block (loop (param i32 f64) (drop) (drop)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, i32f64_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x40, // (block
					OpcodeLoop, 0x1, // (loop (param i32 f64)
					OpcodeDrop, OpcodeDrop, // (drop) (drop)
					OpcodeEnd, // loop
					OpcodeEnd, // block
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough params for loop block
	have ()
	want (i32, f64)`,
		},
		{
			name: `loop.wast - type-param-void-vs-nums`,
			// This should err because the loop type use requires a different param type than what's on the stack:
			//	(module (func $type-param-num-vs-num
			//	  (block (f32.const 0) (loop (param i32) (drop)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x40, // (block
					OpcodeF32Const, 0, 0, 0, 0, // (f32.const 0)
					OpcodeLoop, 0x1, // (loop (param i32)
					OpcodeDrop, // (drop)
					OpcodeEnd,  // loop
					OpcodeEnd,  // block
					OpcodeEnd,  // func
				}}},
			},
			expectedErr: "cannot use f32 in loop block as param[0] type i32",
		},
		{
			name: `loop.wast - type-param-void-vs-nums`,
			// This should err because the loop type use requires a more parameters than what's on the stack:
			//	(module (func $type-param-num-vs-nums
			//	  (block (f32.const 0) (loop (param f32 i32) (drop) (drop)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v, f32i32_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x40, // (block
					OpcodeF32Const, 0, 0, 0, 0, // (f32.const 0)
					OpcodeLoop, 0x1, // (loop (param f32 i32)
					OpcodeDrop, OpcodeDrop, // (drop) (drop)
					OpcodeEnd, // loop
					OpcodeEnd, // block
					OpcodeEnd, // func
				}}},
			},
			expectedErr: `not enough params for loop block
	have (f32)
	want (f32, i32)`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			err := tc.module.validateFunction(FeatureMultiValue, 0, []Index{0}, nil, nil, nil)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestModule_funcValidation_CallIndirect(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		m := &Module{
			TypeSection:     []*FunctionType{v_v},
			FunctionSection: []Index{0},
			CodeSection: []*Code{{Body: []byte{
				OpcodeI32Const, 1,
				OpcodeCallIndirect, 0, 0,
				OpcodeEnd,
			}}},
		}
		err := m.validateFunction(FeatureReferenceTypes, 0, []Index{0}, nil, &Memory{}, []*Table{{Type: RefTypeFuncref}})
		require.NoError(t, err)
	})
	t.Run("non zero table index", func(t *testing.T) {
		m := &Module{
			TypeSection:     []*FunctionType{v_v},
			FunctionSection: []Index{0},
			CodeSection: []*Code{{Body: []byte{
				OpcodeI32Const, 1,
				OpcodeCallIndirect, 0, 100,
				OpcodeEnd,
			}}},
		}
		t.Run("disabled", func(t *testing.T) {
			err := m.validateFunction(Features20191205, 0, []Index{0}, nil, &Memory{}, []*Table{{}, {}})
			require.EqualError(t, err, "table index must be zero but was 100: feature \"reference-types\" is disabled")
		})
		t.Run("enabled but out of range", func(t *testing.T) {
			err := m.validateFunction(FeatureReferenceTypes, 0, []Index{0}, nil, &Memory{}, []*Table{{}, {}})
			require.EqualError(t, err, "unknown table index: 100")
		})
	})
	t.Run("non funcref table", func(t *testing.T) {
		m := &Module{
			TypeSection:     []*FunctionType{v_v},
			FunctionSection: []Index{0},
			CodeSection: []*Code{{Body: []byte{
				OpcodeI32Const, 1,
				OpcodeCallIndirect, 0, 0,
				OpcodeEnd,
			}}},
		}
		err := m.validateFunction(FeatureReferenceTypes, 0, []Index{0}, nil, &Memory{}, []*Table{{Type: RefTypeExternref}})
		require.EqualError(t, err, "table is not funcref type but was externref for call_indirect")
	})
}
