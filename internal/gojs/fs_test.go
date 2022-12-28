package gojs_test

import (
	"os"
	"path"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/writefs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_fs(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := compileAndRun(testCtx, "fs", wazero.NewModuleConfig().WithFS(testFS))

	require.Zero(t, stderr)
	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, `TestFS ok
wd ok
Not a directory
sub mode drwxr-xr-x
/test.txt mode -rw-r--r--
test.txt mode -rw-r--r--
contents: animals

empty: 
`, stdout)
}

func Test_writefs(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	fs := writefs.New(tmpDir)
	// test expects to write under /tmp
	require.NoError(t, os.Mkdir(path.Join(tmpDir, "tmp"), 0o700))

	stdout, stderr, err := compileAndRun(testCtx, "writefs", wazero.NewModuleConfig().WithFS(fs))

	require.Zero(t, stderr)
	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, `/tmp/dir mode drwx------
/tmp/dir/file mode -rw-------
`, stdout)
}
