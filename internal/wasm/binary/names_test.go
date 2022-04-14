package binary

import (
	"bytes"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestEncodeNameSectionData(t *testing.T) {
	tests := []struct {
		name     string
		input    *wasm.NameSection
		expected []byte
	}{
		{
			name:  "empty",
			input: &wasm.NameSection{},
		},
		{
			name: "only module",
			// Ex. (module $simple )
			input: &wasm.NameSection{ModuleName: "simple"},
			expected: []byte{
				subsectionIDModuleName, 0x07, // 7 bytes
				0x06, // the Module name simple is 6 bytes long
				's', 'i', 'm', 'p', 'l', 'e',
			},
		},
		{
			name: "module and function name",
			//	(module $simple
			//		(import "" "Hello" (func $hello))
			//		(start $hello)
			//	)
			input: &wasm.NameSection{
				ModuleName:    "simple",
				FunctionNames: wasm.NameMap{{Index: wasm.Index(0), Name: "hello"}},
			},
			expected: []byte{
				subsectionIDModuleName, 0x07, // 7 bytes
				0x06, // the Module name simple is 6 bytes long
				's', 'i', 'm', 'p', 'l', 'e',
				subsectionIDFunctionNames, 0x08, // 8 bytes
				0x01, // one function name
				0x00, // the function index is zero
				0x05, // the function name hello is 5 bytes long
				'h', 'e', 'l', 'l', 'o',
			},
		},
		{
			name: "two function names", // Ex. TinyGo which at one point didn't set a module name
			//	(module
			//		(import "wasi_snapshot_preview1" "args_sizes_get" (func $wasi.args_sizes_get (param i32, i32) (result i32)))
			//		(import "wasi_snapshot_preview1" "fd_write" (func $wasi.fd_write (param i32, i32, i32, i32) (result i32)))
			//	)
			input: &wasm.NameSection{
				FunctionNames: wasm.NameMap{
					{Index: wasm.Index(0), Name: "wasi.args_sizes_get"},
					{Index: wasm.Index(1), Name: "wasi.fd_write"},
				},
			},
			expected: []byte{
				subsectionIDFunctionNames, 0x25, // 37 bytes
				0x02, // two function names
				0x00, // the function index is zero
				0x13, // the function name wasi.args_sizes_get is 19 bytes long
				'w', 'a', 's', 'i', '.', 'a', 'r', 'g', 's', '_', 's', 'i', 'z', 'e', 's', '_', 'g', 'e', 't',
				0x01, // the function index is one
				0x0d, // the function name wasi.fd_write is 13 bytes long
				'w', 'a', 's', 'i', '.', 'f', 'd', '_', 'w', 'r', 'i', 't', 'e',
			},
		},
		{
			name: "function with local names",
			//	(module
			//		(import "Math" "Mul" (func $mul (param $x f32) (param $y f32) (result f32)))
			//		(import "Math" "Add" (func $add (param $l f32) (param $r f32) (result f32)))
			//	)
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
			expected: []byte{
				subsectionIDFunctionNames, 0x0b, // 7 bytes
				0x02,                      // two function names
				0x00, 0x03, 'm', 'u', 'l', // index 0, size of "mul", "mul"
				0x01, 0x03, 'a', 'd', 'd', // index 1, size of "add", "add"
				subsectionIDLocalNames, 0x11, // 17 bytes
				0x02,       // two functions
				0x00, 0x02, // index 0 has 2 locals
				0x00, 0x01, 'x', // index 0, size of "x", "x"
				0x01, 0x01, 'y', // index 1, size of "y", "y"
				0x01, 0x02, // index 1 has 2 locals
				0x00, 0x01, 'l', // index 0, size of "l", "l"
				0x01, 0x01, 'r', // index 1, size of "r", "r"
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bytes := encodeNameSectionData(tc.input)
			require.Equal(t, tc.expected, bytes)
		})
	}
}

func TestEncodeNameSubsection(t *testing.T) {
	subsectionID := uint8(1)
	name := []byte("simple")
	require.Equal(t, []byte{
		subsectionID,
		byte(1 + 6), // 1 is the size of 6 in LEB128 encoding
		6, 's', 'i', 'm', 'p', 'l', 'e'}, encodeNameSubsection(subsectionID, encodeSizePrefixed(name)))
}

func TestEncodeNameAssoc(t *testing.T) {
	na := &wasm.NameAssoc{Index: 1, Name: "hello"}
	require.Equal(t, []byte{byte(na.Index), 5, 'h', 'e', 'l', 'l', 'o'}, encodeNameAssoc(na))
}

func TestEncodeNameMap(t *testing.T) {
	na := &wasm.NameAssoc{Index: 1, Name: "hello"}
	m := wasm.NameMap{na}
	require.Equal(t, []byte{byte(1), byte(na.Index), 5, 'h', 'e', 'l', 'l', 'o'}, encodeNameMap(m))
}

func TestEncodeSizePrefixed(t *testing.T) {
	// We expect size in bytes (LEB128 encoded) then the bytes
	require.Equal(t, []byte{5, 'h', 'e', 'l', 'l', 'o'}, encodeSizePrefixed([]byte("hello")))
}

// TestDecodeNameSection relies on unit tests for NameSection.EncodeData, specifically that the encoding is
// both known and correct. This avoids having to copy/paste or share variables to assert against byte arrays.
func TestDecodeNameSection(t *testing.T) {
	tests := []struct {
		name  string
		input *wasm.NameSection // round trip test!
	}{{
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
			data := encodeNameSectionData(tc.input)
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
