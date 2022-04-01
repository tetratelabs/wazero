package binary

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestTableSection(t *testing.T) {
	three := uint32(3)
	tests := []struct {
		name     string
		input    []byte
		expected *wasm.Table
	}{
		{
			name: "min and min with max",
			input: []byte{
				0x01,                             // 1 table
				wasm.ElemTypeFuncref, 0x01, 2, 3, // (table 2 3)
			},
			expected: &wasm.Table{Min: 2, Max: &three},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			tables, err := decodeTableSection(bytes.NewReader(tc.input))
			require.NoError(t, err)
			require.Equal(t, tc.expected, tables)
		})
	}
}

func TestTableSection_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectedErr string
	}{
		{
			name: "min and min with max",
			input: []byte{
				0x02,                             // 2 tables
				wasm.ElemTypeFuncref, 0x00, 0x01, // (table 1)
				wasm.ElemTypeFuncref, 0x01, 0x02, 0x03, // (table 2 3)
			},
			expectedErr: "at most one table allowed in module, but read 2",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeTableSection(bytes.NewReader(tc.input))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestMemorySection(t *testing.T) {
	three := uint32(3)
	tests := []struct {
		name     string
		input    []byte
		expected *wasm.Memory
	}{
		{
			name: "min and min with max",
			input: []byte{
				0x01,             // 1 memory
				0x01, 0x02, 0x03, // (memory 2 3)
			},
			expected: &wasm.Memory{Min: 2, Max: three},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			memories, err := decodeMemorySection(bytes.NewReader(tc.input), wasm.MemoryMaxPages)
			require.NoError(t, err)
			require.Equal(t, tc.expected, memories)
		})
	}
}

func TestMemorySection_Errors(t *testing.T) {
	tests := []struct {
		name           string
		input          []byte
		memoryMaxPages uint32
		expectedErr    string
	}{
		{
			name: "min and min with max",
			input: []byte{
				0x02,       // 2 memories
				0x01,       // (memory 1)
				0x02, 0x03, // (memory 2 3)
			},
			expectedErr: "at most one memory allowed in module, but read 2",
		},
	}

	for _, tt := range tests {
		tc := tt

		if tc.memoryMaxPages == 0 {
			tc.memoryMaxPages = wasm.MemoryMaxPages
		}

		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeMemorySection(bytes.NewReader(tc.input), tc.memoryMaxPages)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
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
				wasm.ExternTypeFunc, 0x02, // func[2]
				0x01, 'a', // Size of name, name
				wasm.ExternTypeFunc, 0x01, // func[1]
			},
			expected: map[string]*wasm.Export{
				"":  {Name: "", Type: wasm.ExternTypeFunc, Index: wasm.Index(2)},
				"a": {Name: "a", Type: wasm.ExternTypeFunc, Index: wasm.Index(1)},
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
				wasm.ExternTypeFunc, 0x00, // func[0]
				0x00,                      // Size of empty name
				wasm.ExternTypeFunc, 0x00, // func[0]
			},
			expectedErr: "export[1] duplicates name \"\"",
		},
		{
			name: "duplicates name",
			input: []byte{
				0x02,      // 2 exports
				0x01, 'a', // Size of name, name
				wasm.ExternTypeFunc, 0x00, // func[0]
				0x01, 'a', // Size of name, name
				wasm.ExternTypeFunc, 0x00, // func[0]
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

func TestEncodeFunctionSection(t *testing.T) {
	require.Equal(t, []byte{wasm.SectionIDFunction, 0x2, 0x01, 0x05}, encodeFunctionSection([]wasm.Index{5}))
}

// TestEncodeStartSection uses the same index as TestEncodeFunctionSection to highlight the encoding is different.
func TestEncodeStartSection(t *testing.T) {
	require.Equal(t, []byte{wasm.SectionIDStart, 0x01, 0x05}, encodeStartSection(5))
}
