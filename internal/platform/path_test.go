package platform

import (
	"runtime"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestToPosixPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{},
		{input: ".", expected: "."},
		{input: "/", expected: `/`},
		{input: `/`, expected: `/`},
		{input: "/foo/bar", expected: `/foo/bar`},
		{input: `\foo\bar`, expected: `/foo/bar`},
		{input: "foo/bar", expected: `foo/bar`},
		{input: `foo\bar`, expected: `foo/bar`},
		{input: "c:/foo/bar", expected: `c:/foo/bar`},
		{input: `c:\foo\bar`, expected: `c:/foo/bar`},
	}

	for _, tt := range tests {
		tc := tt

		// We don't expect to translate backslashes unless we are on windows.
		if strings.IndexByte(tc.input, '\\') != -1 && runtime.GOOS != "windows" {
			continue
		}

		t.Run(tc.input, func(t *testing.T) {
			require.Equal(t, tc.expected, ToPosixPath(tc.input))
		})
	}
}
