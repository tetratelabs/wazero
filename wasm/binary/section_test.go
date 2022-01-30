package binary

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestEncodeStartSection(t *testing.T) {
	require.Equal(t, []byte{wasm.SectionIDStart, 0x01, 0x05}, encodeStartSection(5))
}

func TestDecodeExportSection(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected map[string]*wasm.Export
	}{
		{
			name: "empty and non-empty name",
			input: []byte{
				0x02,                      // 2 exports
				0x00,                      // Size of empty name
				wasm.ExportKindFunc, 0x02, // func[2]
				0x01, 'a', // Size of name, name
				wasm.ExportKindFunc, 0x01, // func[1]
			},
			expected: map[string]*wasm.Export{
				"":  {Name: "", Kind: wasm.ExportKindFunc, Index: wasm.Index(2)},
				"a": {Name: "a", Kind: wasm.ExportKindFunc, Index: wasm.Index(1)},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			exports, err := decodeExportSection(bytes.NewReader(tc.input))
			require.NoError(t, err)
			require.Equal(t, tc.expected, exports)
		})
	}
}

func TestDecodeExportSection_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectedErr string
	}{
		{
			name: "duplicates empty name",
			input: []byte{
				0x02,                      // 2 exports
				0x00,                      // Size of empty name
				wasm.ExportKindFunc, 0x00, // func[0]
				0x00,                      // Size of empty name
				wasm.ExportKindFunc, 0x00, // func[0]
			},
			expectedErr: "export[1] duplicates name \"\"",
		},
		{
			name: "duplicates name",
			input: []byte{
				0x02,      // 2 exports
				0x01, 'a', // Size of name, name
				wasm.ExportKindFunc, 0x00, // func[0]
				0x01, 'a', // Size of name, name
				wasm.ExportKindFunc, 0x00, // func[0]
			},
			expectedErr: "export[1] duplicates name \"a\"",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeExportSection(bytes.NewReader(tc.input))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
