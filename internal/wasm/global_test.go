package wasm

import (
	"context"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/internalapi"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u64"
)

func TestGlobalTypes(t *testing.T) {
	tests := []struct {
		name            string
		global          api.Global
		expectedType    api.ValueType
		expectedVal     uint64
		expectedString  string
		expectedMutable bool
	}{
		{
			name: "i32 - immutable",
			global: constantGlobal{g: &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeI32},
				Val:  1,
			}},
			expectedType:   ValueTypeI32,
			expectedVal:    1,
			expectedString: "global(1)",
		},
		{
			name: "i32 - immutable - max",
			global: constantGlobal{g: &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeI32},
				Val:  math.MaxInt32,
			}},
			expectedType:   ValueTypeI32,
			expectedVal:    math.MaxInt32,
			expectedString: "global(2147483647)",
		},
		{
			name: "i64 - immutable",
			global: constantGlobal{g: &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeI64},
				Val:  1,
			}},
			expectedType:   ValueTypeI64,
			expectedVal:    1,
			expectedString: "global(1)",
		},
		{
			name: "i64 - immutable - max",
			global: constantGlobal{g: &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeI64},
				Val:  math.MaxInt64,
			}},
			expectedType:   ValueTypeI64,
			expectedVal:    math.MaxInt64,
			expectedString: "global(9223372036854775807)",
		},
		{
			name: "f32 - immutable",
			global: constantGlobal{g: &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeF32},
				Val:  api.EncodeF32(1.0),
			}},
			expectedType:   ValueTypeF32,
			expectedVal:    api.EncodeF32(1.0),
			expectedString: "global(1.000000)",
		},
		{
			name: "f32 - immutable - max",
			global: constantGlobal{g: &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeF32},
				Val:  api.EncodeF32(math.MaxFloat32),
			}},
			expectedType:   ValueTypeF32,
			expectedVal:    api.EncodeF32(math.MaxFloat32),
			expectedString: "global(340282346638528859811704183484516925440.000000)",
		},
		{
			name: "f64 - immutable",
			global: constantGlobal{g: &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeF64},
				Val:  api.EncodeF64(1.0),
			}},
			expectedType:   ValueTypeF64,
			expectedVal:    api.EncodeF64(1.0),
			expectedString: "global(1.000000)",
		},
		{
			name: "f64 - immutable - max",
			global: constantGlobal{g: &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeF64},
				Val:  api.EncodeF64(math.MaxFloat64),
			}},
			expectedType:   ValueTypeF64,
			expectedVal:    api.EncodeF64(math.MaxFloat64),
			expectedString: "global(179769313486231570814527423731704356798070567525844996598917476803157260780028538760589558632766878171540458953514382464234321326889464182768467546703537516986049910576551282076245490090389328944075868508455133942304583236903222948165808559332123348274797826204144723168738177180919299881250404026184124858368.000000)",
		},
		{
			name: "i32 - mutable",
			global: mutableGlobal{internalapi.WazeroOnlyType{}, &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeI32, Mutable: true},
				Val:  1,
			}},
			expectedType:    ValueTypeI32,
			expectedVal:     1,
			expectedString:  "global(1)",
			expectedMutable: true,
		},
		{
			name: "i64 - mutable",
			global: mutableGlobal{internalapi.WazeroOnlyType{}, &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeI64, Mutable: true},
				Val:  1,
			}},
			expectedType:    ValueTypeI64,
			expectedVal:     1,
			expectedString:  "global(1)",
			expectedMutable: true,
		},
		{
			name: "f32 - mutable",
			global: mutableGlobal{internalapi.WazeroOnlyType{}, &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeF32, Mutable: true},
				Val:  api.EncodeF32(1.0),
			}},
			expectedType:    ValueTypeF32,
			expectedVal:     api.EncodeF32(1.0),
			expectedString:  "global(1.000000)",
			expectedMutable: true,
		},
		{
			name: "f64 - mutable",
			global: mutableGlobal{internalapi.WazeroOnlyType{}, &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeF64, Mutable: true},
				Val:  api.EncodeF64(1.0),
			}},
			expectedType:    ValueTypeF64,
			expectedVal:     api.EncodeF64(1.0),
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

			mutable, ok := tc.global.(api.MutableGlobal)
			require.Equal(t, tc.expectedMutable, ok)
			if ok {
				mutable.Set(2)
				require.Equal(t, uint64(2), tc.global.Get())

				mutable.Set(tc.expectedVal) // Set it back!
				require.Equal(t, tc.expectedVal, tc.global.Get())
			}
		})
	}
}

func TestPublicModule_Global(t *testing.T) {
	tests := []struct {
		name     string
		module   *Module // module as wat doesn't yet support globals
		expected api.Global
	}{
		{
			name:   "no global",
			module: &Module{},
		},
		{
			name: "global not exported",
			module: &Module{
				GlobalSection: []Global{
					{
						Type: GlobalType{ValType: ValueTypeI32},
						Init: ConstantExpression{Opcode: OpcodeI32Const, Data: const1},
					},
				},
			},
		},
		{
			name: "global exported - immutable I32",
			module: &Module{
				GlobalSection: []Global{
					{
						Type: GlobalType{ValType: ValueTypeI32},
						Init: ConstantExpression{Opcode: OpcodeI32Const, Data: const1},
					},
				},
				Exports: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: constantGlobal{g: &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeI32},
				Val:  1,
			}},
		},
		{
			name: "global exported - immutable I64",
			module: &Module{
				GlobalSection: []Global{
					{
						Type: GlobalType{ValType: ValueTypeI64},
						Init: ConstantExpression{Opcode: OpcodeI64Const, Data: leb128.EncodeInt64(1)},
					},
				},
				Exports: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: constantGlobal{g: &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeI64},
				Val:  1,
			}},
		},
		{
			name: "global exported - immutable F32",
			module: &Module{
				GlobalSection: []Global{
					{
						Type: GlobalType{ValType: ValueTypeF32},
						Init: ConstantExpression{
							Opcode: OpcodeF32Const,
							Data:   u64.LeBytes(api.EncodeF32(1.0)),
						},
					},
				},
				Exports: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: constantGlobal{g: &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeF32},
				Val:  api.EncodeF32(1.0),
			}},
		},
		{
			name: "global exported - immutable F64",
			module: &Module{
				GlobalSection: []Global{
					{
						Type: GlobalType{ValType: ValueTypeF64},
						Init: ConstantExpression{
							Opcode: OpcodeF64Const,
							Data:   u64.LeBytes(api.EncodeF64(1.0)),
						},
					},
				},
				Exports: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: constantGlobal{g: &GlobalInstance{
				Type: GlobalType{ValType: ValueTypeF64},
				Val:  api.EncodeF64(1.0),
			}},
		},
		{
			name: "global exported - mutable I32",
			module: &Module{
				GlobalSection: []Global{
					{
						Type: GlobalType{ValType: ValueTypeI32, Mutable: true},
						Init: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeInt32(1)},
					},
				},
				Exports: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: mutableGlobal{
				internalapi.WazeroOnlyType{}, &GlobalInstance{
					Type: GlobalType{ValType: ValueTypeI32, Mutable: true}, Val: 1,
				},
			},
		},
		{
			name: "global exported - mutable I64",
			module: &Module{
				GlobalSection: []Global{
					{
						Type: GlobalType{ValType: ValueTypeI64, Mutable: true},
						Init: ConstantExpression{Opcode: OpcodeI64Const, Data: leb128.EncodeInt64(1)},
					},
				},
				Exports: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: mutableGlobal{
				internalapi.WazeroOnlyType{}, &GlobalInstance{
					Type: GlobalType{ValType: ValueTypeI64, Mutable: true}, Val: 1,
				},
			},
		},
		{
			name: "global exported - mutable F32",
			module: &Module{
				GlobalSection: []Global{
					{
						Type: GlobalType{ValType: ValueTypeF32, Mutable: true},
						Init: ConstantExpression{
							Opcode: OpcodeF32Const,
							Data:   u64.LeBytes(api.EncodeF32(1.0)),
						},
					},
				},
				Exports: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: mutableGlobal{
				internalapi.WazeroOnlyType{}, &GlobalInstance{
					Type: GlobalType{ValType: ValueTypeF32, Mutable: true}, Val: api.EncodeF32(1.0),
				},
			},
		},
		{
			name: "global exported - mutable F64",
			module: &Module{
				GlobalSection: []Global{
					{
						Type: GlobalType{ValType: ValueTypeF64, Mutable: true},
						Init: ConstantExpression{
							Opcode: OpcodeF64Const,
							Data:   u64.LeBytes(api.EncodeF64(1.0)),
						},
					},
				},
				Exports: map[string]*Export{"global": {Type: ExternTypeGlobal, Name: "global"}},
			},
			expected: mutableGlobal{
				internalapi.WazeroOnlyType{}, &GlobalInstance{
					Type: GlobalType{ValType: ValueTypeF64, Mutable: true}, Val: api.EncodeF64(1.0),
				},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		s := newStore()
		t.Run(tc.name, func(t *testing.T) {
			// Instantiate the module and get the export of the above global
			module, err := s.Instantiate(context.Background(), tc.module, t.Name(), nil, nil)
			require.NoError(t, err)

			if global := module.ExportedGlobal("global"); tc.expected != nil {
				require.Equal(t, tc.expected, global)
			} else {
				require.Nil(t, global)
			}
		})
	}
}
