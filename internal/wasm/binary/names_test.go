package binary

import (
	"bytes"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// TestDecodeNameSection relies on unit tests for NameSection.EncodeData, specifically that the encoding is
// both known and correct. This avoids having to copy/paste or share variables to assert against byte arrays.
func TestDecodeNameSection(t *testing.T) {
	tests := []struct {
		name  string
		input *wasm.NameSection // round trip test!
	}{
		{
			name:  "empty",
			input: &wasm.NameSection{},
		},
		{
			name:  "only module",
			input: &wasm.NameSection{ModuleName: "simple"},
		},
		{
			name: "module and function name",
			input: &wasm.NameSection{
				ModuleName:    "simple",
				FunctionNames: wasm.NameMap{{Index: wasm.Index(0), Name: "wasi.hello"}},
			},
		},
		{
			name: "two function names",
			input: &wasm.NameSection{
				FunctionNames: wasm.NameMap{
					{Index: wasm.Index(0), Name: "wasi.args_sizes_get"},
					{Index: wasm.Index(1), Name: "wasi.fd_write"},
				},
			},
		},
		{
			name: "function with local names",
			input: &wasm.NameSection{
				FunctionNames: wasm.NameMap{
					{Index: wasm.Index(0), Name: "mul"},
					{Index: wasm.Index(1), Name: "add"},
				},
				LocalNames: wasm.IndirectNameMap{
					{Index: wasm.Index(0), NameMap: wasm.NameMap{
						{Index: wasm.Index(0), Name: "x"},
						{Index: wasm.Index(1), Name: "y"},
					}},
					{Index: wasm.Index(1), NameMap: wasm.NameMap{
						{Index: wasm.Index(0), Name: "l"},
						{Index: wasm.Index(1), Name: "r"},
					}},
				},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			data := binaryencoding.EncodeNameSectionData(tc.input)
			ns, err := decodeNameSection(bytes.NewReader(data), uint64(len(data)))
			require.NoError(t, err)
			require.Equal(t, tc.input, ns)
		})
	}
}

func TestDecodeNameSection_Errors(t *testing.T) {
	// currently, we ignore the size of known subsections
	ignoredSubsectionSize := byte(50)
	tests := []struct {
		name        string
		input       []byte
		expectedErr string
	}{
		{
			name:        "EOF after module name subsection ID",
			input:       []byte{subsectionIDModuleName},
			expectedErr: "failed to read the size of subsection[0]: EOF",
		},
		{
			name:        "EOF after function names subsection ID",
			input:       []byte{subsectionIDFunctionNames},
			expectedErr: "failed to read the size of subsection[1]: EOF",
		},
		{
			name:        "EOF after local names subsection ID",
			input:       []byte{subsectionIDLocalNames},
			expectedErr: "failed to read the size of subsection[2]: EOF",
		},
		{
			name:        "EOF after unknown subsection ID",
			input:       []byte{4},
			expectedErr: "failed to read the size of subsection[4]: EOF",
		},
		{
			name:        "EOF after module name subsection size",
			input:       []byte{subsectionIDModuleName, ignoredSubsectionSize},
			expectedErr: "failed to read module name size: EOF",
		},
		{
			name:        "EOF after function names subsection size",
			input:       []byte{subsectionIDFunctionNames, ignoredSubsectionSize},
			expectedErr: "failed to read the function count of subsection[1]: EOF",
		},
		{
			name:        "EOF after local names subsection size",
			input:       []byte{subsectionIDLocalNames, ignoredSubsectionSize},
			expectedErr: "failed to read the function count of subsection[2]: EOF",
		},
		{
			name:        "EOF skipping unknown subsection size",
			input:       []byte{4, 100},
			expectedErr: "failed to skip subsection[4]: EOF",
		},
		{
			name:        "EOF after module name size",
			input:       []byte{subsectionIDModuleName, ignoredSubsectionSize, 5},
			expectedErr: "failed to read module name: EOF",
		},
		{
			name:        "EOF after function name count",
			input:       []byte{subsectionIDFunctionNames, ignoredSubsectionSize, 2},
			expectedErr: "failed to read a function index in subsection[1]: EOF",
		},
		{
			name:        "EOF after local names function count",
			input:       []byte{subsectionIDLocalNames, ignoredSubsectionSize, 2},
			expectedErr: "failed to read a function index in subsection[2]: EOF",
		},
		{
			name:        "EOF after function name index",
			input:       []byte{subsectionIDFunctionNames, ignoredSubsectionSize, 2, 0},
			expectedErr: "failed to read function[0] name size: EOF",
		},
		{
			name:        "EOF after local names function index",
			input:       []byte{subsectionIDLocalNames, ignoredSubsectionSize, 2, 0},
			expectedErr: "failed to read the local count for function[0]: EOF",
		},
		{
			name:        "EOF after function name size",
			input:       []byte{subsectionIDFunctionNames, ignoredSubsectionSize, 2, 0, 5},
			expectedErr: "failed to read function[0] name: EOF",
		},
		{
			name:        "EOF after local names count for a function index",
			input:       []byte{subsectionIDLocalNames, ignoredSubsectionSize, 2, 0, 2},
			expectedErr: "failed to read a local index of function[0]: EOF",
		},
		{
			name:        "EOF after local name size",
			input:       []byte{subsectionIDLocalNames, ignoredSubsectionSize, 2, 0, 2, 1},
			expectedErr: "failed to read function[0] local[1] name size: EOF",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeNameSection(bytes.NewReader(tc.input), uint64(len(tc.input)))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
