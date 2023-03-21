package platform

import (
	"bytes"
	"io"
	"os"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_Fdatasync doesn't guarantee sync works because the operating system may
// sync anyway. There is no test in Go for syscall.Fdatasync, but closest is
// similar to below. Effectively, this only tests that things don't error.
func Test_Fdatasync(t *testing.T) {
	f, errno := os.CreateTemp("", t.Name())
	require.NoError(t, errno)
	defer f.Close()

	expected := "hello world!"

	// Write the expected data
	_, errno = f.Write([]byte(expected))
	require.NoError(t, errno)

	// Sync the data.
	if errno = Fdatasync(f); errno == syscall.ENOSYS {
		return // don't continue if it isn't supported.
	}
	require.Zero(t, errno)

	// Rewind while the file is still open.
	_, err := f.Seek(0, io.SeekStart)
	require.NoError(t, err)

	// Read data from the file
	var buf bytes.Buffer
	_, errno = io.Copy(&buf, f)
	require.NoError(t, errno)

	// It may be the case that sync worked.
	require.Equal(t, expected, buf.String())
}
