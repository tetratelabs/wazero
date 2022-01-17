package wasm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModule_Encode(t *testing.T) {
	i32 := ValueTypeI32

	tests := []struct {
		name     string
		input    *Module
		expected []byte
	}{
		{
			name:     "empty",
			input:    &Module{},
			expected: append(magic, version...),
		},
		{
			name:  "only name section",
			input: &Module{NameSection: &NameSection{ModuleName: "simple"}},
			expected: append(append(magic, version...),
				SectionIDCustom, 0x0e, // 14 bytes in this section
				0x04, 'n', 'a', 'm', 'e',
				subsectionIDModuleName, 0x07, // 7 bytes in this subsection
				0x06, // the Module name simple is 6 bytes long
				's', 'i', 'm', 'p', 'l', 'e'),
		},
		{
			name: "only custom section",
			input: &Module{CustomSections: map[string][]byte{
				"meme": {1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
			}},
			expected: append(append(magic, version...),
				SectionIDCustom, 0xf, // 15 bytes in this section
				0x04, 'm', 'e', 'm', 'e',
				1, 2, 3, 4, 5, 6, 7, 8, 9, 0),
		},
		{
			name: "name section and a custom section", // name should encode last
			input: &Module{
				NameSection: &NameSection{ModuleName: "simple"},
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
			input: &Module{
				TypeSection: []*FunctionType{
					{},
					{Params: []ValueType{i32, i32}, Results: []ValueType{i32}},
					{Params: []ValueType{i32, i32, i32, i32}, Results: []ValueType{i32}},
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
			bytes := tc.input.Encode()
			require.Equal(t, tc.expected, bytes)
		})
	}
}

// TestDecodeModule relies on unit tests for Module.Encode, specifically that the encoding is both known and correct.
// This avoids having to copy/paste or share variables to assert against byte arrays.
func TestDecodeModule(t *testing.T) {
	tests := []struct {
		name  string
		input *Module // round trip test!
	}{
		{
			name:  "empty",
			input: &Module{},
		},
		{
			name:  "only name section",
			input: &Module{NameSection: &NameSection{ModuleName: "simple"}},
		},
		{
			name: "only custom section",
			input: &Module{CustomSections: map[string][]byte{
				"meme": {1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
			}},
		},
		{
			name: "name section and a custom section",
			input: &Module{
				NameSection: &NameSection{ModuleName: "simple"},
				CustomSections: map[string][]byte{
					"meme": {1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
				},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, e := DecodeModule(tc.input.Encode())
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
			expectedErr: "readSections failed: section ID 0: malformed custom section meme",
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
			expectedErr: "readSections failed: section ID 0: malformed custom section name",
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
