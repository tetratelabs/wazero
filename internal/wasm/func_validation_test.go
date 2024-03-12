package wasm

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestModule_ValidateFunction_validateFunctionWithMaxStackValues(t *testing.T) {
	const max = 100
	const valuesNum = max + 1

	// Compile a function which has max+1 const instructions.
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
		TypeSection:     []FunctionType{v_v},
		FunctionSection: []Index{0},
		CodeSection:     []Code{{Body: body}},
	}

	t.Run("not exceed", func(t *testing.T) {
		err := m.validateFunctionWithMaxStackValues(&stacks{}, api.CoreFeaturesV1,
			0, []Index{0}, nil, nil, nil, max+1, nil, bytes.NewReader(nil))
		require.NoError(t, err)
	})
	t.Run("exceed", func(t *testing.T) {
		err := m.validateFunctionWithMaxStackValues(&stacks{}, api.CoreFeaturesV1,
			0, []Index{0}, nil, nil, nil, max, nil, bytes.NewReader(nil))
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
					TypeSection:     []FunctionType{v_v},
					FunctionSection: []Index{0},
					CodeSection:     []Code{{Body: []byte{tc.input}}},
				}
				err := m.validateFunction(&stacks{}, api.CoreFeaturesV1,
					0, []Index{0}, nil, nil, nil, nil,
					bytes.NewReader(nil))
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
					TypeSection:     []FunctionType{v_v},
					FunctionSection: []Index{0},
					CodeSection:     []Code{{Body: body}},
				}
				err := m.validateFunction(&stacks{}, api.CoreFeatureSignExtensionOps,
					0, []Index{0}, nil, nil, nil,
					nil, bytes.NewReader(nil))
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
					TypeSection:     []FunctionType{v_v},
					FunctionSection: []Index{0},
					CodeSection:     []Code{{Body: []byte{OpcodeMiscPrefix, tc.input}}},
				}
				err := m.validateFunction(&stacks{}, api.CoreFeaturesV1,
					0, []Index{0}, nil, nil, nil, nil, bytes.NewReader(nil))
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
					TypeSection:     []FunctionType{v_v},
					FunctionSection: []Index{0},
					CodeSection:     []Code{{Body: body}},
				}
				err := m.validateFunction(&stacks{}, api.CoreFeatureNonTrappingFloatToIntConversion,
					0, []Index{0}, nil, nil, nil, nil, bytes.NewReader(nil))
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
				TypeSection:     []FunctionType{v_f64f64},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{i32_i32}, // (func (param i32) (result i32)
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection: []FunctionType{
					i32_i32,    // (func (param i32) (result i32)
					i32i32_i32, // (if (param i32 i32) (result i32)
				},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				err := tc.module.validateFunction(&stacks{}, api.CoreFeaturesV1,
					0, []Index{0}, nil, nil, nil, nil, bytes.NewReader(nil))
				require.EqualError(t, err, tc.expectedErrOnDisable)
			})
			t.Run("enabled", func(t *testing.T) {
				err := tc.module.validateFunction(&stacks{}, api.CoreFeatureMultiValue,
					0, []Index{0}, nil, nil, nil, nil, bytes.NewReader(nil))
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
					TypeSection:      []FunctionType{v_v},
					FunctionSection:  []Index{0},
					CodeSection:      []Code{{Body: body}},
					DataSection:      []DataSegment{{}},
					ElementSection:   []ElementSegment{{}},
					DataCountSection: &c,
				}
				err := m.validateFunction(&stacks{}, api.CoreFeatureBulkMemoryOperations,
					0, []Index{0}, nil, &Memory{}, []Table{{}, {}}, nil, bytes.NewReader(nil))
				require.NoError(t, err)
			})
		}
	})
	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			body                []byte
			dataSection         []DataSegment
			elementSection      []ElementSegment
			dataCountSectionNil bool
			memory              *Memory
			tables              []Table
			flag                api.CoreFeatures
			expectedErr         string
		}{
			// memory.init
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      nil,
				expectedErr: "memory must exist for memory.init",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit},
				flag:        api.CoreFeaturesV1,
				expectedErr: `memory.init invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:                []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit},
				flag:                api.CoreFeatureBulkMemoryOperations,
				dataCountSectionNil: true,
				expectedErr:         `memory must exist for memory.init`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "failed to read data segment index for memory.init: EOF",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit, 100 /* data section out of range */},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []DataSegment{{}},
				expectedErr: "index 100 out of range of data section(len=1)",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []DataSegment{{}},
				expectedErr: "failed to read memory index for memory.init: EOF",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit, 0, 1},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []DataSegment{{}},
				expectedErr: "memory.init reserved byte must be zero encoded with 1 byte",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryInit, 0, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []DataSegment{{}},
				expectedErr: "cannot pop the operand for memory.init: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscMemoryInit, 0, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []DataSegment{{}},
				expectedErr: "cannot pop the operand for memory.init: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscMemoryInit, 0, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []DataSegment{{}},
				expectedErr: "cannot pop the operand for memory.init: i32 missing",
			},
			// data.drop
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscDataDrop},
				flag:        api.CoreFeaturesV1,
				expectedErr: `data.drop invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:                []byte{OpcodeMiscPrefix, OpcodeMiscDataDrop},
				dataCountSectionNil: true,
				memory:              &Memory{},
				flag:                api.CoreFeatureBulkMemoryOperations,
				expectedErr:         `data.drop requires data count section`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscDataDrop},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "failed to read data segment index for data.drop: EOF",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscDataDrop, 100 /* data section out of range */},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				dataSection: []DataSegment{{}},
				expectedErr: "index 100 out of range of data section(len=1)",
			},
			// memory.copy
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryCopy},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      nil,
				expectedErr: "memory must exist for memory.copy",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryCopy},
				flag:        api.CoreFeaturesV1,
				expectedErr: `memory.copy invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryCopy},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: `failed to read memory index for memory.copy: EOF`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryCopy, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "failed to read memory index for memory.copy: EOF",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryCopy, 0, 1},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "memory.copy reserved byte must be zero encoded with 1 byte",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryCopy, 0, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "cannot pop the operand for memory.copy: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscMemoryCopy, 0, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "cannot pop the operand for memory.copy: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscMemoryCopy, 0, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "cannot pop the operand for memory.copy: i32 missing",
			},
			// memory.fill
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryFill},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      nil,
				expectedErr: "memory must exist for memory.fill",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryFill},
				flag:        api.CoreFeaturesV1,
				expectedErr: `memory.fill invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryFill},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: `failed to read memory index for memory.fill: EOF`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryFill, 1},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: `memory.fill reserved byte must be zero encoded with 1 byte`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscMemoryFill, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "cannot pop the operand for memory.fill: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscMemoryFill, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "cannot pop the operand for memory.fill: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscMemoryFill, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				memory:      &Memory{},
				expectedErr: "cannot pop the operand for memory.fill: i32 missing",
			},
			// table.init
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableInit},
				flag:        api.CoreFeaturesV1,
				tables:      []Table{{}},
				expectedErr: `table.init invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableInit},
				flag:        api.CoreFeatureBulkMemoryOperations,
				tables:      []Table{{}},
				expectedErr: "failed to read element segment index for table.init: EOF",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscTableInit, 100 /* data section out of range */},
				flag:           api.CoreFeatureBulkMemoryOperations,
				tables:         []Table{{}},
				elementSection: []ElementSegment{{}},
				expectedErr:    "index 100 out of range of element section(len=1)",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscTableInit, 0},
				flag:           api.CoreFeatureBulkMemoryOperations,
				tables:         []Table{{}},
				elementSection: []ElementSegment{{}},
				expectedErr:    "failed to read source table index for table.init: EOF",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscTableInit, 0, 10},
				flag:           api.CoreFeatureBulkMemoryOperations,
				tables:         []Table{{}},
				elementSection: []ElementSegment{{}},
				expectedErr:    "source table index must be zero for table.init as feature \"reference-types\" is disabled",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscTableInit, 0, 10},
				flag:           api.CoreFeatureBulkMemoryOperations | api.CoreFeatureReferenceTypes,
				tables:         []Table{{}},
				elementSection: []ElementSegment{{}},
				expectedErr:    "table of index 10 not found",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscTableInit, 0, 1},
				flag:           api.CoreFeatureBulkMemoryOperations | api.CoreFeatureReferenceTypes,
				tables:         []Table{{}, {Type: RefTypeExternref}},
				elementSection: []ElementSegment{{Type: RefTypeFuncref}},
				expectedErr:    "type mismatch for table.init: element type funcref does not match table type externref",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscTableInit, 0, 0},
				flag:           api.CoreFeatureBulkMemoryOperations,
				tables:         []Table{{}},
				elementSection: []ElementSegment{{}},
				expectedErr:    "cannot pop the operand for table.init: i32 missing",
			},
			{
				body:           []byte{OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscTableInit, 0, 0},
				flag:           api.CoreFeatureBulkMemoryOperations,
				tables:         []Table{{}},
				elementSection: []ElementSegment{{}},
				expectedErr:    "cannot pop the operand for table.init: i32 missing",
			},
			{
				body:           []byte{OpcodeI32Const, 0, OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscTableInit, 0, 0},
				flag:           api.CoreFeatureBulkMemoryOperations,
				tables:         []Table{{}},
				elementSection: []ElementSegment{{}},
				expectedErr:    "cannot pop the operand for table.init: i32 missing",
			},
			// elem.drop
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscElemDrop},
				flag:        api.CoreFeaturesV1,
				tables:      []Table{{}},
				expectedErr: `elem.drop invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscElemDrop},
				flag:        api.CoreFeatureBulkMemoryOperations,
				tables:      []Table{{}},
				expectedErr: "failed to read element segment index for elem.drop: EOF",
			},
			{
				body:           []byte{OpcodeMiscPrefix, OpcodeMiscElemDrop, 100 /* element section out of range */},
				flag:           api.CoreFeatureBulkMemoryOperations,
				tables:         []Table{{}},
				elementSection: []ElementSegment{{}},
				expectedErr:    "index 100 out of range of element section(len=1)",
			},
			// table.copy
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy},
				flag:        api.CoreFeaturesV1,
				tables:      []Table{{}},
				expectedErr: `table.copy invalid as feature "bulk-memory-operations" is disabled`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy},
				flag:        api.CoreFeatureBulkMemoryOperations,
				tables:      []Table{{}},
				expectedErr: `failed to read destination table index for table.copy: EOF`,
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy, 10},
				flag:        api.CoreFeatureBulkMemoryOperations,
				tables:      []Table{{}},
				expectedErr: "destination table index must be zero for table.copy as feature \"reference-types\" is disabled",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy, 3},
				flag:        api.CoreFeatureBulkMemoryOperations | api.CoreFeatureReferenceTypes,
				tables:      []Table{{}, {}},
				expectedErr: "table of index 3 not found",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy, 3},
				flag:        api.CoreFeatureBulkMemoryOperations | api.CoreFeatureReferenceTypes,
				tables:      []Table{{}, {}, {}, {}},
				expectedErr: "failed to read source table index for table.copy: EOF",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy, 0, 3},
				flag:        api.CoreFeatureBulkMemoryOperations, // Multiple tables require api.CoreFeatureReferenceTypes.
				tables:      []Table{{}, {}, {}, {}},
				expectedErr: "source table index must be zero for table.copy as feature \"reference-types\" is disabled",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy, 3, 1},
				flag:        api.CoreFeatureBulkMemoryOperations | api.CoreFeatureReferenceTypes,
				tables:      []Table{{}, {Type: RefTypeFuncref}, {}, {Type: RefTypeExternref}},
				expectedErr: "table type mismatch for table.copy: funcref (src) != externref (dst)",
			},
			{
				body:        []byte{OpcodeMiscPrefix, OpcodeMiscTableCopy, 0, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				tables:      []Table{{}},
				expectedErr: "cannot pop the operand for table.copy: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscTableCopy, 0, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				tables:      []Table{{}},
				expectedErr: "cannot pop the operand for table.copy: i32 missing",
			},
			{
				body:        []byte{OpcodeI32Const, 0, OpcodeI32Const, 0, OpcodeMiscPrefix, OpcodeMiscTableCopy, 0, 0},
				flag:        api.CoreFeatureBulkMemoryOperations,
				tables:      []Table{{}},
				expectedErr: "cannot pop the operand for table.copy: i32 missing",
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.expectedErr, func(t *testing.T) {
				m := &Module{
					TypeSection:     []FunctionType{v_v},
					FunctionSection: []Index{0},
					CodeSection:     []Code{{Body: tc.body}},
					ElementSection:  tc.elementSection,
					DataSection:     tc.dataSection,
				}
				if !tc.dataCountSectionNil {
					c := uint32(0)
					m.DataCountSection = &c
				}
				err := m.validateFunction(&stacks{}, tc.flag, 0, []Index{0}, nil, tc.memory, tc.tables, nil, bytes.NewReader(nil))
				require.EqualError(t, err, tc.expectedErr)
			})
		}
	})
}

var (
	f32, f64, i32, i64, v128, externref = ValueTypeF32, ValueTypeF64, ValueTypeI32, ValueTypeI64, ValueTypeV128, ValueTypeExternref
	f32i32_v                            = initFt([]ValueType{f32, i32}, nil)
	f64f32_i64                          = initFt([]ValueType{f64, f32}, []ValueType{i64})
	f64i32_v128i64                      = initFt([]ValueType{f64, i32}, []ValueType{v128, i64})
	i32_i32                             = initFt([]ValueType{i32}, []ValueType{i32})
	i32f64_v                            = initFt([]ValueType{i32, f64}, nil)
	i32i32_i32                          = initFt([]ValueType{i32, i32}, []ValueType{i32})
	i32_v                               = initFt([]ValueType{i32}, nil)
	v_v                                 = FunctionType{}
	v_f32                               = initFt(nil, []ValueType{f32})
	v_f32f32                            = initFt(nil, []ValueType{f32, f32})
	v_f64i32                            = initFt(nil, []ValueType{f64, i32})
	v_f64f64                            = initFt(nil, []ValueType{f64, f64})
	v_i32                               = initFt(nil, []ValueType{i32})
	v_i32i32                            = initFt(nil, []ValueType{i32, i32})
	v_i32i64                            = initFt(nil, []ValueType{i32, i64})
	v_i64i64                            = initFt(nil, []ValueType{i64, i64})
)

func initFt(params, results []ValueType) FunctionType {
	ft := FunctionType{Params: params, Results: results}
	ft.CacheNumInUint64()
	return ft
}

// TestModule_ValidateFunction_MultiValue_TypeMismatch are "type mismatch" tests when "multi-value" was merged.
//
// See https://github.com/WebAssembly/spec/commit/484180ba3d9d7638ba1cb400b699ffede796927c
func TestModule_ValidateFunction_MultiValue_TypeMismatch(t *testing.T) {
	tests := []struct {
		name            string
		module          *Module
		expectedErr     string
		enabledFeatures api.CoreFeatures
	}{
		// test/core/func.wast

		{
			name: `func.wast - type-empty-f64-i32`,
			module: &Module{
				TypeSection:     []FunctionType{v_f64i32},
				FunctionSection: []Index{0},
				CodeSection:     []Code{{Body: []byte{OpcodeEnd}}},
			},
			expectedErr: `not enough results
	have ()
	want (f64, i32)`,
		},
		{
			name: `func.wast - type-value-void-vs-nums`,
			module: &Module{
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection:     []Code{{Body: []byte{OpcodeNop, OpcodeEnd}}},
			},
			expectedErr: `not enough results
	have ()
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-value-nums-vs-void`,
			module: &Module{
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection:     []Code{{Body: []byte{OpcodeI32Const, 0, OpcodeI64Const, 0, OpcodeEnd}}},
			},
			expectedErr: `too many results
	have (i32, i64)
	want ()`,
		},
		{
			name: `func.wast - type-value-num-vs-nums - v_f32f32 -> f32`,
			module: &Module{
				TypeSection:     []FunctionType{v_f32f32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_f32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_f32f32},
				FunctionSection: []Index{0},
				CodeSection:     []Code{{Body: []byte{OpcodeReturn, OpcodeEnd}}},
			},
			expectedErr: `not enough results
	have ()
	want (f32, f32)`,
		},
		{
			name: `func.wast - type-return-last-void-vs-nums`,
			module: &Module{
				TypeSection:     []FunctionType{v_i32i64},
				FunctionSection: []Index{0},
				CodeSection:     []Code{{Body: []byte{OpcodeNop, OpcodeReturn, OpcodeEnd}}}, // (return (nop))
			},
			expectedErr: `not enough results
	have ()
	want (i32, i64)`,
		},
		{
			name: `func.wast - type-return-last-num-vs-nums`,
			module: &Module{
				TypeSection:     []FunctionType{v_i64i64},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, i32f64_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, f32i32_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, i32f64_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, f32i32_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_i32},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, i32f64_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, f32i32_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, i32f64_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, i32_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
				TypeSection:     []FunctionType{v_v, f32i32_v},
				FunctionSection: []Index{0},
				CodeSection: []Code{{Body: []byte{
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
			err := tc.module.validateFunction(&stacks{}, api.CoreFeatureMultiValue,
				0, []Index{0}, nil, nil, nil, nil, bytes.NewReader(nil))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestModule_funcValidation_CallIndirect(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		m := &Module{
			TypeSection:     []FunctionType{v_v},
			FunctionSection: []Index{0},
			CodeSection: []Code{{Body: []byte{
				OpcodeI32Const, 1,
				OpcodeCallIndirect, 0, 0,
				OpcodeEnd,
			}}},
		}
		err := m.validateFunction(&stacks{}, api.CoreFeatureReferenceTypes,
			0, []Index{0}, nil, &Memory{}, []Table{{Type: RefTypeFuncref}}, nil, bytes.NewReader(nil))
		require.NoError(t, err)
	})
	t.Run("non zero table index", func(t *testing.T) {
		m := &Module{
			TypeSection:     []FunctionType{v_v},
			FunctionSection: []Index{0},
			CodeSection: []Code{{Body: []byte{
				OpcodeI32Const, 1,
				OpcodeCallIndirect, 0, 100,
				OpcodeEnd,
			}}},
		}
		t.Run("disabled", func(t *testing.T) {
			err := m.validateFunction(&stacks{}, api.CoreFeaturesV1,
				0, []Index{0}, nil, &Memory{}, []Table{{}, {}}, nil, bytes.NewReader(nil))
			require.EqualError(t, err, "table index must be zero but was 100: feature \"reference-types\" is disabled")
		})
		t.Run("enabled but out of range", func(t *testing.T) {
			err := m.validateFunction(&stacks{}, api.CoreFeatureReferenceTypes,
				0, []Index{0}, nil, &Memory{}, []Table{{}, {}}, nil, bytes.NewReader(nil))
			require.EqualError(t, err, "unknown table index: 100")
		})
	})
	t.Run("non funcref table", func(t *testing.T) {
		m := &Module{
			TypeSection:     []FunctionType{v_v},
			FunctionSection: []Index{0},
			CodeSection: []Code{{Body: []byte{
				OpcodeI32Const, 1,
				OpcodeCallIndirect, 0, 0,
				OpcodeEnd,
			}}},
		}
		err := m.validateFunction(&stacks{}, api.CoreFeatureReferenceTypes,
			0, []Index{0}, nil, &Memory{}, []Table{{Type: RefTypeExternref}}, nil, bytes.NewReader(nil))
		require.EqualError(t, err, "table is not funcref type but was externref for call_indirect")
	})
}

func TestModule_funcValidation_RefTypes(t *testing.T) {
	tests := []struct {
		name                    string
		body                    []byte
		flag                    api.CoreFeatures
		declaredFunctionIndexes map[Index]struct{}
		expectedErr             string
	}{
		{
			name: "ref.null (funcref)",
			flag: api.CoreFeatureReferenceTypes,
			body: []byte{
				OpcodeRefNull, ValueTypeFuncref,
				OpcodeDrop, OpcodeEnd,
			},
		},
		{
			name: "ref.null (externref)",
			flag: api.CoreFeatureReferenceTypes,
			body: []byte{
				OpcodeRefNull, ValueTypeExternref,
				OpcodeDrop, OpcodeEnd,
			},
		},
		{
			name: "ref.null - disabled",
			flag: api.CoreFeaturesV1,
			body: []byte{
				OpcodeRefNull, ValueTypeFuncref,
				OpcodeDrop, OpcodeEnd,
			},
			expectedErr: "ref.null invalid as feature \"reference-types\" is disabled",
		},
		{
			name: "ref.is_null",
			flag: api.CoreFeatureReferenceTypes,
			body: []byte{
				OpcodeRefNull, ValueTypeFuncref,
				OpcodeRefIsNull,
				OpcodeDrop, OpcodeEnd,
			},
		},
		{
			name: "ref.is_null - disabled",
			flag: api.CoreFeaturesV1,
			body: []byte{
				OpcodeRefIsNull,
				OpcodeDrop, OpcodeEnd,
			},
			expectedErr: `ref.is_null invalid as feature "reference-types" is disabled`,
		},
		{
			name:                    "ref.func",
			flag:                    api.CoreFeatureReferenceTypes,
			declaredFunctionIndexes: map[uint32]struct{}{0: {}},
			body: []byte{
				OpcodeRefFunc, 0,
				OpcodeDrop, OpcodeEnd,
			},
		},
		{
			name:                    "ref.func - undeclared function index",
			flag:                    api.CoreFeatureReferenceTypes,
			declaredFunctionIndexes: map[uint32]struct{}{0: {}},
			body: []byte{
				OpcodeRefFunc, 100,
				OpcodeDrop, OpcodeEnd,
			},
			expectedErr: `undeclared function index 100 for ref.func`,
		},
		{
			name:                    "ref.func",
			flag:                    api.CoreFeaturesV1,
			declaredFunctionIndexes: map[uint32]struct{}{0: {}},
			body: []byte{
				OpcodeRefFunc, 0,
				OpcodeDrop, OpcodeEnd,
			},
			expectedErr: "ref.func invalid as feature \"reference-types\" is disabled",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			m := &Module{
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection:     []Code{{Body: tc.body}},
			}
			err := m.validateFunction(&stacks{}, tc.flag,
				0, []Index{0}, nil, nil, nil, tc.declaredFunctionIndexes, bytes.NewReader(nil))
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestModule_funcValidation_TableGrowSizeFill(t *testing.T) {
	tables := []Table{{Type: RefTypeFuncref}, {Type: RefTypeExternref}}
	tests := []struct {
		name        string
		body        []byte
		flag        api.CoreFeatures
		expectedErr string
	}{
		{
			name: "table.grow (funcref)",
			body: []byte{
				OpcodeRefNull, RefTypeFuncref,
				OpcodeI32Const, 1, // number of elements
				OpcodeMiscPrefix, OpcodeMiscTableGrow,
				0, // Table Index.
				OpcodeDrop,
				OpcodeEnd,
			},
			flag: api.CoreFeatureReferenceTypes,
		},
		{
			name: "table.grow (funcref) - type mismatch",
			body: []byte{
				OpcodeRefNull, RefTypeFuncref,
				OpcodeI32Const, 1, // number of elements
				OpcodeMiscPrefix, OpcodeMiscTableGrow,
				1, // Table of externref type -> mismatch.
				OpcodeEnd,
			},
			flag:        api.CoreFeatureReferenceTypes,
			expectedErr: `cannot pop the operand for table.grow: type mismatch: expected externref, but was funcref`,
		},
		{
			name: "table.grow (externref)",
			body: []byte{
				OpcodeRefNull, RefTypeExternref,
				OpcodeI32Const, 1, // number of elements
				OpcodeMiscPrefix, OpcodeMiscTableGrow,
				1, // Table Index.
				OpcodeDrop,
				OpcodeEnd,
			},
			flag: api.CoreFeatureReferenceTypes,
		},
		{
			name: "table.grow (externref) type mismatch",
			body: []byte{
				OpcodeRefNull, RefTypeExternref,
				OpcodeI32Const, 1, // number of elements
				OpcodeMiscPrefix, OpcodeMiscTableGrow,
				0, // Table of funcref type -> mismatch.
				OpcodeEnd,
			},
			flag:        api.CoreFeatureReferenceTypes,
			expectedErr: `cannot pop the operand for table.grow: type mismatch: expected funcref, but was externref`,
		},
		{
			name: "table.grow - table not found",
			body: []byte{
				OpcodeRefNull, RefTypeFuncref,
				OpcodeI32Const, 1, // number of elements
				OpcodeMiscPrefix, OpcodeMiscTableGrow,
				10, // Table Index.
				OpcodeEnd,
			},
			flag:        api.CoreFeatureReferenceTypes,
			expectedErr: `table of index 10 not found`,
		},
		{
			name: "table.size - table not found",
			body: []byte{
				OpcodeMiscPrefix, OpcodeMiscTableSize,
				10, // Table Index.
				OpcodeEnd,
			},
			flag:        api.CoreFeatureReferenceTypes,
			expectedErr: `table of index 10 not found`,
		},
		{
			name: "table.size",
			body: []byte{
				OpcodeMiscPrefix, OpcodeMiscTableSize,
				1, // Table Index.
				OpcodeDrop,
				OpcodeEnd,
			},
			flag: api.CoreFeatureReferenceTypes,
		},
		{
			name: "table.fill (funcref)",
			body: []byte{
				OpcodeI32Const, 1, // offset
				OpcodeRefNull, RefTypeFuncref,
				OpcodeI32Const, 1, // number of elements
				OpcodeMiscPrefix, OpcodeMiscTableFill,
				0, // Table Index.
				OpcodeEnd,
			},
			flag: api.CoreFeatureReferenceTypes,
		},
		{
			name: "table.fill (funcref) - type mismatch",
			body: []byte{
				OpcodeI32Const, 1, // offset
				OpcodeRefNull, RefTypeFuncref,
				OpcodeI32Const, 1, // number of elements
				OpcodeMiscPrefix, OpcodeMiscTableFill,
				1, // Table of externref type -> mismatch.
				OpcodeEnd,
			},
			flag:        api.CoreFeatureReferenceTypes,
			expectedErr: `cannot pop the operand for table.fill: type mismatch: expected externref, but was funcref`,
		},
		{
			name: "table.fill (externref)",
			body: []byte{
				OpcodeI32Const, 1, // offset
				OpcodeRefNull, RefTypeExternref,
				OpcodeI32Const, 1, // number of elements
				OpcodeMiscPrefix, OpcodeMiscTableFill,
				1, // Table Index.
				OpcodeEnd,
			},
			flag: api.CoreFeatureReferenceTypes,
		},
		{
			name: "table.fill (externref) - type mismatch",
			body: []byte{
				OpcodeI32Const, 1, // offset
				OpcodeRefNull, RefTypeExternref,
				OpcodeI32Const, 1, // number of elements
				OpcodeMiscPrefix, OpcodeMiscTableFill,
				0, // Table of funcref type -> mismatch.
				OpcodeEnd,
			},
			flag:        api.CoreFeatureReferenceTypes,
			expectedErr: `cannot pop the operand for table.fill: type mismatch: expected funcref, but was externref`,
		},
		{
			name: "table.fill - table not found",
			body: []byte{
				OpcodeMiscPrefix, OpcodeMiscTableFill,
				10, // Table Index.
				OpcodeEnd,
			},
			flag:        api.CoreFeatureReferenceTypes,
			expectedErr: `table of index 10 not found`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			m := &Module{
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection:     []Code{{Body: tc.body}},
			}
			err := m.validateFunction(&stacks{}, tc.flag,
				0, []Index{0}, nil, nil, tables, nil, bytes.NewReader(nil))
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestModule_funcValidation_TableGetSet(t *testing.T) {
	tables := []Table{{Type: RefTypeFuncref}, {Type: RefTypeExternref}}
	tests := []struct {
		name        string
		body        []byte
		flag        api.CoreFeatures
		expectedErr string
	}{
		{
			name: "table.get (funcref)",
			body: []byte{
				OpcodeI32Const, 0,
				OpcodeTableGet, 0,
				OpcodeRefIsNull,
				OpcodeDrop,
				OpcodeEnd,
			},
			flag: api.CoreFeatureReferenceTypes,
		},
		{
			name: "table.get (externref)",
			body: []byte{
				OpcodeI32Const, 0,
				OpcodeTableGet, 1,
				OpcodeRefIsNull,
				OpcodeDrop,
				OpcodeEnd,
			},
			flag: api.CoreFeatureReferenceTypes,
		},
		{
			name: "table.get (disabled)",
			body: []byte{
				OpcodeI32Const, 0,
				OpcodeTableGet, 0,
				OpcodeDrop,
				OpcodeEnd,
			},
			flag:        api.CoreFeaturesV1,
			expectedErr: `table.get is invalid as feature "reference-types" is disabled`,
		},
		{
			name: "table.set (funcref)",
			body: []byte{
				OpcodeI32Const, 0,
				OpcodeRefNull, ValueTypeFuncref,
				OpcodeTableSet, 0,
				OpcodeEnd,
			},
			flag: api.CoreFeatureReferenceTypes,
		},
		{
			name: "table.set type mismatch (src=funcref, dst=externref)",
			body: []byte{
				OpcodeI32Const, 0,
				OpcodeRefNull, ValueTypeFuncref,
				OpcodeTableSet, 1,
				OpcodeEnd,
			},
			flag:        api.CoreFeatureReferenceTypes,
			expectedErr: `cannot pop the operand for table.set: type mismatch: expected externref, but was funcref`,
		},
		{
			name: "table.set (externref)",
			body: []byte{
				OpcodeI32Const, 0,
				OpcodeRefNull, ValueTypeExternref,
				OpcodeTableSet, 1,
				OpcodeEnd,
			},
			flag: api.CoreFeatureReferenceTypes,
		},
		{
			name: "table.set type mismatch (src=externref, dst=funcref)",
			body: []byte{
				OpcodeI32Const, 0,
				OpcodeRefNull, ValueTypeExternref,
				OpcodeTableSet, 0,
				OpcodeEnd,
			},
			flag:        api.CoreFeatureReferenceTypes,
			expectedErr: `cannot pop the operand for table.set: type mismatch: expected funcref, but was externref`,
		},
		{
			name: "table.set (disabled)",
			body: []byte{
				OpcodeTableSet, 1,
				OpcodeEnd,
			},
			flag:        api.CoreFeaturesV1,
			expectedErr: `table.set is invalid as feature "reference-types" is disabled`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			m := &Module{
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection:     []Code{{Body: tc.body}},
			}
			err := m.validateFunction(&stacks{}, tc.flag,
				0, []Index{0}, nil, nil, tables, nil, bytes.NewReader(nil))
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestModule_funcValidation_Select_error(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		flag        api.CoreFeatures
		expectedErr string
	}{
		{
			name: "typed_select (disabled)",
			body: []byte{
				OpcodeI32Const, 0, OpcodeI32Const, 0, OpcodeI32Const, 0,
				OpcodeTypedSelect, 1, ValueTypeI32, // immediate vector's size must be one
				OpcodeDrop,
				OpcodeEnd,
			},
			flag:        api.CoreFeaturesV1,
			expectedErr: "typed_select is invalid as feature \"reference-types\" is disabled",
		},
		{
			name: "typed_select (too many immediate types)",
			body: []byte{
				OpcodeI32Const, 0, OpcodeI32Const, 0, OpcodeI32Const, 0,
				OpcodeTypedSelect, 2, // immediate vector's size must be one
			},
			flag:        api.CoreFeatureReferenceTypes,
			expectedErr: `too many type immediates for typed_select`,
		},
		{
			name: "typed_select (immediate type not found)",
			body: []byte{
				OpcodeI32Const, 0, OpcodeI32Const, 0, OpcodeI32Const, 0,
				OpcodeTypedSelect, 1, 0,
				OpcodeEnd,
			},
			flag:        api.CoreFeatureReferenceTypes,
			expectedErr: `invalid type unknown for typed_select`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			m := &Module{
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection:     []Code{{Body: tc.body}},
			}
			err := m.validateFunction(&stacks{}, tc.flag,
				0, []Index{0}, nil, nil, nil, nil, bytes.NewReader(nil))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestModule_funcValidation_SIMD(t *testing.T) {
	addV128Const := func(in []byte) []byte {
		return append(in, OpcodeVecPrefix,
			OpcodeVecV128Const,
			1, 1, 1, 1, 1, 1, 1, 1,
			1, 1, 1, 1, 1, 1, 1, 1)
	}
	vv2v := func(vec OpcodeVec) (ret []byte) {
		ret = addV128Const(ret)
		ret = addV128Const(ret)
		return append(ret,
			OpcodeVecPrefix,
			vec,
			OpcodeDrop,
			OpcodeEnd,
		)
	}
	vvv2v := func(vec OpcodeVec) (ret []byte) {
		ret = addV128Const(ret)
		ret = addV128Const(ret)
		ret = addV128Const(ret)
		return append(ret,
			OpcodeVecPrefix,
			vec,
			OpcodeDrop,
			OpcodeEnd,
		)
	}

	v2v := func(vec OpcodeVec) (ret []byte) {
		ret = addV128Const(ret)
		return append(ret,
			OpcodeVecPrefix,
			vec,
			OpcodeDrop,
			OpcodeEnd,
		)
	}

	vi2v := func(vec OpcodeVec) (ret []byte) {
		ret = addV128Const(ret)
		return append(ret,
			OpcodeI32Const, 1,
			OpcodeVecPrefix,
			vec,
			OpcodeDrop,
			OpcodeEnd,
		)
	}

	load := func(vec OpcodeVec, offset, align uint32) (ret []byte) {
		ret = []byte{
			OpcodeI32Const, 1,
			OpcodeVecPrefix,
			vec,
		}

		ret = append(ret, leb128.EncodeUint32(align)...)
		ret = append(ret, leb128.EncodeUint32(offset)...)
		ret = append(ret,
			OpcodeDrop,
			OpcodeEnd,
		)
		return
	}

	loadLane := func(vec OpcodeVec, offset, align uint32, lane byte) (ret []byte) {
		ret = addV128Const([]byte{OpcodeI32Const, 1})
		ret = append(ret,
			OpcodeVecPrefix,
			vec,
		)

		ret = append(ret, leb128.EncodeUint32(align)...)
		ret = append(ret, leb128.EncodeUint32(offset)...)
		ret = append(ret,
			lane,
			OpcodeDrop,
			OpcodeEnd,
		)
		return
	}

	storeLane := func(vec OpcodeVec, offset, align uint32, lane byte) (ret []byte) {
		ret = addV128Const([]byte{OpcodeI32Const, 1})
		ret = append(ret,
			OpcodeVecPrefix,
			vec,
		)
		ret = append(ret, leb128.EncodeUint32(align)...)
		ret = append(ret, leb128.EncodeUint32(offset)...)
		ret = append(ret,
			lane,
			OpcodeEnd,
		)
		return
	}

	extractLane := func(vec OpcodeVec, lane byte) (ret []byte) {
		ret = addV128Const(ret)
		ret = append(ret,
			OpcodeVecPrefix,
			vec,
			lane,
			OpcodeDrop,
			OpcodeEnd,
		)
		return
	}

	replaceLane := func(vec OpcodeVec, lane byte) (ret []byte) {
		ret = addV128Const(ret)

		switch vec {
		case OpcodeVecI8x16ReplaceLane, OpcodeVecI16x8ReplaceLane, OpcodeVecI32x4ReplaceLane:
			ret = append(ret, OpcodeI32Const, 0)
		case OpcodeVecI64x2ReplaceLane:
			ret = append(ret, OpcodeI64Const, 0)
		case OpcodeVecF32x4ReplaceLane:
			ret = append(ret, OpcodeF32Const, 0, 0, 0, 0)
		case OpcodeVecF64x2ReplaceLane:
			ret = append(ret, OpcodeF64Const, 0, 0, 0, 0, 0, 0, 0, 0)
		}

		ret = append(ret,
			OpcodeVecPrefix,
			vec,
			lane,
			OpcodeDrop,
			OpcodeEnd,
		)
		return
	}

	splat := func(vec OpcodeVec) (ret []byte) {
		switch vec {
		case OpcodeVecI8x16Splat, OpcodeVecI16x8Splat, OpcodeVecI32x4Splat:
			ret = append(ret, OpcodeI32Const, 0, 0, 0, 0)
		case OpcodeVecI64x2Splat:
			ret = append(ret, OpcodeI64Const, 0, 0, 0, 0, 0, 0, 0, 0)
		case OpcodeVecF32x4Splat:
			ret = append(ret, OpcodeF32Const, 0, 0, 0, 0)
		case OpcodeVecF64x2Splat:
			ret = append(ret, OpcodeF64Const, 0, 0, 0, 0, 0, 0, 0, 0)
		}

		ret = append(ret,
			OpcodeVecPrefix,
			vec,
			OpcodeDrop,
			OpcodeEnd,
		)
		return
	}

	tests := []struct {
		name        string
		body        []byte
		expectedErr string
	}{
		{
			name: "v128.const",
			body: []byte{
				OpcodeVecPrefix,
				OpcodeVecV128Const,
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
				OpcodeDrop,
				OpcodeEnd,
			},
		},
		{name: OpcodeVecI8x16AddName, body: vv2v(OpcodeVecI8x16Add)},
		{name: OpcodeVecI16x8AddName, body: vv2v(OpcodeVecI16x8Add)},
		{name: OpcodeVecI32x4AddName, body: vv2v(OpcodeVecI32x4Add)},
		{name: OpcodeVecI64x2AddName, body: vv2v(OpcodeVecI64x2Add)},
		{name: OpcodeVecI8x16SubName, body: vv2v(OpcodeVecI8x16Sub)},
		{name: OpcodeVecI16x8SubName, body: vv2v(OpcodeVecI16x8Sub)},
		{name: OpcodeVecI32x4SubName, body: vv2v(OpcodeVecI32x4Sub)},
		{name: OpcodeVecI64x2SubName, body: vv2v(OpcodeVecI64x2Sub)},
		{name: OpcodeVecV128AnyTrueName, body: v2v(OpcodeVecV128AnyTrue)},
		{name: OpcodeVecI8x16AllTrueName, body: v2v(OpcodeVecI8x16AllTrue)},
		{name: OpcodeVecI16x8AllTrueName, body: v2v(OpcodeVecI16x8AllTrue)},
		{name: OpcodeVecI32x4AllTrueName, body: v2v(OpcodeVecI32x4AllTrue)},
		{name: OpcodeVecI64x2AllTrueName, body: v2v(OpcodeVecI64x2AllTrue)},
		{name: OpcodeVecI8x16BitMaskName, body: v2v(OpcodeVecI8x16BitMask)},
		{name: OpcodeVecI16x8BitMaskName, body: v2v(OpcodeVecI16x8BitMask)},
		{name: OpcodeVecI32x4BitMaskName, body: v2v(OpcodeVecI32x4BitMask)},
		{name: OpcodeVecI64x2BitMaskName, body: v2v(OpcodeVecI64x2BitMask)},
		{name: OpcodeVecV128LoadName, body: load(OpcodeVecV128Load, 0, 0)},
		{name: OpcodeVecV128LoadName + "/align=4", body: load(OpcodeVecV128Load, 0, 4)},
		{name: OpcodeVecV128Load8x8SName, body: load(OpcodeVecV128Load8x8s, 1, 0)},
		{name: OpcodeVecV128Load8x8SName + "/align=1", body: load(OpcodeVecV128Load8x8s, 0, 1)},
		{name: OpcodeVecV128Load8x8UName, body: load(OpcodeVecV128Load8x8u, 0, 0)},
		{name: OpcodeVecV128Load8x8UName + "/align=1", body: load(OpcodeVecV128Load8x8u, 0, 1)},
		{name: OpcodeVecV128Load16x4SName, body: load(OpcodeVecV128Load16x4s, 1, 0)},
		{name: OpcodeVecV128Load16x4SName + "/align=2", body: load(OpcodeVecV128Load16x4s, 0, 2)},
		{name: OpcodeVecV128Load16x4UName, body: load(OpcodeVecV128Load16x4u, 0, 0)},
		{name: OpcodeVecV128Load16x4UName + "/align=2", body: load(OpcodeVecV128Load16x4u, 0, 2)},
		{name: OpcodeVecV128Load32x2SName, body: load(OpcodeVecV128Load32x2s, 1, 0)},
		{name: OpcodeVecV128Load32x2SName + "/align=3", body: load(OpcodeVecV128Load32x2s, 0, 3)},
		{name: OpcodeVecV128Load32x2UName, body: load(OpcodeVecV128Load32x2u, 0, 0)},
		{name: OpcodeVecV128Load32x2UName + "/align=3", body: load(OpcodeVecV128Load32x2u, 0, 3)},
		{name: OpcodeVecV128Load8SplatName, body: load(OpcodeVecV128Load8Splat, 2, 0)},
		{name: OpcodeVecV128Load16SplatName, body: load(OpcodeVecV128Load16Splat, 0, 1)},
		{name: OpcodeVecV128Load32SplatName, body: load(OpcodeVecV128Load32Splat, 3, 2)},
		{name: OpcodeVecV128Load64SplatName, body: load(OpcodeVecV128Load64Splat, 0, 3)},
		{name: OpcodeVecV128Load32zeroName, body: load(OpcodeVecV128Load32zero, 0, 2)},
		{name: OpcodeVecV128Load64zeroName, body: load(OpcodeVecV128Load64zero, 5, 3)},
		{name: OpcodeVecV128Load8LaneName, body: loadLane(OpcodeVecV128Load8Lane, 5, 0, 10)},
		{name: OpcodeVecV128Load16LaneName, body: loadLane(OpcodeVecV128Load16Lane, 100, 1, 7)},
		{name: OpcodeVecV128Load32LaneName, body: loadLane(OpcodeVecV128Load32Lane, 0, 2, 3)},
		{name: OpcodeVecV128Load64LaneName, body: loadLane(OpcodeVecV128Load64Lane, 0, 3, 1)},
		{
			name: OpcodeVecV128StoreName, body: []byte{
				OpcodeI32Const,
				1, 1, 1, 1,
				OpcodeVecPrefix,
				OpcodeVecV128Const,
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
				OpcodeVecPrefix,
				OpcodeVecV128Store,
				4,  // alignment
				10, // offset
				OpcodeEnd,
			},
		},
		{name: OpcodeVecV128Store8LaneName, body: storeLane(OpcodeVecV128Store8Lane, 0, 0, 0)},
		{name: OpcodeVecV128Store8LaneName + "/lane=15", body: storeLane(OpcodeVecV128Store8Lane, 100, 0, 15)},
		{name: OpcodeVecV128Store16LaneName, body: storeLane(OpcodeVecV128Store16Lane, 0, 0, 0)},
		{name: OpcodeVecV128Store16LaneName + "/lane=7/align=1", body: storeLane(OpcodeVecV128Store16Lane, 100, 1, 7)},
		{name: OpcodeVecV128Store32LaneName, body: storeLane(OpcodeVecV128Store32Lane, 0, 0, 0)},
		{name: OpcodeVecV128Store32LaneName + "/lane=3/align=2", body: storeLane(OpcodeVecV128Store32Lane, 100, 2, 3)},
		{name: OpcodeVecV128Store64LaneName, body: storeLane(OpcodeVecV128Store64Lane, 0, 0, 0)},
		{name: OpcodeVecV128Store64LaneName + "/lane=1/align=3", body: storeLane(OpcodeVecV128Store64Lane, 50, 3, 1)},
		{name: OpcodeVecI8x16ExtractLaneSName, body: extractLane(OpcodeVecI8x16ExtractLaneS, 0)},
		{name: OpcodeVecI8x16ExtractLaneSName + "/lane=15", body: extractLane(OpcodeVecI8x16ExtractLaneS, 15)},
		{name: OpcodeVecI8x16ExtractLaneUName, body: extractLane(OpcodeVecI8x16ExtractLaneU, 0)},
		{name: OpcodeVecI8x16ExtractLaneUName + "/lane=15", body: extractLane(OpcodeVecI8x16ExtractLaneU, 15)},
		{name: OpcodeVecI16x8ExtractLaneSName, body: extractLane(OpcodeVecI16x8ExtractLaneS, 0)},
		{name: OpcodeVecI16x8ExtractLaneSName + "/lane=7", body: extractLane(OpcodeVecI16x8ExtractLaneS, 7)},
		{name: OpcodeVecI16x8ExtractLaneUName, body: extractLane(OpcodeVecI16x8ExtractLaneU, 0)},
		{name: OpcodeVecI16x8ExtractLaneUName + "/lane=8", body: extractLane(OpcodeVecI16x8ExtractLaneU, 7)},
		{name: OpcodeVecI32x4ExtractLaneName, body: extractLane(OpcodeVecI32x4ExtractLane, 0)},
		{name: OpcodeVecI32x4ExtractLaneName + "/lane=3", body: extractLane(OpcodeVecI32x4ExtractLane, 3)},
		{name: OpcodeVecI64x2ExtractLaneName, body: extractLane(OpcodeVecI64x2ExtractLane, 0)},
		{name: OpcodeVecI64x2ExtractLaneName + "/lane=1", body: extractLane(OpcodeVecI64x2ExtractLane, 1)},
		{name: OpcodeVecF32x4ExtractLaneName, body: extractLane(OpcodeVecF32x4ExtractLane, 0)},
		{name: OpcodeVecF32x4ExtractLaneName + "/lane=3", body: extractLane(OpcodeVecF32x4ExtractLane, 3)},
		{name: OpcodeVecF64x2ExtractLaneName, body: extractLane(OpcodeVecF64x2ExtractLane, 0)},
		{name: OpcodeVecF64x2ExtractLaneName + "/lane=1", body: extractLane(OpcodeVecF64x2ExtractLane, 1)},
		{name: OpcodeVecI8x16ReplaceLaneName, body: replaceLane(OpcodeVecI8x16ReplaceLane, 0)},
		{name: OpcodeVecI8x16ReplaceLaneName + "/lane=15", body: replaceLane(OpcodeVecI8x16ReplaceLane, 15)},
		{name: OpcodeVecI16x8ReplaceLaneName, body: replaceLane(OpcodeVecI16x8ReplaceLane, 0)},
		{name: OpcodeVecI16x8ReplaceLaneName + "/lane=7", body: replaceLane(OpcodeVecI16x8ReplaceLane, 7)},
		{name: OpcodeVecI32x4ReplaceLaneName, body: replaceLane(OpcodeVecI32x4ReplaceLane, 0)},
		{name: OpcodeVecI32x4ReplaceLaneName + "/lane=3", body: replaceLane(OpcodeVecI32x4ReplaceLane, 3)},
		{name: OpcodeVecI64x2ReplaceLaneName, body: replaceLane(OpcodeVecI64x2ReplaceLane, 0)},
		{name: OpcodeVecI64x2ReplaceLaneName + "/lane=1", body: replaceLane(OpcodeVecI64x2ReplaceLane, 1)},
		{name: OpcodeVecF32x4ReplaceLaneName, body: replaceLane(OpcodeVecF32x4ReplaceLane, 0)},
		{name: OpcodeVecF32x4ReplaceLaneName + "/lane=3", body: replaceLane(OpcodeVecF32x4ReplaceLane, 3)},
		{name: OpcodeVecF64x2ReplaceLaneName, body: replaceLane(OpcodeVecF64x2ReplaceLane, 0)},
		{name: OpcodeVecF64x2ReplaceLaneName + "/lane=1", body: replaceLane(OpcodeVecF64x2ReplaceLane, 1)},
		{name: OpcodeVecI8x16SplatName, body: splat(OpcodeVecI8x16Splat)},
		{name: OpcodeVecI16x8SplatName, body: splat(OpcodeVecI16x8Splat)},
		{name: OpcodeVecI32x4SplatName, body: splat(OpcodeVecI32x4Splat)},
		{name: OpcodeVecI64x2SplatName, body: splat(OpcodeVecI64x2Splat)},
		{name: OpcodeVecF32x4SplatName, body: splat(OpcodeVecF32x4Splat)},
		{name: OpcodeVecF64x2SplatName, body: splat(OpcodeVecF64x2Splat)},
		{name: OpcodeVecI8x16SwizzleName, body: vv2v(OpcodeVecI8x16Swizzle)},
		{
			name: OpcodeVecV128i8x16ShuffleName, body: []byte{
				OpcodeVecPrefix,
				OpcodeVecV128Const,
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
				OpcodeVecPrefix,
				OpcodeVecV128Const,
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
				OpcodeVecPrefix,
				OpcodeVecV128i8x16Shuffle,
				1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
				OpcodeDrop,
				OpcodeEnd,
			},
		},
		{name: OpcodeVecV128NotName, body: v2v(OpcodeVecV128Not)},
		{name: OpcodeVecV128AndName, body: vv2v(OpcodeVecV128And)},
		{name: OpcodeVecV128AndNotName, body: vv2v(OpcodeVecV128AndNot)},
		{name: OpcodeVecV128OrName, body: vv2v(OpcodeVecV128Or)},
		{name: OpcodeVecV128XorName, body: vv2v(OpcodeVecV128Xor)},
		{name: OpcodeVecV128BitselectName, body: vvv2v(OpcodeVecV128Bitselect)},
		{name: OpcodeVecI8x16ShlName, body: vi2v(OpcodeVecI8x16Shl)},
		{name: OpcodeVecI8x16ShrSName, body: vi2v(OpcodeVecI8x16ShrS)},
		{name: OpcodeVecI8x16ShrUName, body: vi2v(OpcodeVecI8x16ShrU)},
		{name: OpcodeVecI16x8ShlName, body: vi2v(OpcodeVecI16x8Shl)},
		{name: OpcodeVecI16x8ShrSName, body: vi2v(OpcodeVecI16x8ShrS)},
		{name: OpcodeVecI16x8ShrUName, body: vi2v(OpcodeVecI16x8ShrU)},
		{name: OpcodeVecI32x4ShlName, body: vi2v(OpcodeVecI32x4Shl)},
		{name: OpcodeVecI32x4ShrSName, body: vi2v(OpcodeVecI32x4ShrS)},
		{name: OpcodeVecI32x4ShrUName, body: vi2v(OpcodeVecI32x4ShrU)},
		{name: OpcodeVecI64x2ShlName, body: vi2v(OpcodeVecI64x2Shl)},
		{name: OpcodeVecI64x2ShrSName, body: vi2v(OpcodeVecI64x2ShrS)},
		{name: OpcodeVecI64x2ShrUName, body: vi2v(OpcodeVecI64x2ShrU)},
		{name: OpcodeVecI8x16EqName, body: vv2v(OpcodeVecI8x16Eq)},
		{name: OpcodeVecI8x16NeName, body: vv2v(OpcodeVecI8x16Ne)},
		{name: OpcodeVecI8x16LtSName, body: vv2v(OpcodeVecI8x16LtS)},
		{name: OpcodeVecI8x16LtUName, body: vv2v(OpcodeVecI8x16LtU)},
		{name: OpcodeVecI8x16GtSName, body: vv2v(OpcodeVecI8x16GtS)},
		{name: OpcodeVecI8x16GtUName, body: vv2v(OpcodeVecI8x16GtU)},
		{name: OpcodeVecI8x16LeSName, body: vv2v(OpcodeVecI8x16LeS)},
		{name: OpcodeVecI8x16LeUName, body: vv2v(OpcodeVecI8x16LeU)},
		{name: OpcodeVecI8x16GeSName, body: vv2v(OpcodeVecI8x16GeS)},
		{name: OpcodeVecI8x16GeUName, body: vv2v(OpcodeVecI8x16GeU)},
		{name: OpcodeVecI16x8EqName, body: vv2v(OpcodeVecI16x8Eq)},
		{name: OpcodeVecI16x8NeName, body: vv2v(OpcodeVecI16x8Ne)},
		{name: OpcodeVecI16x8LtSName, body: vv2v(OpcodeVecI16x8LtS)},
		{name: OpcodeVecI16x8LtUName, body: vv2v(OpcodeVecI16x8LtU)},
		{name: OpcodeVecI16x8GtSName, body: vv2v(OpcodeVecI16x8GtS)},
		{name: OpcodeVecI16x8GtUName, body: vv2v(OpcodeVecI16x8GtU)},
		{name: OpcodeVecI16x8LeSName, body: vv2v(OpcodeVecI16x8LeS)},
		{name: OpcodeVecI16x8LeUName, body: vv2v(OpcodeVecI16x8LeU)},
		{name: OpcodeVecI16x8GeSName, body: vv2v(OpcodeVecI16x8GeS)},
		{name: OpcodeVecI16x8GeUName, body: vv2v(OpcodeVecI16x8GeU)},
		{name: OpcodeVecI32x4EqName, body: vv2v(OpcodeVecI32x4Eq)},
		{name: OpcodeVecI32x4NeName, body: vv2v(OpcodeVecI32x4Ne)},
		{name: OpcodeVecI32x4LtSName, body: vv2v(OpcodeVecI32x4LtS)},
		{name: OpcodeVecI32x4LtUName, body: vv2v(OpcodeVecI32x4LtU)},
		{name: OpcodeVecI32x4GtSName, body: vv2v(OpcodeVecI32x4GtS)},
		{name: OpcodeVecI32x4GtUName, body: vv2v(OpcodeVecI32x4GtU)},
		{name: OpcodeVecI32x4LeSName, body: vv2v(OpcodeVecI32x4LeS)},
		{name: OpcodeVecI32x4LeUName, body: vv2v(OpcodeVecI32x4LeU)},
		{name: OpcodeVecI32x4GeSName, body: vv2v(OpcodeVecI32x4GeS)},
		{name: OpcodeVecI32x4GeUName, body: vv2v(OpcodeVecI32x4GeU)},
		{name: OpcodeVecI64x2EqName, body: vv2v(OpcodeVecI64x2Eq)},
		{name: OpcodeVecI64x2NeName, body: vv2v(OpcodeVecI64x2Ne)},
		{name: OpcodeVecI64x2LtSName, body: vv2v(OpcodeVecI64x2LtS)},
		{name: OpcodeVecI64x2GtSName, body: vv2v(OpcodeVecI64x2GtS)},
		{name: OpcodeVecI64x2LeSName, body: vv2v(OpcodeVecI64x2LeS)},
		{name: OpcodeVecI64x2GeSName, body: vv2v(OpcodeVecI64x2GeS)},
		{name: OpcodeVecF32x4EqName, body: vv2v(OpcodeVecF32x4Eq)},
		{name: OpcodeVecF32x4NeName, body: vv2v(OpcodeVecF32x4Ne)},
		{name: OpcodeVecF32x4LtName, body: vv2v(OpcodeVecF32x4Lt)},
		{name: OpcodeVecF32x4GtName, body: vv2v(OpcodeVecF32x4Gt)},
		{name: OpcodeVecF32x4LeName, body: vv2v(OpcodeVecF32x4Le)},
		{name: OpcodeVecF32x4GeName, body: vv2v(OpcodeVecF32x4Ge)},
		{name: OpcodeVecF64x2EqName, body: vv2v(OpcodeVecF64x2Eq)},
		{name: OpcodeVecF64x2NeName, body: vv2v(OpcodeVecF64x2Ne)},
		{name: OpcodeVecF64x2LtName, body: vv2v(OpcodeVecF64x2Lt)},
		{name: OpcodeVecF64x2GtName, body: vv2v(OpcodeVecF64x2Gt)},
		{name: OpcodeVecF64x2LeName, body: vv2v(OpcodeVecF64x2Le)},
		{name: OpcodeVecF64x2GeName, body: vv2v(OpcodeVecF64x2Ge)},
		{name: OpcodeVecI8x16AddName, body: vv2v(OpcodeVecI8x16Add)},
		{name: OpcodeVecI8x16AddSatSName, body: vv2v(OpcodeVecI8x16AddSatS)},
		{name: OpcodeVecI8x16AddSatUName, body: vv2v(OpcodeVecI8x16AddSatU)},
		{name: OpcodeVecI8x16SubName, body: vv2v(OpcodeVecI8x16Sub)},
		{name: OpcodeVecI8x16SubSatSName, body: vv2v(OpcodeVecI8x16SubSatS)},
		{name: OpcodeVecI8x16SubSatUName, body: vv2v(OpcodeVecI8x16SubSatU)},
		{name: OpcodeVecI16x8AddName, body: vv2v(OpcodeVecI16x8Add)},
		{name: OpcodeVecI16x8AddSatSName, body: vv2v(OpcodeVecI16x8AddSatS)},
		{name: OpcodeVecI16x8AddSatUName, body: vv2v(OpcodeVecI16x8AddSatU)},
		{name: OpcodeVecI16x8SubName, body: vv2v(OpcodeVecI16x8Sub)},
		{name: OpcodeVecI16x8SubSatSName, body: vv2v(OpcodeVecI16x8SubSatS)},
		{name: OpcodeVecI16x8SubSatUName, body: vv2v(OpcodeVecI16x8SubSatU)},
		{name: OpcodeVecI16x8MulName, body: vv2v(OpcodeVecI16x8Mul)},
		{name: OpcodeVecI32x4AddName, body: vv2v(OpcodeVecI32x4Add)},
		{name: OpcodeVecI32x4SubName, body: vv2v(OpcodeVecI32x4Sub)},
		{name: OpcodeVecI32x4MulName, body: vv2v(OpcodeVecI32x4Mul)},
		{name: OpcodeVecI64x2AddName, body: vv2v(OpcodeVecI64x2Add)},
		{name: OpcodeVecI64x2SubName, body: vv2v(OpcodeVecI64x2Sub)},
		{name: OpcodeVecI64x2MulName, body: vv2v(OpcodeVecI64x2Mul)},
		{name: OpcodeVecF32x4AddName, body: vv2v(OpcodeVecF32x4Add)},
		{name: OpcodeVecF32x4SubName, body: vv2v(OpcodeVecF32x4Sub)},
		{name: OpcodeVecF32x4MulName, body: vv2v(OpcodeVecF32x4Mul)},
		{name: OpcodeVecF32x4DivName, body: vv2v(OpcodeVecF32x4Div)},
		{name: OpcodeVecF64x2AddName, body: vv2v(OpcodeVecF64x2Add)},
		{name: OpcodeVecF64x2SubName, body: vv2v(OpcodeVecF64x2Sub)},
		{name: OpcodeVecF64x2MulName, body: vv2v(OpcodeVecF64x2Mul)},
		{name: OpcodeVecF64x2DivName, body: vv2v(OpcodeVecF64x2Div)},
		{name: OpcodeVecI8x16NegName, body: v2v(OpcodeVecI8x16Neg)},
		{name: OpcodeVecI16x8NegName, body: v2v(OpcodeVecI16x8Neg)},
		{name: OpcodeVecI32x4NegName, body: v2v(OpcodeVecI32x4Neg)},
		{name: OpcodeVecI64x2NegName, body: v2v(OpcodeVecI64x2Neg)},
		{name: OpcodeVecF32x4NegName, body: v2v(OpcodeVecF32x4Neg)},
		{name: OpcodeVecF64x2NegName, body: v2v(OpcodeVecF64x2Neg)},
		{name: OpcodeVecF32x4SqrtName, body: v2v(OpcodeVecF32x4Sqrt)},
		{name: OpcodeVecF64x2SqrtName, body: v2v(OpcodeVecF64x2Sqrt)},
		{name: OpcodeVecI8x16MinSName, body: vv2v(OpcodeVecI8x16MinS)},
		{name: OpcodeVecI8x16MinUName, body: vv2v(OpcodeVecI8x16MinU)},
		{name: OpcodeVecI8x16MaxSName, body: vv2v(OpcodeVecI8x16MaxS)},
		{name: OpcodeVecI8x16MaxUName, body: vv2v(OpcodeVecI8x16MaxU)},
		{name: OpcodeVecI8x16AvgrUName, body: vv2v(OpcodeVecI8x16AvgrU)},
		{name: OpcodeVecI8x16AbsName, body: v2v(OpcodeVecI8x16Abs)},
		{name: OpcodeVecI8x16PopcntName, body: v2v(OpcodeVecI8x16Popcnt)},
		{name: OpcodeVecI16x8MinSName, body: vv2v(OpcodeVecI16x8MinS)},
		{name: OpcodeVecI16x8MinUName, body: vv2v(OpcodeVecI16x8MinU)},
		{name: OpcodeVecI16x8MaxSName, body: vv2v(OpcodeVecI16x8MaxS)},
		{name: OpcodeVecI16x8MaxUName, body: vv2v(OpcodeVecI16x8MaxU)},
		{name: OpcodeVecI16x8AvgrUName, body: vv2v(OpcodeVecI16x8AvgrU)},
		{name: OpcodeVecI16x8AbsName, body: v2v(OpcodeVecI16x8Abs)},
		{name: OpcodeVecI32x4MinSName, body: vv2v(OpcodeVecI32x4MinS)},
		{name: OpcodeVecI32x4MinUName, body: vv2v(OpcodeVecI32x4MinU)},
		{name: OpcodeVecI32x4MaxSName, body: vv2v(OpcodeVecI32x4MaxS)},
		{name: OpcodeVecI32x4MaxUName, body: vv2v(OpcodeVecI32x4MaxU)},
		{name: OpcodeVecI32x4AbsName, body: v2v(OpcodeVecI32x4Abs)},
		{name: OpcodeVecI64x2AbsName, body: v2v(OpcodeVecI64x2Abs)},
		{name: OpcodeVecF32x4AbsName, body: v2v(OpcodeVecF32x4Abs)},
		{name: OpcodeVecF64x2AbsName, body: v2v(OpcodeVecF64x2Abs)},
		{name: OpcodeVecF32x4MinName, body: vv2v(OpcodeVecF32x4Min)},
		{name: OpcodeVecF32x4MaxName, body: vv2v(OpcodeVecF32x4Max)},
		{name: OpcodeVecF64x2MinName, body: vv2v(OpcodeVecF64x2Min)},
		{name: OpcodeVecF64x2MaxName, body: vv2v(OpcodeVecF64x2Max)},
		{name: OpcodeVecF32x4CeilName, body: v2v(OpcodeVecF32x4Ceil)},
		{name: OpcodeVecF32x4FloorName, body: v2v(OpcodeVecF32x4Floor)},
		{name: OpcodeVecF32x4TruncName, body: v2v(OpcodeVecF32x4Trunc)},
		{name: OpcodeVecF32x4NearestName, body: v2v(OpcodeVecF32x4Nearest)},
		{name: OpcodeVecF64x2CeilName, body: v2v(OpcodeVecF64x2Ceil)},
		{name: OpcodeVecF64x2FloorName, body: v2v(OpcodeVecF64x2Floor)},
		{name: OpcodeVecF64x2TruncName, body: v2v(OpcodeVecF64x2Trunc)},
		{name: OpcodeVecF64x2NearestName, body: v2v(OpcodeVecF64x2Nearest)},
		{name: OpcodeVecF32x4MinName, body: vv2v(OpcodeVecF32x4Pmin)},
		{name: OpcodeVecF32x4MaxName, body: vv2v(OpcodeVecF32x4Pmax)},
		{name: OpcodeVecF64x2MinName, body: vv2v(OpcodeVecF64x2Pmin)},
		{name: OpcodeVecF64x2MaxName, body: vv2v(OpcodeVecF64x2Pmax)},
		{name: OpcodeVecI16x8ExtendLowI8x16SName, body: v2v(OpcodeVecI16x8ExtendLowI8x16S)},
		{name: OpcodeVecI16x8ExtendHighI8x16SName, body: v2v(OpcodeVecI16x8ExtendHighI8x16S)},
		{name: OpcodeVecI16x8ExtendLowI8x16UName, body: v2v(OpcodeVecI16x8ExtendLowI8x16U)},
		{name: OpcodeVecI16x8ExtendHighI8x16UName, body: v2v(OpcodeVecI16x8ExtendHighI8x16U)},
		{name: OpcodeVecI32x4ExtendLowI16x8SName, body: v2v(OpcodeVecI32x4ExtendLowI16x8S)},
		{name: OpcodeVecI32x4ExtendHighI16x8SName, body: v2v(OpcodeVecI32x4ExtendHighI16x8S)},
		{name: OpcodeVecI32x4ExtendLowI16x8UName, body: v2v(OpcodeVecI32x4ExtendLowI16x8U)},
		{name: OpcodeVecI32x4ExtendHighI16x8UName, body: v2v(OpcodeVecI32x4ExtendHighI16x8U)},
		{name: OpcodeVecI64x2ExtendLowI32x4SName, body: v2v(OpcodeVecI64x2ExtendLowI32x4S)},
		{name: OpcodeVecI64x2ExtendHighI32x4SName, body: v2v(OpcodeVecI64x2ExtendHighI32x4S)},
		{name: OpcodeVecI64x2ExtendLowI32x4UName, body: v2v(OpcodeVecI64x2ExtendLowI32x4U)},
		{name: OpcodeVecI64x2ExtendHighI32x4UName, body: v2v(OpcodeVecI64x2ExtendHighI32x4U)},
		{name: OpcodeVecI16x8Q15mulrSatSName, body: vv2v(OpcodeVecI16x8Q15mulrSatS)},
		{name: OpcodeVecI16x8ExtMulLowI8x16SName, body: vv2v(OpcodeVecI16x8ExtMulLowI8x16S)},
		{name: OpcodeVecI16x8ExtMulHighI8x16SName, body: vv2v(OpcodeVecI16x8ExtMulHighI8x16S)},
		{name: OpcodeVecI16x8ExtMulLowI8x16UName, body: vv2v(OpcodeVecI16x8ExtMulLowI8x16U)},
		{name: OpcodeVecI16x8ExtMulHighI8x16UName, body: vv2v(OpcodeVecI16x8ExtMulHighI8x16U)},
		{name: OpcodeVecI32x4ExtMulLowI16x8SName, body: vv2v(OpcodeVecI32x4ExtMulLowI16x8S)},
		{name: OpcodeVecI32x4ExtMulHighI16x8SName, body: vv2v(OpcodeVecI32x4ExtMulHighI16x8S)},
		{name: OpcodeVecI32x4ExtMulLowI16x8UName, body: vv2v(OpcodeVecI32x4ExtMulLowI16x8U)},
		{name: OpcodeVecI32x4ExtMulHighI16x8UName, body: vv2v(OpcodeVecI32x4ExtMulHighI16x8U)},
		{name: OpcodeVecI64x2ExtMulLowI32x4SName, body: vv2v(OpcodeVecI64x2ExtMulLowI32x4S)},
		{name: OpcodeVecI64x2ExtMulHighI32x4SName, body: vv2v(OpcodeVecI64x2ExtMulHighI32x4S)},
		{name: OpcodeVecI64x2ExtMulLowI32x4UName, body: vv2v(OpcodeVecI64x2ExtMulLowI32x4U)},
		{name: OpcodeVecI64x2ExtMulHighI32x4UName, body: vv2v(OpcodeVecI64x2ExtMulHighI32x4U)},
		{name: OpcodeVecI16x8ExtaddPairwiseI8x16SName, body: v2v(OpcodeVecI16x8ExtaddPairwiseI8x16S)},
		{name: OpcodeVecI16x8ExtaddPairwiseI8x16UName, body: v2v(OpcodeVecI16x8ExtaddPairwiseI8x16U)},
		{name: OpcodeVecI32x4ExtaddPairwiseI16x8SName, body: v2v(OpcodeVecI32x4ExtaddPairwiseI16x8S)},
		{name: OpcodeVecI32x4ExtaddPairwiseI16x8UName, body: v2v(OpcodeVecI32x4ExtaddPairwiseI16x8U)},
		{name: OpcodeVecF64x2PromoteLowF32x4ZeroName, body: v2v(OpcodeVecF64x2PromoteLowF32x4Zero)},
		{name: OpcodeVecF32x4DemoteF64x2ZeroName, body: v2v(OpcodeVecF32x4DemoteF64x2Zero)},
		{name: OpcodeVecF32x4ConvertI32x4SName, body: v2v(OpcodeVecF32x4ConvertI32x4S)},
		{name: OpcodeVecF32x4ConvertI32x4UName, body: v2v(OpcodeVecF32x4ConvertI32x4U)},
		{name: OpcodeVecF64x2ConvertLowI32x4SName, body: v2v(OpcodeVecF64x2ConvertLowI32x4S)},
		{name: OpcodeVecF64x2ConvertLowI32x4UName, body: v2v(OpcodeVecF64x2ConvertLowI32x4U)},
		{name: OpcodeVecI32x4DotI16x8SName, body: vv2v(OpcodeVecI32x4DotI16x8S)},
		{name: OpcodeVecI8x16NarrowI16x8SName, body: vv2v(OpcodeVecI8x16NarrowI16x8S)},
		{name: OpcodeVecI8x16NarrowI16x8UName, body: vv2v(OpcodeVecI8x16NarrowI16x8U)},
		{name: OpcodeVecI16x8NarrowI32x4SName, body: vv2v(OpcodeVecI16x8NarrowI32x4S)},
		{name: OpcodeVecI16x8NarrowI32x4UName, body: vv2v(OpcodeVecI16x8NarrowI32x4U)},
		{name: OpcodeVecI32x4TruncSatF32x4SName, body: v2v(OpcodeVecI32x4TruncSatF32x4S)},
		{name: OpcodeVecI32x4TruncSatF32x4UName, body: v2v(OpcodeVecI32x4TruncSatF32x4U)},
		{name: OpcodeVecI32x4TruncSatF64x2SZeroName, body: v2v(OpcodeVecI32x4TruncSatF64x2SZero)},
		{name: OpcodeVecI32x4TruncSatF64x2UZeroName, body: v2v(OpcodeVecI32x4TruncSatF64x2UZero)},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			m := &Module{
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection:     []Code{{Body: tc.body}},
			}
			err := m.validateFunction(&stacks{}, api.CoreFeatureSIMD,
				0, []Index{0}, nil, &Memory{}, nil, nil, bytes.NewReader(nil))
			require.NoError(t, err)
		})
	}
}

func TestModule_funcValidation_SIMD_error(t *testing.T) {
	type testCase struct {
		name        string
		body        []byte
		flag        api.CoreFeatures
		expectedErr string
	}

	tests := []testCase{
		{
			name: "simd disabled",
			body: []byte{
				OpcodeVecPrefix,
				OpcodeVecF32x4Abs,
			},
			flag:        api.CoreFeaturesV1,
			expectedErr: "f32x4.abs invalid as feature \"simd\" is disabled",
		},
		{
			name: "v128.const immediate",
			body: []byte{
				OpcodeVecPrefix,
				OpcodeVecV128Const,
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1,
			},
			flag:        api.CoreFeatureSIMD,
			expectedErr: "cannot read constant vector value for v128.const",
		},
		{
			name: "i32x4.add operand",
			body: []byte{
				OpcodeVecPrefix,
				OpcodeVecV128Const,
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
				OpcodeVecPrefix,
				OpcodeVecI32x4Add,
				OpcodeDrop,
				OpcodeEnd,
			},
			flag:        api.CoreFeatureSIMD,
			expectedErr: "cannot pop the operand for i32x4.add: v128 missing",
		},
		{
			name: "i64x2.add operand",
			body: []byte{
				OpcodeVecPrefix,
				OpcodeVecV128Const,
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
				OpcodeVecPrefix,
				OpcodeVecI64x2Add,
				OpcodeDrop,
				OpcodeEnd,
			},
			flag:        api.CoreFeatureSIMD,
			expectedErr: "cannot pop the operand for i64x2.add: v128 missing",
		},
		{
			name: "shuffle lane index not found",
			flag: api.CoreFeatureSIMD,
			body: []byte{
				OpcodeVecPrefix,
				OpcodeVecV128i8x16Shuffle,
			},
			expectedErr: "16 lane indexes for v128.shuffle not found",
		},
		{
			name: "shuffle lane index not found",
			flag: api.CoreFeatureSIMD,
			body: []byte{
				OpcodeVecPrefix,
				OpcodeVecV128i8x16Shuffle,
				0xff, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
			},
			expectedErr: "invalid lane index[0] 255 >= 32 for v128.shuffle",
		},
	}

	addExtractOrReplaceLaneOutOfIndexCase := func(op OpcodeVec, lane, laneCeil byte) {
		n := VectorInstructionName(op)
		tests = append(tests, testCase{
			name: n + "/lane index out of range",
			flag: api.CoreFeatureSIMD,
			body: []byte{
				OpcodeVecPrefix, op, lane,
			},
			expectedErr: fmt.Sprintf("invalid lane index %d >= %d for %s", lane, laneCeil, n),
		})
	}

	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecI8x16ExtractLaneS, 16, 16)
	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecI8x16ExtractLaneU, 20, 16)
	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecI16x8ExtractLaneS, 8, 8)
	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecI16x8ExtractLaneU, 8, 8)
	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecI32x4ExtractLane, 4, 4)
	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecF32x4ExtractLane, 4, 4)
	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecI64x2ExtractLane, 2, 2)
	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecF64x2ExtractLane, 2, 2)
	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecI8x16ReplaceLane, 16, 16)
	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecI16x8ReplaceLane, 8, 8)
	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecI32x4ReplaceLane, 4, 4)
	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecI64x2ReplaceLane, 2, 2)
	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecF32x4ReplaceLane, 10, 4)
	addExtractOrReplaceLaneOutOfIndexCase(OpcodeVecF64x2ReplaceLane, 3, 2)

	addStoreOrLoadLaneOutOfIndexCase := func(op OpcodeVec, lane, laneCeil byte) {
		n := VectorInstructionName(op)
		tests = append(tests, testCase{
			name: n + "/lane index out of range",
			flag: api.CoreFeatureSIMD,
			body: []byte{
				OpcodeVecPrefix, op,
				0, 0, // align and offset.
				lane,
			},
			expectedErr: fmt.Sprintf("invalid lane index %d >= %d for %s", lane, laneCeil, n),
		})
	}

	addStoreOrLoadLaneOutOfIndexCase(OpcodeVecV128Load8Lane, 16, 16)
	addStoreOrLoadLaneOutOfIndexCase(OpcodeVecV128Load16Lane, 8, 8)
	addStoreOrLoadLaneOutOfIndexCase(OpcodeVecV128Load32Lane, 4, 4)
	addStoreOrLoadLaneOutOfIndexCase(OpcodeVecV128Load64Lane, 2, 2)
	addStoreOrLoadLaneOutOfIndexCase(OpcodeVecV128Store8Lane, 16, 16)
	addStoreOrLoadLaneOutOfIndexCase(OpcodeVecV128Store16Lane, 8, 8)
	addStoreOrLoadLaneOutOfIndexCase(OpcodeVecV128Store32Lane, 4, 4)
	addStoreOrLoadLaneOutOfIndexCase(OpcodeVecV128Store64Lane, 2, 2)

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			m := &Module{
				TypeSection:     []FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection:     []Code{{Body: tc.body}},
			}
			err := m.validateFunction(&stacks{}, tc.flag,
				0, []Index{0}, nil, &Memory{}, nil, nil, bytes.NewReader(nil))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestDecodeBlockType(t *testing.T) {
	t.Run("primitive", func(t *testing.T) {
		for _, tc := range []struct {
			name                 string
			in                   byte
			exp                  ValueType
			expResultNumInUint64 int
		}{
			{name: "nil", in: 0x40},
			{name: "i32", in: 0x7f, exp: ValueTypeI32, expResultNumInUint64: 1},
			{name: "i64", in: 0x7e, exp: ValueTypeI64, expResultNumInUint64: 1},
			{name: "f32", in: 0x7d, exp: ValueTypeF32, expResultNumInUint64: 1},
			{name: "f64", in: 0x7c, exp: ValueTypeF64, expResultNumInUint64: 1},
			{name: "v128", in: 0x7b, exp: ValueTypeV128, expResultNumInUint64: 2},
			{name: "funcref", in: 0x70, exp: ValueTypeFuncref, expResultNumInUint64: 1},
			{name: "externref", in: 0x6f, exp: ValueTypeExternref, expResultNumInUint64: 1},
		} {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				actual, read, err := DecodeBlockType(nil, bytes.NewReader([]byte{tc.in}), api.CoreFeaturesV2)
				require.NoError(t, err)
				require.Equal(t, uint64(1), read)
				require.Equal(t, 0, len(actual.Params))
				require.Equal(t, tc.expResultNumInUint64, actual.ResultNumInUint64)
				require.Equal(t, 0, actual.ParamNumInUint64)
				if tc.exp == 0 {
					require.Equal(t, 0, len(actual.Results))
				} else {
					require.Equal(t, 1, len(actual.Results))
					require.Equal(t, tc.exp, actual.Results[0])
				}
			})
		}
	})
	t.Run("function type", func(t *testing.T) {
		types := []FunctionType{
			{},
			{Params: []ValueType{ValueTypeI32}},
			{Results: []ValueType{ValueTypeI32}},
			{Params: []ValueType{ValueTypeF32, ValueTypeV128}, Results: []ValueType{ValueTypeI32}},
			{Params: []ValueType{ValueTypeF32, ValueTypeV128}, Results: []ValueType{ValueTypeI32, ValueTypeF32, ValueTypeV128}},
		}
		for index := range types {
			expected := &types[index]
			actual, read, err := DecodeBlockType(types, bytes.NewReader([]byte{byte(index)}), api.CoreFeatureMultiValue)
			require.NoError(t, err)
			require.Equal(t, uint64(1), read)
			require.Equal(t, expected, actual)
		}
	})
}

// TestFuncValidation_UnreachableBrTable_NotModifyTypes ensures that we do not modify the
// original function type during the function validation with the presence of unreachable br_table
// targeting the function return label.
func TestFuncValidation_UnreachableBrTable_NotModifyTypes(t *testing.T) {
	funcType := FunctionType{Results: []ValueType{i32, i64}, Params: []ValueType{i32}}
	copiedFuncType := FunctionType{
		Params:  make([]ValueType, len(funcType.Params)),
		Results: make([]ValueType, len(funcType.Results)),
	}

	copy(copiedFuncType.Results, funcType.Results)
	copy(copiedFuncType.Params, funcType.Params)

	for _, tc := range []struct {
		name string
		m    *Module
	}{
		{
			name: "on function return",
			m: &Module{
				TypeSection:     []FunctionType{funcType},
				FunctionSection: []Index{0},
				CodeSection: []Code{
					{Body: []byte{
						OpcodeUnreachable,
						// Having br_table in unreachable state.
						OpcodeI32Const, 1,
						// Setting the destination as labels of index 0 which
						// is the function return.
						OpcodeBrTable, 2, 0, 0, 0,
						OpcodeEnd,
					}},
				},
			},
		},
		{
			name: "on loop return",
			m: &Module{
				TypeSection:     []FunctionType{funcType},
				FunctionSection: []Index{0},
				CodeSection: []Code{
					{Body: []byte{
						OpcodeUnreachable,
						OpcodeLoop, 0, // indicates that loop has funcType as its block type
						OpcodeUnreachable,
						// Having br_table in unreachable state.
						OpcodeI32Const, 1,
						// Setting the destination as labels of index 0 which
						// is the loop return.
						OpcodeBrTable, 2, 0, 0, 0,
						OpcodeEnd, // End of loop
						OpcodeEnd,
					}},
				},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.m.validateFunction(&stacks{}, api.CoreFeaturesV2,
				0, nil, nil, nil, nil, nil, bytes.NewReader(nil))
			require.NoError(t, err)

			// Ensures that funcType has remained intact.
			require.Equal(t, copiedFuncType, funcType)
		})
	}
}

func TestModule_funcValidation_loopWithParams(t *testing.T) {
	tests := []struct {
		name   string
		body   []byte
		expErr string
	}{
		{
			name: "br",
			body: []byte{
				OpcodeI32Const, 1,
				OpcodeLoop, 1, // loop (param i32)
				OpcodeBr, 0,
				OpcodeEnd,
				OpcodeUnreachable,
				OpcodeEnd,
			},
		},
		{
			name: "br_if",
			body: []byte{
				OpcodeI32Const, 1,
				OpcodeLoop, 1, // loop (param i32)
				OpcodeI32Const, 1, // operand for br_if
				OpcodeBrIf, 0,
				OpcodeUnreachable,
				OpcodeEnd,
				OpcodeUnreachable,
				OpcodeEnd,
			},
		},
		{
			name: "br_table",
			body: []byte{
				OpcodeI32Const, 1,
				OpcodeLoop, 1, // loop (param i32)
				OpcodeI32Const, 4, // Operand for br_table.
				OpcodeBrTable, 2, 0, 0, 0, 0, // Jump into the loop header anyway.
				OpcodeEnd,
				OpcodeUnreachable,
				OpcodeEnd,
			},
		},
		{
			name: "br_table - nested",
			body: []byte{
				OpcodeI32Const, 1,
				OpcodeLoop, 1, // loop (param i32)
				OpcodeLoop, 1, // loop (param i32)
				OpcodeI32Const, 4, // Operand for br_table.
				OpcodeBrTable, 2, 0, 1, 0, // Jump into the loop header anyway.
				OpcodeEnd,
				OpcodeEnd,
				OpcodeUnreachable,
				OpcodeEnd,
			},
		},
		{
			name: "br / mismatch",
			body: []byte{
				OpcodeI32Const, 1,
				OpcodeLoop, 1, // loop (param i32)
				OpcodeDrop,
				OpcodeBr, 0, // trying to jump the loop head after dropping the value which causes the type mismatch.
				OpcodeEnd,
				OpcodeUnreachable,
				OpcodeEnd,
			},
			expErr: `not enough results in br block
	have ()
	want (i32)`,
		},
		{
			name: "br_if / mismatch",
			body: []byte{
				OpcodeI32Const, 1,
				OpcodeLoop, 1, // loop (param i32)
				// Use up the param for the br_if, therefore at the time of jumping, we don't have any param to the header.
				OpcodeBrIf, 0, // trying to jump the loop head after dropping the value which causes the type mismatch.
				OpcodeUnreachable,
				OpcodeEnd,
				OpcodeUnreachable,
				OpcodeEnd,
			},
			expErr: `not enough results in br_if block
	have ()
	want (i32)`,
		},
		{
			name: "br_table",
			body: []byte{
				OpcodeI32Const, 1,
				OpcodeLoop, 1, // loop (param i32)
				// Use up the param for the br_table, therefore at the time of jumping, we don't have any param to the header.
				OpcodeBrTable, 2, 0, 0, 0, // Jump into the loop header anyway.
				OpcodeEnd,
				OpcodeUnreachable,
				OpcodeEnd,
			},
			expErr: `not enough results in br_table block
	have ()
	want (i32)`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			m := &Module{
				TypeSection: []FunctionType{
					v_i32,
					i32_v,
				},
				FunctionSection: []Index{0},
				CodeSection:     []Code{{Body: tc.body}},
			}
			err := m.validateFunction(&stacks{}, api.CoreFeatureMultiValue,
				0, []Index{0}, nil, nil, nil, nil, bytes.NewReader(nil))
			if tc.expErr != "" {
				require.EqualError(t, err, tc.expErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestFunctionValidation_redundantEnd is found in th validation fuzzing #879.
func TestFunctionValidation_redundantEnd(t *testing.T) {
	m := &Module{
		TypeSection:     []FunctionType{{}},
		FunctionSection: []Index{0},
		CodeSection:     []Code{{Body: []byte{OpcodeEnd, OpcodeEnd}}},
	}
	err := m.validateFunction(&stacks{}, api.CoreFeaturesV2,
		0, nil, nil, nil, nil, nil, bytes.NewReader(nil))
	require.EqualError(t, err, "unexpected end of function at pc=0x1")
}

// TestFunctionValidation_redundantEnd is found in th validation fuzzing.
func TestFunctionValidation_redundantElse(t *testing.T) {
	for _, tc := range []struct {
		body   []byte
		expErr string
	}{
		{
			body:   []byte{OpcodeEnd, OpcodeElse},
			expErr: "unexpected end of function at pc=0x1",
		},
		{
			body:   []byte{OpcodeElse, OpcodeEnd},
			expErr: "else instruction must be used in if block: 0x0",
		},
		{
			body:   []byte{OpcodeBlock, 0, OpcodeElse, OpcodeEnd},
			expErr: "else instruction must be used in if block: 0x2",
		},
		{
			body: []byte{
				OpcodeI32Const, 0,
				OpcodeIf, 0, OpcodeElse, OpcodeElse, OpcodeEnd, OpcodeEnd,
			},
			expErr: "else instruction must be used in if block: 0x5",
		},
	} {
		t.Run(tc.expErr, func(t *testing.T) {
			m := &Module{TypeSection: []FunctionType{{}}, FunctionSection: []Index{0}, CodeSection: []Code{{Body: tc.body}}}
			err := m.validateFunction(&stacks{}, api.CoreFeaturesV2,
				0, nil, nil, nil, nil, nil, bytes.NewReader(nil))
			require.EqualError(t, err, tc.expErr)
		})
	}
}

func Test_SplitCallStack(t *testing.T) {
	oneToEight := []uint64{1, 2, 3, 4, 5, 6, 7, 8}

	tests := []struct {
		name                                   string
		ft                                     *FunctionType
		stack, expectedParams, expectedResults []uint64
		expectedErr                            string
	}{
		{
			name:            "v_v",
			ft:              &v_v,
			stack:           oneToEight,
			expectedParams:  nil,
			expectedResults: nil,
		},
		{
			name:            "v_v - stack nil",
			ft:              &v_v,
			expectedParams:  nil,
			expectedResults: nil,
		},
		{
			name:            "v_i32",
			ft:              &v_i32,
			stack:           oneToEight,
			expectedParams:  nil,
			expectedResults: []uint64{1},
		},
		{
			name:            "f32i32_v",
			ft:              &f32i32_v,
			stack:           oneToEight,
			expectedParams:  []uint64{1, 2},
			expectedResults: nil,
		},
		{
			name:            "f64f32_i64",
			ft:              &f64f32_i64,
			stack:           oneToEight,
			expectedParams:  []uint64{1, 2},
			expectedResults: []uint64{1},
		},
		{
			name:            "f64i32_v128i64",
			ft:              &f64i32_v128i64,
			stack:           oneToEight,
			expectedParams:  []uint64{1, 2},
			expectedResults: []uint64{1, 2, 3},
		},
		{
			name:        "not enough room for params",
			ft:          &f64i32_v128i64,
			stack:       oneToEight[0:1],
			expectedErr: "need 2 params, but stack size is 1",
		},
		{
			name:        "not enough room for results",
			ft:          &f64i32_v128i64,
			stack:       oneToEight[0:2],
			expectedErr: "need 3 results, but stack size is 2",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			params, results, err := SplitCallStack(tc.ft, tc.stack)
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.Equal(t, tc.expectedParams, params)
				require.Equal(t, tc.expectedResults, results)
			}
		})
	}
}

func TestModule_funcValidation_Atomic(t *testing.T) {
	t.Run("valid bytecode", func(t *testing.T) {
		tests := []struct {
			name               string
			body               []byte
			noDropBeforeReturn bool
		}{
			{
				name: "i32.atomic.load8_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI32Load8U, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i32.atomic.load16_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI32Load16U, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.load",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI32Load, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.load8_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI64Load8U, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i64.atomic.load16_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI64Load16U, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.load32_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI64Load32U, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.load",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI64Load, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i32.atomic.store8",
				body: []byte{
					OpcodeI32Const, 0x1,
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI32Store8, 0x0, 0x8, // alignment=2^0, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i32.atomic.store16",
				body: []byte{
					OpcodeI32Const, 0x1,
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI32Store16, 0x1, 0x8, // alignment=2^1, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i32.atomic.store",
				body: []byte{
					OpcodeI32Const, 0x1,
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI32Store, 0x2, 0x8, // alignment=2^2, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i64.atomic.store8",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Store8, 0x0, 0x8, // alignment=2^0, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i64.atomic.store16",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Store16, 0x1, 0x8, // alignment=2^1, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i64.atomic.store32",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Store32, 0x2, 0x8, // alignment=2^2, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i64.atomic.store",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Store, 0x3, 0x8, // alignment=2^3, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i32.atomic.rmw8.add_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8AddU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.add_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16AddU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.add",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwAdd, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.add_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8AddU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.add_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16AddU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.add_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32AddU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.add",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwAdd, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i32.atomic.rmw8.sub_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8SubU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.sub_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16SubU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.sub",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwSub, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.sub_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8SubU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.sub_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16SubU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.sub_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32SubU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.sub",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwSub, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i32.atomic.rmw8.and_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8AndU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.and_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16AndU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.and",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwAnd, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.and_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8AndU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.and_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16AndU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.and_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32AndU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.and",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwAnd, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i32.atomic.rmw8.or",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8OrU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.or_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16OrU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.or",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwOr, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.or_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8OrU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.or_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16OrU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.or_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32OrU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.or",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwOr, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i32.atomic.rmw8.xor_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8XorU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.xor_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16XorU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.xor",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwXor, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.xor_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8XorU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.xor_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16XorU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.xor_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32XorU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.xor",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwXor, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i32.atomic.rmw8.xchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8XchgU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.xchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16XchgU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.xchg",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwXchg, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.xchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8XchgU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.xchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16XchgU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.xchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32XchgU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.xchg",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwXchg, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i32.atomic.rmw8.cmpxchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeI32Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8CmpxchgU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.cmpxchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeI32Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16CmpxchgU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.cmpxchg",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeI32Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwCmpxchg, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.xchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeI64Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8CmpxchgU, 0x0, 0x8, // alignment=2^0, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.cmpxchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeI64Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16CmpxchgU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.cmpxchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32CmpxchgU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.cmpxchg",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeI64Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwCmpxchg, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "memory.atomic.wait32",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeI64Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicMemoryWait32, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "memory.atomic.wait64",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeI64Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicMemoryWait64, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "memory.atomic.notify",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicMemoryNotify, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "memory.atomic.fence",
				body: []byte{
					OpcodeAtomicPrefix, OpcodeAtomicFence, 0x0,
				},
				noDropBeforeReturn: true,
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.name, func(t *testing.T) {
				body := append([]byte{}, tc.body...)
				if !tt.noDropBeforeReturn {
					body = append(body, OpcodeDrop)
				}
				body = append(body, OpcodeEnd)
				m := &Module{
					TypeSection:     []FunctionType{v_v},
					FunctionSection: []Index{0},
					CodeSection:     []Code{{Body: body}},
				}

				t.Run("with memory", func(t *testing.T) {
					err := m.validateFunction(&stacks{}, experimental.CoreFeaturesThreads,
						0, []Index{0}, nil, &Memory{}, []Table{}, nil, bytes.NewReader(nil))
					require.NoError(t, err)
				})

				t.Run("without memory", func(t *testing.T) {
					err := m.validateFunction(&stacks{}, experimental.CoreFeaturesThreads,
						0, []Index{0}, nil, nil, []Table{}, nil, bytes.NewReader(nil))
					// Only fence doesn't require memory
					if tc.name == "memory.atomic.fence" {
						require.NoError(t, err)
					} else {
						require.Error(t, err, fmt.Sprintf("memory must exist for %s", tc.name))
					}
				})
			})
		}
	})

	t.Run("atomic.fence bad immediate", func(t *testing.T) {
		body := []byte{
			OpcodeAtomicPrefix, OpcodeAtomicFence, 0x1,
			OpcodeEnd,
		}
		m := &Module{
			TypeSection:     []FunctionType{v_v},
			FunctionSection: []Index{0},
			CodeSection:     []Code{{Body: body}},
		}
		err := m.validateFunction(&stacks{}, experimental.CoreFeaturesThreads,
			0, []Index{0}, nil, &Memory{}, []Table{}, nil, bytes.NewReader(nil))
		require.Error(t, err, "invalid immediate value for atomic.fence")
	})

	t.Run("bad alignment", func(t *testing.T) {
		tests := []struct {
			name               string
			body               []byte
			noDropBeforeReturn bool
		}{
			{
				name: "i32.atomic.load8_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI32Load8U, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.load16_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI32Load16U, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i32.atomic.load",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI32Load, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.load8_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI64Load8U, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.load16_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI64Load16U, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.load32_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI64Load32U, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.load",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI64Load, 0x4, 0x8, // alignment=2^4, offset=8
				},
			},
			{
				name: "i32.atomic.store8",
				body: []byte{
					OpcodeI32Const, 0x1,
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI32Store8, 0x1, 0x8, // alignment=2^1, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i32.atomic.store16",
				body: []byte{
					OpcodeI32Const, 0x1,
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI32Store16, 0x2, 0x8, // alignment=2^2, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i32.atomic.store",
				body: []byte{
					OpcodeI32Const, 0x1,
					OpcodeI32Const, 0x0,
					OpcodeAtomicPrefix, OpcodeAtomicI32Store, 0x3, 0x8, // alignment=2^3, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i64.atomic.store8",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Store8, 0x1, 0x8, // alignment=2^1, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i64.atomic.store16",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Store16, 0x2, 0x8, // alignment=2^2, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i64.atomic.store32",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Store32, 0x3, 0x8, // alignment=2^3, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i64.atomic.store",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Store, 0x4, 0x8, // alignment=2^4, offset=8
				},
				noDropBeforeReturn: true,
			},
			{
				name: "i32.atomic.rmw8.add_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8AddU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.add_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16AddU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.add",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwAdd, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.add_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8AddU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.add_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16AddU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.add_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32AddU, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.add",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwAdd, 0x4, 0x8, // alignment=2^4, offset=8
				},
			},
			{
				name: "i32.atomic.rmw8.sub_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8SubU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.sub_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16SubU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.sub",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwSub, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.sub_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8SubU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.sub_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16SubU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.sub_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32SubU, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.sub",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwSub, 0x4, 0x8, // alignment=2^4, offset=8
				},
			},
			{
				name: "i32.atomic.rmw8.and_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8AndU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.and_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16AndU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.and",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwAnd, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.and_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8AndU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.and_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16AndU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.and_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32AndU, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.and",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwAnd, 0x4, 0x8, // alignment=2^4, offset=8
				},
			},
			{
				name: "i32.atomic.rmw8.or",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8OrU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.or_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16OrU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.or",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwOr, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.or_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8OrU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.or_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16OrU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.or_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32OrU, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.or",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwOr, 0x4, 0x8, // alignment=2^4, offset=8
				},
			},
			{
				name: "i32.atomic.rmw8.xor_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8XorU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.xor_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16XorU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.xor",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwXor, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.xor_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8XorU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.xor_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16XorU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.xor_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32XorU, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.xor",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwXor, 0x4, 0x8, // alignment=2^4, offset=8
				},
			},
			{
				name: "i32.atomic.rmw8.xchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8XchgU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.xchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16XchgU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.xchg",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwXchg, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.xchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8XchgU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.xchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16XchgU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.xchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32XchgU, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.xchg",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwXchg, 0x4, 0x8, // alignment=2^4, offset=8
				},
			},
			{
				name: "i32.atomic.rmw8.cmpxchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeI32Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw8CmpxchgU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i32.atomic.rmw16.cmpxchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeI32Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicI32Rmw16CmpxchgU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i32.atomic.rmw.cmpxchg",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeI32Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicI32RmwCmpxchg, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw8.xchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeI64Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw8CmpxchgU, 0x1, 0x8, // alignment=2^1, offset=8
				},
			},
			{
				name: "i64.atomic.rmw16.cmpxchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeI64Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw16CmpxchgU, 0x2, 0x8, // alignment=2^2, offset=8
				},
			},
			{
				name: "i64.atomic.rmw32.cmpxchg_u",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeI64Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicI64Rmw32CmpxchgU, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "i64.atomic.rmw.cmpxchg",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeI64Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicI64RmwCmpxchg, 0x4, 0x8, // alignment=2^4, offset=8
				},
			},
			{
				name: "memory.atomic.wait32",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeI64Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicMemoryWait32, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
			{
				name: "memory.atomic.wait64",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI64Const, 0x1,
					OpcodeI64Const, 0x2,
					OpcodeAtomicPrefix, OpcodeAtomicMemoryWait64, 0x4, 0x8, // alignment=2^4, offset=8
				},
			},
			{
				name: "memory.atomic.notify",
				body: []byte{
					OpcodeI32Const, 0x0,
					OpcodeI32Const, 0x1,
					OpcodeAtomicPrefix, OpcodeAtomicMemoryNotify, 0x3, 0x8, // alignment=2^3, offset=8
				},
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.name, func(t *testing.T) {
				body := append([]byte{}, tc.body...)
				if !tt.noDropBeforeReturn {
					body = append(body, OpcodeDrop)
				}
				body = append(body, OpcodeEnd)
				m := &Module{
					TypeSection:     []FunctionType{v_v},
					FunctionSection: []Index{0},
					CodeSection:     []Code{{Body: body}},
				}
				err := m.validateFunction(&stacks{}, experimental.CoreFeaturesThreads,
					0, []Index{0}, nil, &Memory{}, []Table{}, nil, bytes.NewReader(nil))
				require.Error(t, err, "invalid memory alignment")
			})
		}
	})
}
