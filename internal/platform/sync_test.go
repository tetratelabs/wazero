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
	f, err := os.CreateTemp("", t.Name())
	require.NoError(t, err)
	defer f.Close()

	expected := "hello world!"

	// Write the expected data
	_, err = f.Write([]byte(expected))
	require.NoError(t, err)

	// Sync the data.
	if err = Fdatasync(f); err == syscall.ENOSYS {
		return // don't continue if it isn't supported.
	}
	require.NoError(t, err)

	// Rewind while the file is still open.
	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)

	// Read data from the file
	var buf bytes.Buffer
	_, err = io.Copy(&buf, f)
	require.NoError(t, err)

	// It may be the case that sync worked.
	require.Equal(t, expected, buf.String())
}
