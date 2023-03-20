package platform

import (
	"os"
	"path"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_IsTerminal(t *testing.T) {
	if !CompilerSupported() {
		t.Skip() // because it will always return false
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(path.Join(dir, "foo"), nil, 0o400))
	file, err := os.Open(path.Join(dir, "foo"))
	require.NoError(t, err)
	defer file.Close()

	// We aren't guaranteed to have a terminal device for os.Stdout, due to how
	// `go test` forks processes. Instead, we test if this is consistent. For
	// example, when run in a debugger, this could end up true.
	stdioIsTTY := IsTerminal(os.Stdout.Fd())

	tests := []struct {
		name     string
		file     *os.File
		expected bool
	}{
		{name: "Stdin", file: os.Stdin, expected: stdioIsTTY},
		{name: "Stdout", file: os.Stdout, expected: stdioIsTTY},
		{name: "Stderr", file: os.Stderr, expected: stdioIsTTY},
		{name: "File", file: file, expected: false},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, IsTerminal(tc.file.Fd()))
		})
	}
}

func Test_IsTerminalFd(t *testing.T) {
	stdioIsTTY := IsTerminal(os.Stdout.Fd())

	tests := []struct {
		name     string
		fd       uintptr
		expected bool
	}{
		{name: "Stdin", fd: 0, expected: stdioIsTTY},
		{name: "Stdout", fd: 1, expected: stdioIsTTY},
		{name: "Stderr", fd: 2, expected: stdioIsTTY},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, IsTerminal(tc.fd))
		})
	}
}
