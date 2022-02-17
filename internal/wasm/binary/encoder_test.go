package binary

import (
	"testing"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	wasm2 "github.com/tetratelabs/wazero/wasm"
)

func TestModule_Encode(t *testing.T) {
	i32, f32 := wasm2.ValueTypeI32, wasm2.ValueTypeF32
	zero := uint32(0)

	tests := []struct {
		name     string
		input    *wasm.Module
		expected []byte
	}{
		{
			name:     "empty",
			input:    &wasm.Module{},
			expected: append(magic, version...),
		},
		{
			name:  "only name section",
			input: &wasm.Module{NameSection: &wasm.NameSection{ModuleName: "simple"}},
			expected: append(append(magic, version...),
				wasm2.SectionIDCustom, 0x0e, // 14 bytes in this section
				0x04, 'n', 'a', 'm', 'e',
				subsectionIDModuleName, 0x07, // 7 bytes in this subsection
				0x06, // the Module name simple is 6 bytes long
				's', 'i', 'm', 'p', 'l', 'e'),
		},
		{
			name: "type section",
			input: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{},
					{Params: []wasm2.ValueType{i32, i32}, Results: []wasm2.ValueType{i32}},
					{Params: []wasm2.ValueType{i32, i32, i32, i32}, Results: []wasm2.ValueType{i32}},
				},
			},
			expected: append(append(magic, version...),
				wasm2.SectionIDType, 0x12, // 18 bytes in this section
				0x03,             // 3 types
				0x60, 0x00, 0x00, // func=0x60 no param no result
				0x60, 0x02, i32, i32, 0x01, i32, // func=0x60 2 params and 1 result
				0x60, 0x04, i32, i32, i32, i32, 0x01, i32, // func=0x60 4 params and 1 result
			),
		},
		{
			name: "type and import section",
			input: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []wasm2.ValueType{i32, i32}, Results: []wasm2.ValueType{i32}},
					{Params: []wasm2.ValueType{f32, f32}, Results: []wasm2.ValueType{f32}},
				},
				ImportSection: []*wasm.Import{
					{
						Module: "Math", Name: "Mul",
						Kind:     wasm2.ImportKindFunc,
						DescFunc: 1,
					}, {
						Module: "Math", Name: "Add",
						Kind:     wasm2.ImportKindFunc,
						DescFunc: 0,
					},
				},
			},
			expected: append(append(magic, version...),
				wasm2.SectionIDType, 0x0d, // 13 bytes in this section
				0x02,                            // 2 types
				0x60, 0x02, i32, i32, 0x01, i32, // func=0x60 2 params and 1 result
				0x60, 0x02, f32, f32, 0x01, f32, // func=0x60 2 params and 1 result
				wasm2.SectionIDImport, 0x17, // 23 bytes in this section
				0x02, // 2 imports
				0x04, 'M', 'a', 't', 'h', 0x03, 'M', 'u', 'l', wasm2.ImportKindFunc,
				0x01, // type index
				0x04, 'M', 'a', 't', 'h', 0x03, 'A', 'd', 'd', wasm2.ImportKindFunc,
				0x00, // type index
			),
		},
		{
			name: "type function and start section",
			input: &wasm.Module{
				TypeSection: []*wasm.FunctionType{{}},
				ImportSection: []*wasm.Import{{
					Module: "", Name: "hello",
					Kind:     wasm2.ImportKindFunc,
					DescFunc: 0,
				}},
				StartSection: &zero,
			},
			expected: append(append(magic, version...),
				wasm2.SectionIDType, 0x04, // 4 bytes in this section
				0x01,           // 1 type
				0x60, 0x0, 0x0, // func=0x60 0 params and 0 result
				wasm2.SectionIDImport, 0x0a, // 10 bytes in this section
				0x01, // 1 import
				0x00, 0x05, 'h', 'e', 'l', 'l', 'o', wasm2.ImportKindFunc,
				0x00, // type index
				wasm2.SectionIDStart, 0x01,
				0x00, // start function index
			),
		},
		{
			name: "exported func with instructions",
			input: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []wasm2.ValueType{i32, i32}, Results: []wasm2.ValueType{i32}},
				},
				FunctionSection: []wasm.Index{0},
				CodeSection: []*wasm.Code{
					{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeI32Add, wasm.OpcodeEnd}},
				},
				ExportSection: map[string]*wasm.Export{
					"AddInt": {Name: "AddInt", Kind: wasm2.ExportKindFunc, Index: wasm.Index(0)},
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
			expected: append(append(magic, version...),
				wasm2.SectionIDType, 0x07, // 7 bytes in this section
				0x01,                            // 1 type
				0x60, 0x02, i32, i32, 0x01, i32, // func=0x60 2 params and 1 result
				wasm2.SectionIDFunction, 0x02, // 2 bytes in this section
				0x01,                        // 1 function
				0x00,                        // func[0] type index 0
				wasm2.SectionIDExport, 0x0a, // 10 bytes in this section
				0x01,                               // 1 export
				0x06, 'A', 'd', 'd', 'I', 'n', 't', // size of "AddInt", "AddInt"
				wasm2.ExportKindFunc, 0x00, // func[0]
				wasm2.SectionIDCode, 0x09, // 9 bytes in this section
				01,                     // one code section
				07,                     // length of the body + locals
				00,                     // count of local blocks
				wasm.OpcodeLocalGet, 0, // local.get 0
				wasm.OpcodeLocalGet, 1, // local.get 1
				wasm.OpcodeI32Add,           // i32.add
				wasm.OpcodeEnd,              // end of instructions/code
				wasm2.SectionIDCustom, 0x27, // 39 bytes in this section
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
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bytes := EncodeModule(tc.input)
			require.Equal(t, tc.expected, bytes)
		})
	}
}
