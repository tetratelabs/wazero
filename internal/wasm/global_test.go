package internalwasm

import (
	"context"
	gobinary "encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	publicwasm "github.com/tetratelabs/wazero/wasm"
)

func TestGlobalTypes(t *testing.T) {
	tests := []struct {
		name            string
		global          publicwasm.Global
		expectedType    publicwasm.ValueType
		expectedVal     uint64
		expectedString  string
		expectedMutable bool
	}{
		{
			name:           "i32 - immutable",
			global:         globalI32(1),
			expectedType:   ValueTypeI32,
			expectedVal:    1,
			expectedString: "global(1)",
		},
		{
			name:           "i64 - immutable",
			global:         globalI64(1),
			expectedType:   ValueTypeI64,
			expectedVal:    1,
			expectedString: "global(1)",
		},
		{
			name:           "f32 - immutable",
			global:         globalF32(publicwasm.EncodeF32(1.0)),
			expectedType:   ValueTypeF32,
			expectedVal:    publicwasm.EncodeF32(1.0),
			expectedString: "global(1.000000)",
		},
		{
			name:           "f64 - immutable",
			global:         globalF64(publicwasm.EncodeF64(1.0)),
			expectedType:   ValueTypeF64,
			expectedVal:    publicwasm.EncodeF64(1.0),
			expectedString: "global(1.000000)",
		},
		{
			name: "i32 - mutable",
			global: &mutableGlobal{g: &GlobalInstance{
				Type: &GlobalType{ValType: ValueTypeI32, Mutable: true},
				Val:  1,
			}},
			expectedType:    ValueTypeI32,
			expectedVal:     1,
			expectedString:  "global(1)",
			expectedMutable: true,
		},
		{
			name: "i64 - mutable",
			global: &mutableGlobal{g: &GlobalInstance{
				Type: &GlobalType{ValType: ValueTypeI64, Mutable: true},
				Val:  1,
			}},
			expectedType:    ValueTypeI64,
			expectedVal:     1,
			expectedString:  "global(1)",
			expectedMutable: true,
		},
		{
			name: "f32 - mutable",
			global: &mutableGlobal{g: &GlobalInstance{
				Type: &GlobalType{ValType: ValueTypeF32, Mutable: true},
				Val:  publicwasm.EncodeF32(1.0),
			}},
			expectedType:    ValueTypeF32,
			expectedVal:     publicwasm.EncodeF32(1.0),
			expectedString:  "global(1.000000)",
			expectedMutable: true,
		},
		{
			name: "f64 - mutable",
			global: &mutableGlobal{g: &GlobalInstance{
				Type: &GlobalType{ValType: ValueTypeF64, Mutable: true},
				Val:  publicwasm.EncodeF64(1.0),
			}},
			expectedType:    ValueTypeF64,
			expectedVal:     publicwasm.EncodeF64(1.0),
			expectedString:  "global(1.000000)",
			expectedMutable: true,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expectedType, tc.global.Type())
			require.Equal(t, tc.expectedVal, tc.global.Get())
			require.Equal(t, tc.expectedString, tc.global.String())

			mutable, ok := tc.global.(publicwasm.MutableGlobal)
			require.Equal(t, tc.expectedMutable, ok)
			if ok {
				mutable.Set(2)
				require.Equal(t, uint64(2), tc.global.Get())
			}
		})
	}
}

func TestPublicModule_Global(t *testing.T) {
	tests := []struct {
		name     string
		module   *Module // module as wat doesn't yet support globals
		expected publicwasm.Global
	}{
		{
			name:   "no global",
			module: &Module{},
		},
		{
			name: "global not exported",
			module: &Module{
				GlobalSection: []*Global{
					{
						Type: &GlobalType{ValType: ValueTypeI32},
						Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{1}},
					},
				},
			},
		},
		{
			name: "global exported - immutable I32",
			module: &Module{
				GlobalSection: []*Global{
					{
						Type: &GlobalType{ValType: ValueTypeI32},
						Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{1}},
					},
				},
				ExportSection: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: globalI32(1),
		},
		{
			name: "global exported - immutable I64",
			module: &Module{
				GlobalSection: []*Global{
					{
						Type: &GlobalType{ValType: ValueTypeI64},
						Init: &ConstantExpression{Opcode: OpcodeI64Const, Data: []byte{1}},
					},
				},
				ExportSection: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: globalI64(1),
		},
		{
			name: "global exported - immutable F32",
			module: &Module{
				GlobalSection: []*Global{
					{
						Type: &GlobalType{ValType: ValueTypeF32},
						Init: &ConstantExpression{Opcode: OpcodeF32Const,
							Data: uint64Le(publicwasm.EncodeF32(1.0)),
						},
					},
				},
				ExportSection: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: globalF32(publicwasm.EncodeF32(1.0)),
		},
		{
			name: "global exported - immutable F64",
			module: &Module{
				GlobalSection: []*Global{
					{
						Type: &GlobalType{ValType: ValueTypeF64},
						Init: &ConstantExpression{Opcode: OpcodeF64Const,
							Data: uint64Le(publicwasm.EncodeF64(1.0)),
						},
					},
				},
				ExportSection: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: globalF64(publicwasm.EncodeF64(1.0)),
		},
		{
			name: "global exported - mutable I32",
			module: &Module{
				GlobalSection: []*Global{
					{
						Type: &GlobalType{ValType: ValueTypeI32, Mutable: true},
						Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{1}},
					},
				},
				ExportSection: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: &mutableGlobal{
				g: &GlobalInstance{Type: &GlobalType{ValType: ValueTypeI32, Mutable: true}, Val: 1},
			},
		},
		{
			name: "global exported - mutable I64",
			module: &Module{
				GlobalSection: []*Global{
					{
						Type: &GlobalType{ValType: ValueTypeI64, Mutable: true},
						Init: &ConstantExpression{Opcode: OpcodeI64Const, Data: []byte{1}},
					},
				},
				ExportSection: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: &mutableGlobal{
				g: &GlobalInstance{Type: &GlobalType{ValType: ValueTypeI64, Mutable: true}, Val: 1},
			},
		},
		{
			name: "global exported - mutable F32",
			module: &Module{
				GlobalSection: []*Global{
					{
						Type: &GlobalType{ValType: ValueTypeF32, Mutable: true},
						Init: &ConstantExpression{Opcode: OpcodeF32Const,
							Data: uint64Le(publicwasm.EncodeF32(1.0)),
						},
					},
				},
				ExportSection: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: &mutableGlobal{
				g: &GlobalInstance{Type: &GlobalType{ValType: ValueTypeF32, Mutable: true}, Val: publicwasm.EncodeF32(1.0)},
			},
		},
		{
			name: "global exported - mutable F64",
			module: &Module{
				GlobalSection: []*Global{
					{
						Type: &GlobalType{ValType: ValueTypeF64, Mutable: true},
						Init: &ConstantExpression{Opcode: OpcodeF64Const,
							Data: uint64Le(publicwasm.EncodeF64(1.0)),
						},
					},
				},
				ExportSection: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: &mutableGlobal{
				g: &GlobalInstance{Type: &GlobalType{ValType: ValueTypeF64, Mutable: true}, Val: publicwasm.EncodeF64(1.0)},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		s := newStore()
		t.Run(tc.name, func(t *testing.T) {
			// Instantiate the module and get the export of the above global
			module, err := s.Instantiate(context.Background(), tc.module, t.Name())
			require.NoError(t, err)

			if global := module.ExportedGlobal("global"); tc.expected != nil {
				require.Equal(t, tc.expected, global)
			} else {
				require.Nil(t, global)
			}
		})
	}
}

func uint64Le(v uint64) (ret []byte) {
	ret = make([]byte, 8)
	gobinary.LittleEndian.PutUint64(ret, v)
	return
}
