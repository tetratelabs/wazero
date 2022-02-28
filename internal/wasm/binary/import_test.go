package binary

import (
	"testing"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
)

func TestEncodeImport(t *testing.T) {
	tests := []struct {
		name     string
		input    *wasm.Import
		expected []byte
	}{
		{
			name: "func no module, no name, type index 0",
			input: &wasm.Import{ // Ex. (import "" "" (func (type 0)))
				Type:     wasm.ExternTypeFunc,
				Module:   "",
				Name:     "",
				DescFunc: 0,
			},
			expected: []byte{wasm.ExternTypeFunc, 0x00, 0x00, 0x00},
		},
		{
			name: "func module, no name, type index 0",
			input: &wasm.Import{ // Ex. (import "$test" "" (func (type 0)))
				Type:     wasm.ExternTypeFunc,
				Module:   "test",
				Name:     "",
				DescFunc: 0,
			},
			expected: []byte{
				0x04, 't', 'e', 's', 't',
				0x00,
				wasm.ExternTypeFunc,
				0x00,
			},
		},
		{
			name: "func module, name, type index 0",
			input: &wasm.Import{ // Ex. (import "$math" "$pi" (func (type 0)))
				Type:     wasm.ExternTypeFunc,
				Module:   "math",
				Name:     "pi",
				DescFunc: 0,
			},
			expected: []byte{
				0x04, 'm', 'a', 't', 'h',
				0x02, 'p', 'i',
				wasm.ExternTypeFunc,
				0x00,
			},
		},
		{
			name: "func module, name, type index 10",
			input: &wasm.Import{ // Ex. (import "$math" "$pi" (func (type 10)))
				Type:     wasm.ExternTypeFunc,
				Module:   "math",
				Name:     "pi",
				DescFunc: 10,
			},
			expected: []byte{
				0x04, 'm', 'a', 't', 'h',
				0x02, 'p', 'i',
				wasm.ExternTypeFunc,
				0x0a,
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bytes := encodeImport(tc.input)
			require.Equal(t, tc.expected, bytes)
		})
	}
}
