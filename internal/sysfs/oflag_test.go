package sysfs

import (
	"os"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_toOsOpenFlag doesn't use subtests to reduce volume of verbose output,
// and in recognition we have tens of thousands of tests, which can hit IDE
// limits.
func Test_toOsOpenFlag(t *testing.T) {
	tests := []struct {
		name     string
		flag     sys.Oflag
		expected int
	}{
		{name: "O_RDONLY", flag: sys.O_RDONLY, expected: os.O_RDONLY},
		{name: "O_RDWR", flag: sys.O_RDWR, expected: os.O_RDWR},
		{name: "O_WRONLY", flag: sys.O_WRONLY, expected: os.O_WRONLY},
		{name: "O_CREAT", flag: sys.O_CREAT, expected: os.O_RDONLY | os.O_CREATE},
		{name: "O_APPEND", flag: sys.O_APPEND, expected: os.O_RDONLY | os.O_APPEND},
		{
			name:     "all portable",
			flag:     sys.O_RDWR | sys.O_APPEND | sys.O_CREAT | sys.O_EXCL | sys.O_SYNC | sys.O_TRUNC,
			expected: os.O_RDWR | os.O_APPEND | os.O_CREATE | os.O_EXCL | os.O_SYNC | os.O_TRUNC,
		},
		{name: "undefined", flag: 1 << 15, expected: os.O_RDONLY},
	}

	for _, tc := range tests {
		require.Equal(t, tc.expected, toOsOpenFlag(tc.flag), tc.name)
	}

	// Tests any supported syscall flags
	for n, f := range map[string]sys.Oflag{
		"O_DIRECTORY": sys.O_DIRECTORY,
		"O_DSYNC":     sys.O_DSYNC,
		"O_NOFOLLOW":  sys.O_NOFOLLOW,
		"O_NONBLOCK":  sys.O_NONBLOCK,
		"O_RSYNC":     sys.O_RSYNC,
	} {
		if supportedSyscallOflag&f == 0 {
			continue
		}
		require.NotEqual(t, 0, toOsOpenFlag(f), n)
	}

	// Example of a flag that can be or'd into O_RDONLY even if not
	// currently supported in WASI.
	const O_NOATIME = sys.Oflag(0x40000)
	require.Zero(t, 0, toOsOpenFlag(O_NOATIME))
}
