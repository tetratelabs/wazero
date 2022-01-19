package binary

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
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
			expected: append(magic, version...),
		},
		{
			name:  "only name section",
			input: &wasm.Module{NameSection: &wasm.NameSection{ModuleName: "simple"}},
			expected: append(append(magic, version...),
				SectionIDCustom, 0x0e, // 14 bytes in this section
				0x04, 'n', 'a', 'm', 'e',
				subsectionIDModuleName, 0x07, // 7 bytes in this subsection
				0x06, // the Module name simple is 6 bytes long
				's', 'i', 'm', 'p', 'l', 'e'),
		},
		{
			name: "only custom section",
			input: &wasm.Module{CustomSections: map[string][]byte{
				"meme": {1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
			}},
			expected: append(append(magic, version...),
				SectionIDCustom, 0xf, // 15 bytes in this section
				0x04, 'm', 'e', 'm', 'e',
				1, 2, 3, 4, 5, 6, 7, 8, 9, 0),
		},
		{
			name: "name section and a custom section", // name should encode last
			input: &wasm.Module{
				NameSection: &wasm.NameSection{ModuleName: "simple"},
				CustomSections: map[string][]byte{
					"meme": {1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
				},
			},
			expected: append(append(magic, version...),
				SectionIDCustom, 0xf, // 15 bytes in this section
				0x04, 'm', 'e', 'm', 'e',
				1, 2, 3, 4, 5, 6, 7, 8, 9, 0,
				SectionIDCustom, 0x0e, // 14 bytes in this section
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
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
			},
			expected: append(append(magic, version...),
				SectionIDType, 0x12, // 18 bytes in this section
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
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{f32, f32}, Results: []wasm.ValueType{f32}},
				},
				ImportSection: []*wasm.Import{
					{
						Module: "Math", Name: "Mul",
						Kind:     wasm.ImportKindFunc,
						DescFunc: 1,
					}, {
						Module: "Math", Name: "Add",
						Kind:     wasm.ImportKindFunc,
						DescFunc: 0,
					},
				},
			},
			expected: append(append(magic, version...),
				SectionIDType, 0x0d, // 13 bytes in this section
				0x02,                            // 2 types
				0x60, 0x02, i32, i32, 0x01, i32, // func=0x60 2 params and 1 result
				0x60, 0x02, f32, f32, 0x01, f32, // func=0x60 2 params and 1 result
				SectionIDImport, 0x17, // 23 bytes in this section
				0x02, // 2 imports
				0x04, 'M', 'a', 't', 'h', 0x03, 'M', 'u', 'l', wasm.ImportKindFunc,
				0x01, // type index
				0x04, 'M', 'a', 't', 'h', 0x03, 'A', 'd', 'd', wasm.ImportKindFunc,
				0x00, // type index
			),
		},
		{
			name: "type function and start section",
			input: &wasm.Module{
				TypeSection: []*wasm.FunctionType{{}},
				ImportSection: []*wasm.Import{{
					Module: "", Name: "hello",
					Kind:     wasm.ImportKindFunc,
					DescFunc: 0,
				}},
				StartSection: &zero,
			},
			expected: append(append(magic, version...),
				SectionIDType, 0x04, // 4 bytes in this section
				0x01,           // 1 type
				0x60, 0x0, 0x0, // func=0x60 0 params and 0 result
				SectionIDImport, 0x0a, // 10 bytes in this section
				0x01, // 1 import
				0x00, 0x05, 'h', 'e', 'l', 'l', 'o', wasm.ImportKindFunc,
				0x00, // type index
				SectionIDStart, 0x01,
				0x00, // start function index
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
