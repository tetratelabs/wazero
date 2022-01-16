package wasm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeModule(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expected    *Module
		expectedErr string
	}{
		{
			name:     "empty",
			input:    []byte("\x00asm\x01\x00\x00\x00"),
			expected: &Module{CustomSections: map[string][]byte{}},
		},
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
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, e := DecodeModule(tc.input)
			if tc.expectedErr != "" {
				require.EqualError(t, e, tc.expectedErr)
			} else {
				require.NoError(t, e)
				require.Equal(t, tc.expected, m)
			}
		})
	}
}

func TestModule_Encode(t *testing.T) {
	tests := []struct {
		name     string
		input    *Module
		expected []byte
	}{
		{
			name:     "empty",
			input:    &Module{CustomSections: map[string][]byte{}},
			expected: append(magic, version...),
		},
		{
			name: "only name section",
			input: &Module{CustomSections: map[string][]byte{
				"name": {1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
			}},
			expected: append(append(magic, version...), SectionIDCustom,
				byte(1+4+10), // size prefixed "name", followed by the section data
				0x04, 'n', 'a', 'm', 'e',
				1, 2, 3, 4, 5, 6, 7, 8, 9, 0),
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
