package util

import (
	"fmt"
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

func TestResolvePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cwd, path string
		expected  string
	}{
		{cwd: "/", path: ".", expected: "/"},
		{cwd: "/", path: "/", expected: "/"},
		{cwd: "/", path: "..", expected: "/"},
		{cwd: "/", path: "a", expected: "/a"},
		{cwd: "/", path: "/a", expected: "/a"},
		{cwd: "/", path: "./a/", expected: "/a/"}, // retain trailing slash
		{cwd: "/", path: "./a/.", expected: "/a"},
		{cwd: "/", path: "a/.", expected: "/a"},
		{cwd: "/a", path: "/..", expected: "/"},
		{cwd: "/a", path: "/", expected: "/"},
		{cwd: "/a", path: "b", expected: "/a/b"},
		{cwd: "/a", path: "/b", expected: "/b"},
		{cwd: "/a", path: "/b/", expected: "/b/"}, // retain trailing slash
		{cwd: "/a", path: "./b/.", expected: "/a/b"},
		{cwd: "/a/b", path: ".", expected: "/a/b"},
		{cwd: "/a/b", path: "../.", expected: "/a"},
		{cwd: "/a/b", path: "../..", expected: "/"},
		{cwd: "/a/b", path: "../../..", expected: "/"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(fmt.Sprintf("%s,%s", tc.cwd, tc.path), func(t *testing.T) {
			require.Equal(t, tc.expected, ResolvePath(tc.cwd, tc.path))
		})
	}
}
