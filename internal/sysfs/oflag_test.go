package sysfs

import (
	"os"
	"testing"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_toOsOpenFlag doesn't use subtests to reduce volume of verbose output,
// and in recognition we have tens of thousands of tests, which can hit IDE
// limits.
func Test_toOsOpenFlag(t *testing.T) {
	tests := []struct {
		name     string
		flag     fsapi.Oflag
		expected int
	}{
		{name: "O_RDONLY", flag: fsapi.O_RDONLY, expected: os.O_RDONLY},
		{name: "O_RDWR", flag: fsapi.O_RDWR, expected: os.O_RDWR},
		{name: "O_WRONLY", flag: fsapi.O_WRONLY, expected: os.O_WRONLY},
		{name: "O_CREAT", flag: fsapi.O_CREAT, expected: os.O_RDONLY | os.O_CREATE},
		{name: "O_APPEND", flag: fsapi.O_APPEND, expected: os.O_RDONLY | os.O_APPEND},
		{
			name:     "all portable",
			flag:     fsapi.O_RDWR | fsapi.O_APPEND | fsapi.O_CREAT | fsapi.O_EXCL | fsapi.O_SYNC | fsapi.O_TRUNC,
			expected: os.O_RDWR | os.O_APPEND | os.O_CREATE | os.O_EXCL | os.O_SYNC | os.O_TRUNC,
		},
		{name: "undefined", flag: 1 << 15, expected: os.O_RDONLY},
	}

	for _, tc := range tests {
		require.Equal(t, tc.expected, toOsOpenFlag(tc.flag), tc.name)
	}

	// Tests any supported syscall flags
	for n, f := range map[string]fsapi.Oflag{
		"O_DIRECTORY": fsapi.O_DIRECTORY,
		"O_DSYNC":     fsapi.O_DSYNC,
		"O_NOFOLLOW":  fsapi.O_NOFOLLOW,
		"O_NONBLOCK":  fsapi.O_NONBLOCK,
		"O_RSYNC":     fsapi.O_RSYNC,
	} {
		if supportedSyscallOflag&f == 0 {
			continue
		}
		require.NotEqual(t, 0, toOsOpenFlag(f), n)
	}

	// Example of a flag that can be or'd into O_RDONLY even if not
	// currently supported in WASI or GOOS=js
	const O_NOATIME = fsapi.Oflag(0x40000)
	require.Zero(t, 0, toOsOpenFlag(O_NOATIME))
}
