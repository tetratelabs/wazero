package wasm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
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

var (
	f32, f64, i32, i64 = ValueTypeF32, ValueTypeF64, ValueTypeI32, ValueTypeI64
	i32_i32            = &FunctionType{Params: []ValueType{i32}, Results: []ValueType{i32}}
	i32i32_i32         = &FunctionType{Params: []ValueType{i32, i32}, Results: []ValueType{i32}}
	v_v                = &FunctionType{}
	v_f32              = &FunctionType{Results: []ValueType{f32}}
	v_f32f32           = &FunctionType{Results: []ValueType{f32, f32}}
	v_f64i32           = &FunctionType{Results: []ValueType{f64, i32}}
	v_f64f64           = &FunctionType{Results: []ValueType{f64, f64}}
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
			expectedErr: `not enough arguments to return
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
			expectedErr: `not enough arguments to return
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
			expectedErr: `too many arguments to return
	have (i32, i64)
	want ()`,
		},
		{
			name: `func.wast - type-value-num-vs-nums - v_f32f32 -> f32`,
			module: &Module{
				TypeSection:     []*FunctionType{v_f32f32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeF32Const, 0, 0, 0, 0, OpcodeEnd, // (f32.const 0)
				}}},
			},
			expectedErr: `not enough arguments to return
	have (f32)
	want (f32, f32)`,
		},
		{
			name: `func.wast - type-value-num-vs-nums - v_f32 -> f32f32`,
			module: &Module{
				TypeSection:     []*FunctionType{v_f32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeF32Const, 0, 0, 0, 0, OpcodeF32Const, 0, 0, 0, 0, OpcodeEnd, // (f32.const 0) (f32.const 0)
				}}},
			},
			expectedErr: `too many arguments to return
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
			expectedErr: `not enough arguments to return
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
			expectedErr: `not enough arguments to return
	have ()
	want (i32, i64)`,
		},
		{
			name: `func.wast - type-return-last-num-vs-nums`,
			module: &Module{
				TypeSection:     []*FunctionType{v_i64i64},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI64Const, 0, OpcodeReturn, OpcodeEnd, // (return (i64.const 0))
				}}},
			},
			expectedErr: `not enough arguments to return
	have (i64)
	want (i64, i64)`,
		},
		{
			name: `func.wast - type-return-empty-vs-nums`,
			// This example should err because (return) precedes the values expected in the signature (i32i32):
			//	(module (func $type-return-empty-vs-nums (result i32 i32)
			//	  (return) (i32.const 1) (i32.const 2)
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeReturn, OpcodeI32Const, 1, OpcodeI32Const, 2, OpcodeEnd,
				}}},
			},
			expectedErr: `not enough arguments to return
	have ()
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-return-partial-vs-nums`,
			// This example should err because (return) precedes one of the values expected in the signature (i32i32):
			//	(module (func $type-return-partial-vs-nums (result i32 i32)
			//	  (i32.const 1) (return) (i32.const 2)
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeReturn, OpcodeI32Const, 2, OpcodeEnd,
				}}},
			},
			expectedErr: `not enough arguments to return
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-return-void-vs-nums`,
			// This example should err because (return) is empty due to nop, but the signature requires i32i32:
			//	(module (func $type-return-void-vs-nums (result i32 i32)
			//	  (return (nop)) (i32.const 1)
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeNop, OpcodeReturn, // (return (nop))
					OpcodeI32Const, 1, OpcodeEnd, // (i32.const 1)
				}}},
			},
			expectedErr: `not enough arguments to return
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
					OpcodeI32Const, 1, OpcodeI32Const, 2, OpcodeEnd, // (i32.const 1) (i32.const 2)
				}}},
			},
			expectedErr: "cannot use i64 as type i32 in return argument",
		},
		{
			name: `func.wast - type-return-first-num-vs-nums`,
			// This example should err because the first return doesn't match the result types i32i32:
			//	(module (func $type-return-first-num-vs-nums (result i32 i32)
			//	  (return (i32.const 1)) (return (i32.const 1) (i32.const 2))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI64Const, 1, OpcodeReturn, // (return (i64.const 1))
					OpcodeI32Const, 1, OpcodeI32Const, 2, OpcodeReturn, OpcodeEnd, // (return (i32.const 1) (i32.const 2))
				}}},
			},
			expectedErr: "cannot use i64 as type i32 in return argument",
		},
		{
			name: `func.wast - type-break-last-num-vs-nums`,
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 0, OpcodeBr, 0, OpcodeEnd, // (br 0 (i32.const 0))
				}}},
			},
			expectedErr: `type mismatch on the br operation: not enough arguments to return
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-break-void-vs-nums`,
			// This example should err because (br 0) returns no values, but its enclosing function requires two:
			//	(module (func $type-break-void-vs-nums (result i32 i32)
			//	  (br 0) (i32.const 1) (i32.const 2)
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBr, 0, // (br 0)
					OpcodeI32Const, 1, OpcodeI32Const, 2, // (i32.const 1) (i32.const 2)
					OpcodeEnd,
				}}},
			},
			expectedErr: `type mismatch on the br operation: not enough arguments to return
	have ()
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-break-num-vs-nums`,
			// This example should err because (br 0) returns one value, but its enclosing function requires two:
			//	(module (func $type-break-num-vs-nums (result i32 i32)
			//	  (br 0 (i32.const 1)) (i32.const 1) (i32.const 2)
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeBr, 0, // (br 0 (i32.const 1))
					OpcodeI32Const, 1, OpcodeI32Const, 2, // (i32.const 1) (i32.const 2)
					OpcodeEnd,
				}}},
			},
			expectedErr: `type mismatch on the br operation: not enough arguments to return
	have (i32)
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-break-nested-empty-vs-nums`,
			// This example should err because (br 1) doesn't return values, but its enclosing function does:
			//	(module (func $type-break-nested-empty-vs-nums (result i32 i32)
			//	  (block (br 1)) (br 0 (i32.const 1) (i32.const 2))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x40, OpcodeBr, 0x01, OpcodeEnd, // (block (br 1))
					OpcodeI32Const, 1, OpcodeI32Const, 2, OpcodeBr, 0, // (br 0 (i32.const 1) (i32.const 2))
					OpcodeEnd,
				}}},
			},
			expectedErr: `type mismatch on the br operation: not enough arguments to return
	have ()
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-break-nested-void-vs-nums`,
			// This example should err because nop returns the empty type, but the enclosing function returns i32i32:
			//	(module (func $type-break-nested-void-vs-nums (result i32 i32)
			//	  (block (br 1 (nop))) (br 0 (i32.const 1) (i32.const 2))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x40, OpcodeNop, OpcodeBr, 0x01, OpcodeEnd, // (block (br 1 (nop)))
					OpcodeI32Const, 1, OpcodeI32Const, 2, OpcodeBr, 0, // (br 0 (i32.const 1) (i32.const 2))
					OpcodeEnd,
				}}},
			},
			expectedErr: `type mismatch on the br operation: not enough arguments to return
	have ()
	want (i32, i32)`,
		},
		{
			name: `func.wast - type-break-nested-num-vs-nums`,
			// This example should err because the block signature is v_i32, but the enclosing function is v_i32i32:
			//	(module (func $type-break-nested-num-vs-nums (result i32 i32)
			//	  (block (result i32) (br 1 (i32.const 1))) (br 0 (i32.const 1) (i32.const 2))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_i32i32},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeBlock, 0x7f, OpcodeI32Const, 1, OpcodeBr, 1, OpcodeEnd, // (block (result i32) (br 1 (i32.const 1)))
					OpcodeI32Const, 1, OpcodeI32Const, 2, OpcodeBr, 0, // (br 0 (i32.const 1) (i32.const 2))
					OpcodeEnd,
				}}},
			},
			expectedErr: `type mismatch on the br operation: not enough arguments to return
	have (i32)
	want (i32, i32)`,
		},

		// test/core/if.wast
		{
			name: `if.wast - wrong signature for if type use`,
			// This example should err because (br 0) returns no values, but its enclosing function requires two:
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
					OpcodeEnd, // )
					OpcodeEnd, // )
				}}},
			},
			expectedErr: `too many arguments to return
	have (i32)
	want ()`,
		},
		{
			name: `if.wast - type-then-value-nums-vs-void`,
			// This example should err because (if) without a type use returns no values, but its (then) returns two:
			//	(module (func $type-then-value-nums-vs-void
			//	  (if (i32.const 1) (then (i32.const 1) (i32.const 2)))
			//	))
			module: &Module{
				TypeSection:     []*FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection: []*Code{{Body: []byte{
					OpcodeI32Const, 1, OpcodeIf, 0x40, // (if (i32.const 1)
					OpcodeI32Const, 1, OpcodeI32Const, 2, // (then (i32.const 1) (i32.const 2))
					OpcodeEnd, // )
					OpcodeEnd, // )
				}}},
			},
			expectedErr: `invalid then block: too many arguments to return
	have (i32, i32)
	want ()`,
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
