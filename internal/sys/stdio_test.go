package sys

import (
	"io/fs"
	"os"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestStdio(t *testing.T) {
	// simulate regular file attached to stdin
	f, err := os.CreateTemp(t.TempDir(), "somefile")
	require.NoError(t, err)
	defer f.Close()

	stdin, err := stdinFileEntry(os.Stdin)
	require.NoError(t, err)
	stdinStat, err := os.Stdin.Stat()
	require.NoError(t, err)

	stdinNil, err := stdinFileEntry(nil)
	require.NoError(t, err)

	stdinFile, err := stdinFileEntry(f)
	require.NoError(t, err)

	stdout, err := stdioWriterFileEntry("stdout", os.Stdout)
	require.NoError(t, err)
	stdoutStat, err := os.Stdout.Stat()
	require.NoError(t, err)

	stdoutNil, err := stdioWriterFileEntry("stdout", nil)
	require.NoError(t, err)

	stdoutFile, err := stdioWriterFileEntry("stdout", f)
	require.NoError(t, err)

	stderr, err := stdioWriterFileEntry("stderr", os.Stderr)
	require.NoError(t, err)
	stderrStat, err := os.Stderr.Stat()
	require.NoError(t, err)

	stderrNil, err := stdioWriterFileEntry("stderr", nil)
	require.NoError(t, err)

	stderrFile, err := stdioWriterFileEntry("stderr", f)
	require.NoError(t, err)

	tests := []struct {
		name string
		f    *FileEntry
		// Depending on how the tests run, os.Stdin won't necessarily be a char
		// device. We compare against an os.File, to account for this.
		expectedType fs.FileMode
	}{
		{
			name:         "stdin",
			f:            stdin,
			expectedType: stdinStat.Mode().Type(),
		},
		{
			name:         "stdin noop",
			f:            stdinNil,
			expectedType: fs.ModeDevice,
		},
		{
			name:         "stdin file",
			f:            stdinFile,
			expectedType: 0, // normal file
		},
		{
			name:         "stdout",
			f:            stdout,
			expectedType: stdoutStat.Mode().Type(),
		},
		{
			name:         "stdout noop",
			f:            stdoutNil,
			expectedType: fs.ModeDevice,
		},
		{
			name:         "stdout file",
			f:            stdoutFile,
			expectedType: 0, // normal file
		},
		{
			name:         "stderr",
			f:            stderr,
			expectedType: stderrStat.Mode().Type(),
		},
		{
			name:         "stderr noop",
			f:            stderrNil,
			expectedType: fs.ModeDevice,
		},
		{
			name:         "stderr file",
			f:            stderrFile,
			expectedType: 0, // normal file
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name+" Stat", func(t *testing.T) {
			st, errno := tc.f.File.Stat()
			require.EqualErrno(t, 0, errno)
			require.Equal(t, tc.expectedType, st.Mode&fs.ModeType)
			require.Equal(t, uint64(1), st.Nlink)

			// Fake times are needed to pass wasi-testsuite.
			// See https://github.com/WebAssembly/wasi-testsuite/blob/af57727/tests/rust/src/bin/fd_filestat_get.rs#L1-L19
			require.Zero(t, st.Ctim)
			require.Zero(t, st.Mtim)
			require.Zero(t, st.Atim)
		})

		buf := make([]byte, 5)
		switch tc.f {
		case stdinNil:
			t.Run(tc.name+" returns zero on Read", func(t *testing.T) {
				n, errno := tc.f.File.Read(buf)
				require.EqualErrno(t, 0, errno)
				require.Zero(t, n) // like reading io.EOF
			})
		case stdoutNil, stderrNil:
			// This is important because some code will loop forever attempting
			// to write data. This happened in TestShortHash.
			t.Run(tc.name+" returns length on Write", func(t *testing.T) {
				n, errno := tc.f.File.Write(buf)
				require.EqualErrno(t, 0, errno)
				require.Equal(t, len(buf), n) // like io.Discard
			})
		}
	}
}
