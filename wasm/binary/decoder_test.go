package binary

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

// TestDecodeModule relies on unit tests for Module.Encode, specifically that the encoding is both known and correct.
// This avoids having to copy/paste or share variables to assert against byte arrays.
func TestDecodeModule(t *testing.T) {
	i32, f32 := wasm.ValueTypeI32, wasm.ValueTypeF32

	tests := []struct {
		name  string
		input *wasm.Module // round trip test!
	}{
		{
			name:  "empty",
			input: &wasm.Module{},
		},
		{
			name:  "only name section",
			input: &wasm.Module{NameSection: &wasm.NameSection{ModuleName: "simple"}},
		},
		{
			name: "only custom section",
			input: &wasm.Module{CustomSections: map[string][]byte{
				"meme": {1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
			}},
		},
		{
			name: "name section and a custom section",
			input: &wasm.Module{
				NameSection: &wasm.NameSection{ModuleName: "simple"},
				CustomSections: map[string][]byte{
					"meme": {1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
				},
			},
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
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, e := DecodeModule(EncodeModule(tc.input))
			require.NoError(t, e)
			require.Equal(t, tc.input, m)
		})
	}
}

func TestDecodeModule_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectedErr string
	}{
		{
			name:        "wrong magic",
			input:       []byte("wasm\x01\x00\x00\x00"),
			expectedErr: "invalid magic number",
		},
		{
			name:        "wrong version",
			input:       []byte("\x00asm\x01\x00\x00\x01"),
			expectedErr: "invalid version header",
		},
		{
			name: "redundant custom section",
			input: append(append(magic, version...),
				SectionIDCustom, 0x09, // 9 bytes in this section
				0x04, 'm', 'e', 'm', 'e',
				subsectionIDModuleName, 0x03, 0x01, 'x',
				SectionIDCustom, 0x09, // 9 bytes in this section
				0x04, 'm', 'e', 'm', 'e',
				subsectionIDModuleName, 0x03, 0x01, 'y'),
			expectedErr: "section ID 0: redundant custom section meme",
		},
		{
			name: "redundant name section",
			input: append(append(magic, version...),
				SectionIDCustom, 0x09, // 9 bytes in this section
				0x04, 'n', 'a', 'm', 'e',
				subsectionIDModuleName, 0x03, 0x01, 'x',
				SectionIDCustom, 0x09, // 9 bytes in this section
				0x04, 'n', 'a', 'm', 'e',
				subsectionIDModuleName, 0x03, 0x01, 'x'),
			expectedErr: "section ID 0: redundant custom section name",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, e := DecodeModule(tc.input)
			require.EqualError(t, e, tc.expectedErr)
		})
	}
}
