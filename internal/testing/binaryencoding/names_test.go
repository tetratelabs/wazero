package binaryencoding

import (
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
			// e.g. (module $simple )
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
			name: "two function names", // e.g. TinyGo which at one point didn't set a module name
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
			bytes := EncodeNameSectionData(tc.input)
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
		6, 's', 'i', 'm', 'p', 'l', 'e',
	}, encodeNameSubsection(subsectionID, encodeSizePrefixed(name)))
}

func TestEncodeNameAssoc(t *testing.T) {
	na := wasm.NameAssoc{Index: 1, Name: "hello"}
	require.Equal(t, []byte{byte(na.Index), 5, 'h', 'e', 'l', 'l', 'o'}, encodeNameAssoc(na))
}

func TestEncodeNameMap(t *testing.T) {
	na := wasm.NameAssoc{Index: 1, Name: "hello"}
	m := wasm.NameMap{na}
	require.Equal(t, []byte{byte(1), byte(na.Index), 5, 'h', 'e', 'l', 'l', 'o'}, encodeNameMap(m))
}

func TestEncodeSizePrefixed(t *testing.T) {
	// We expect size in bytes (LEB128 encoded) then the bytes
	require.Equal(t, []byte{5, 'h', 'e', 'l', 'l', 'o'}, encodeSizePrefixed([]byte("hello")))
}
