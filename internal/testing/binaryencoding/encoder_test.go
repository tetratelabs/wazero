package binaryencoding

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestModule_Encode(t *testing.T) {
	i32, f32 := wasm.ValueTypeI32, wasm.ValueTypeF32
	zero := uint32(0)

	tests := []struct {
		name     string
		input    *wasm.Module
		expected []byte
	}{
		{
			name:     "empty",
			input:    &wasm.Module{},
			expected: append(Magic, version...),
		},
		{
			name:  "only name section",
			input: &wasm.Module{NameSection: &wasm.NameSection{ModuleName: "simple"}},
			expected: append(append(Magic, version...),
				wasm.SectionIDCustom, 0x0e, // 14 bytes in this section
				0x04, 'n', 'a', 'm', 'e',
				subsectionIDModuleName, 0x07, // 7 bytes in this subsection
				0x06, // the Module name simple is 6 bytes long
				's', 'i', 'm', 'p', 'l', 'e'),
		},
		{
			name: "type section",
			input: &wasm.Module{
				TypeSection: []wasm.FunctionType{
					{},
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
			},
			expected: append(append(Magic, version...),
				wasm.SectionIDType, 0x12, // 18 bytes in this section
				0x03,             // 3 types
				0x60, 0x00, 0x00, // func=0x60 no param no result
				0x60, 0x02, i32, i32, 0x01, i32, // func=0x60 2 params and 1 result
				0x60, 0x04, i32, i32, i32, i32, 0x01, i32, // func=0x60 4 params and 1 result
			),
		},
		{
			name: "type and import section",
			input: &wasm.Module{
				TypeSection: []wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{f32, f32}, Results: []wasm.ValueType{f32}},
				},
				ImportSection: []wasm.Import{
					{
						Module: "Math", Name: "Mul",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					}, {
						Module: "Math", Name: "Add",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 0,
					},
				},
			},
			expected: append(append(Magic, version...),
				wasm.SectionIDType, 0x0d, // 13 bytes in this section
				0x02,                            // 2 types
				0x60, 0x02, i32, i32, 0x01, i32, // func=0x60 2 params and 1 result
				0x60, 0x02, f32, f32, 0x01, f32, // func=0x60 2 params and 1 result
				wasm.SectionIDImport, 0x17, // 23 bytes in this section
				0x02, // 2 imports
				0x04, 'M', 'a', 't', 'h', 0x03, 'M', 'u', 'l', wasm.ExternTypeFunc,
				0x01, // type index
				0x04, 'M', 'a', 't', 'h', 0x03, 'A', 'd', 'd', wasm.ExternTypeFunc,
				0x00, // type index
			),
		},
		{
			name: "type function and start section",
			input: &wasm.Module{
				TypeSection: []wasm.FunctionType{{}},
				ImportSection: []wasm.Import{{
					Module: "", Name: "hello",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
				StartSection: &zero,
			},
			expected: append(append(Magic, version...),
				wasm.SectionIDType, 0x04, // 4 bytes in this section
				0x01,           // 1 type
				0x60, 0x0, 0x0, // func=0x60 0 params and 0 result
				wasm.SectionIDImport, 0x0a, // 10 bytes in this section
				0x01, // 1 import
				0x00, 0x05, 'h', 'e', 'l', 'l', 'o', wasm.ExternTypeFunc,
				0x00, // type index
				wasm.SectionIDStart, 0x01,
				0x00, // start function index
			),
		},
		{
			name: "table and memory section",
			input: &wasm.Module{
				TableSection:  []wasm.Table{{Min: 3, Type: wasm.RefTypeFuncref}},
				MemorySection: &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true},
			},
			expected: append(append(Magic, version...),
				wasm.SectionIDTable, 0x04, // 4 bytes in this section
				0x01,                           // 1 table
				wasm.RefTypeFuncref, 0x0, 0x03, // func, only min: 3
				wasm.SectionIDMemory, 0x04, // 4 bytes in this section
				0x01,             // 1 memory
				0x01, 0x01, 0x01, // min and max = 1
			),
		},
		{
			name: "exported func with instructions",
			input: &wasm.Module{
				TypeSection: []wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
				},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{
					{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeI32Add, wasm.OpcodeEnd}},
				},
				ExportSection: []wasm.Export{
					{Name: "AddInt", Type: wasm.ExternTypeFunc, Index: wasm.Index(0)},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: wasm.Index(0), Name: "addInt"}},
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(0), NameMap: wasm.NameMap{
							{Index: wasm.Index(0), Name: "value_1"},
							{Index: wasm.Index(1), Name: "value_2"},
						}},
					},
				},
			},
			expected: append(append(Magic, version...),
				wasm.SectionIDType, 0x07, // 7 bytes in this section
				0x01,                            // 1 type
				0x60, 0x02, i32, i32, 0x01, i32, // func=0x60 2 params and 1 result
				wasm.SectionIDFunction, 0x02, // 2 bytes in this section
				0x01,                       // 1 function
				0x00,                       // func[0] type index 0
				wasm.SectionIDExport, 0x0a, // 10 bytes in this section
				0x01,                               // 1 export
				0x06, 'A', 'd', 'd', 'I', 'n', 't', // size of "AddInt", "AddInt"
				wasm.ExternTypeFunc, 0x00, // func[0]
				wasm.SectionIDCode, 0x09, // 9 bytes in this section
				0o1,                    // one code section
				0o7,                    // length of the body + locals
				0o0,                    // count of local blocks
				wasm.OpcodeLocalGet, 0, // local.get 0
				wasm.OpcodeLocalGet, 1, // local.get 1
				wasm.OpcodeI32Add,          // i32.add
				wasm.OpcodeEnd,             // end of instructions/code
				wasm.SectionIDCustom, 0x27, // 39 bytes in this section
				0x04, 'n', 'a', 'm', 'e',
				subsectionIDFunctionNames, 0x09, // 9 bytes
				0x01,                                     // two function names
				0x00, 0x06, 'a', 'd', 'd', 'I', 'n', 't', // index 0, size of "addInt", "addInt"
				subsectionIDLocalNames, 0x15, // 21 bytes
				0x01,       // one function
				0x00, 0x02, // index 0 has 2 locals
				0x00, 0x07, 'v', 'a', 'l', 'u', 'e', '_', '1', // index 0, size of "value_1", "value_1"
				0x01, 0x07, 'v', 'a', 'l', 'u', 'e', '_', '2', // index 1, size of "value_2", "value_2"
			),
		},
		{
			name: "exported global var",
			input: &wasm.Module{
				GlobalSection: []wasm.Global{
					{
						Type: wasm.GlobalType{ValType: i32, Mutable: true},
						Init: wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(0)},
					},
				},
				ExportSection: []wasm.Export{
					{Name: "sp", Type: wasm.ExternTypeGlobal, Index: wasm.Index(0)},
				},
			},
			expected: append(append(Magic, version...),
				wasm.SectionIDGlobal, 0x06, // 6 bytes in this section
				0x01, wasm.ValueTypeI32, 0x01, // 1 global i32 mutable
				wasm.OpcodeI32Const, 0x00, wasm.OpcodeEnd, // arbitrary init to zero
				wasm.SectionIDExport, 0x06, // 6 bytes in this section
				0x01,           // 1 export
				0x02, 's', 'p', // size of "sp", "sp"
				wasm.ExternTypeGlobal, 0x00, // global[0]
			),
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bytes := EncodeModule(tc.input)
			require.Equal(t, tc.expected, bytes)
		})
	}
}

func TestModule_Encode_HostFunctionSection_Unsupported(t *testing.T) {
	// We don't currently have an approach to serialize reflect.Value pointers
	fn := func() {}

	captured := require.CapturePanic(func() {
		EncodeModule(&wasm.Module{
			TypeSection: []wasm.FunctionType{{}},
			CodeSection: []wasm.Code{wasm.MustParseGoReflectFuncCode(fn)},
		})
	})
	require.EqualError(t, captured, "BUG: GoFunction is not encodable")
}
