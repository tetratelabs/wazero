package util

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/gojs/custom"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestMustRead(t *testing.T) {
	mem := &wasm.MemoryInstance{Buffer: []byte{1, 2, 3, 4, 5, 6, 7, 8}, Min: 1}

	tests := []struct {
		name              string
		funcName          string
		paramIdx          int
		offset, byteCount uint32
		expected          []byte
		expectedPanic     string
	}{
		{
			name: "read nothing",
		},
		{
			name:      "read all",
			offset:    0,
			byteCount: 8,
			expected:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name:      "read some",
			offset:    4,
			byteCount: 2,
			expected:  []byte{5, 6},
		},
		{
			name:          "read too many",
			funcName:      custom.NameSyscallCopyBytesToGo,
			offset:        4,
			byteCount:     5,
			expectedPanic: "out of memory reading dst",
		},
		{
			name:          "read too many - function not in names",
			funcName:      "not_in_names",
			offset:        4,
			byteCount:     5,
			expectedPanic: "out of memory reading not_in_names param[0]",
		},
		{
			name:          "read too many - in names, but no params",
			funcName:      custom.NameDebug,
			offset:        4,
			byteCount:     5,
			expectedPanic: "out of memory reading debug param[0]",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			if tc.expectedPanic != "" {
				err := require.CapturePanic(func() {
					MustRead(mem, tc.funcName, tc.paramIdx, tc.offset, tc.byteCount)
				})
				require.EqualError(t, err, tc.expectedPanic)
			} else {
				buf := MustRead(mem, tc.funcName, tc.paramIdx, tc.offset, tc.byteCount)
				require.Equal(t, tc.expected, buf)
			}
		})
	}
}
