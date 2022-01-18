package binary

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestModule_Encode(t *testing.T) {
	i32 := wasm.ValueTypeI32

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
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bytes := EncodeModule(tc.input)
			require.Equal(t, tc.expected, bytes)
		})
	}
}
