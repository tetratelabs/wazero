package binary

import (
	"bytes"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestTableSection(t *testing.T) {
	three := uint32(3)
	tests := []struct {
		name     string
		input    []byte
		expected []wasm.Table
	}{
		{
			name: "min and min with max",
			input: []byte{
				0x01,                            // 1 table
				wasm.RefTypeFuncref, 0x01, 2, 3, // (table 2 3)
			},
			expected: []wasm.Table{{Min: 2, Max: &three, Type: wasm.RefTypeFuncref}},
		},
		{
			name: "min and min with max - three tables",
			input: []byte{
				0x03,                            // 3 table
				wasm.RefTypeFuncref, 0x01, 2, 3, // (table 2 3)
				wasm.RefTypeExternref, 0x01, 2, 3, // (table 2 3)
				wasm.RefTypeFuncref, 0x01, 2, 3, // (table 2 3)
			},
			expected: []wasm.Table{
				{Min: 2, Max: &three, Type: wasm.RefTypeFuncref},
				{Min: 2, Max: &three, Type: wasm.RefTypeExternref},
				{Min: 2, Max: &three, Type: wasm.RefTypeFuncref},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			tables, err := decodeTableSection(bytes.NewReader(tc.input), api.CoreFeatureReferenceTypes)
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
		features    api.CoreFeatures
	}{
		{
			name: "min and min with max",
			input: []byte{
				0x02,                            // 2 tables
				wasm.RefTypeFuncref, 0x00, 0x01, // (table 1)
				wasm.RefTypeFuncref, 0x01, 0x02, 0x03, // (table 2 3)
			},
			expectedErr: "at most one table allowed in module as feature \"reference-types\" is disabled",
			features:    api.CoreFeaturesV1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeTableSection(bytes.NewReader(tc.input), tc.features)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestMemorySection(t *testing.T) {
	max := wasm.MemoryLimitPages

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
			expected: &wasm.Memory{Min: 2, Cap: 2, Max: three, IsMaxEncoded: true},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			memories, err := decodeMemorySection(bytes.NewReader(tc.input), api.CoreFeaturesV2, newMemorySizer(max, false), max)
			require.NoError(t, err)
			require.Equal(t, tc.expected, memories)
		})
	}
}

func TestMemorySection_Errors(t *testing.T) {
	max := wasm.MemoryLimitPages

	tests := []struct {
		name        string
		input       []byte
		expectedErr string
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

		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeMemorySection(bytes.NewReader(tc.input), api.CoreFeaturesV2, newMemorySizer(max, false), max)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestDecodeExportSection(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []wasm.Export
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
			expected: []wasm.Export{
				{Name: "", Type: wasm.ExternTypeFunc, Index: wasm.Index(2)},
				{Name: "a", Type: wasm.ExternTypeFunc, Index: wasm.Index(1)},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			actual, actualExpMap, err := decodeExportSection(bytes.NewReader(tc.input))
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)

			expMap := make(map[string]*wasm.Export, len(tc.expected))
			for i := range tc.expected {
				exp := &tc.expected[i]
				expMap[exp.Name] = exp
			}
			require.Equal(t, expMap, actualExpMap)
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
			_, _, err := decodeExportSection(bytes.NewReader(tc.input))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestEncodeFunctionSection(t *testing.T) {
	require.Equal(t, []byte{wasm.SectionIDFunction, 0x2, 0x01, 0x05}, binaryencoding.EncodeFunctionSection([]wasm.Index{5}))
}

// TestEncodeStartSection uses the same index as TestEncodeFunctionSection to highlight the encoding is different.
func TestEncodeStartSection(t *testing.T) {
	require.Equal(t, []byte{wasm.SectionIDStart, 0x01, 0x05}, binaryencoding.EncodeStartSection(5))
}

func TestDecodeDataCountSection(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		v, err := decodeDataCountSection(bytes.NewReader([]byte{0x1}))
		require.NoError(t, err)
		require.Equal(t, uint32(1), *v)
	})
	t.Run("eof", func(t *testing.T) {
		// EOF is fine as the datacount is optional.
		_, err := decodeDataCountSection(bytes.NewReader([]byte{}))
		require.NoError(t, err)
	})
}
