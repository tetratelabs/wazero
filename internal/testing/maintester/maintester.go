package maintester

import (
	"os"
	"path"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMain(t *testing.T, main func(), args ...string) (stdout, stderr string) {
	// Setup files to capture stdout and stderr
	tmp := t.TempDir()

	stdoutPath := path.Join(tmp, "stdout.txt")
	stdoutF, err := os.Create(stdoutPath)
	require.NoError(t, err)

	stderrPath := path.Join(tmp, "stderr.txt")
	stderrF, err := os.Create(stderrPath)
	require.NoError(t, err)

	// Save the old os.XXX and revert regardless of the outcome.
	oldArgs := os.Args
	os.Args = args
	oldStdout := os.Stdout
	os.Stdout = stdoutF
	oldStderr := os.Stderr
	os.Stderr = stderrF
	revertOS := func() {
		os.Args = oldArgs
		_ = stdoutF.Close()
		os.Stdout = oldStdout
		_ = stderrF.Close()
		os.Stderr = oldStderr
	}
	defer revertOS()

	// Run the main command.
	main()

	// Revert os.XXX so that test output is visible on failure.
	revertOS()

	// Capture any output and return it in a portable way (ex without windows newlines)
	stdoutB, err := os.ReadFile(stdoutPath)
	require.NoError(t, err)
	stdout = strings.ReplaceAll(string(stdoutB), "\r\n", "\n")

	stderrB, err := os.ReadFile(stderrPath)
	require.NoError(t, err)
	stderr = strings.ReplaceAll(string(stderrB), "\r\n", "\n")

	return
}
